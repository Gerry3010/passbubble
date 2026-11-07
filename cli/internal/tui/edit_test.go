// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
	vaultpkg "github.com/Gerry3010/passbubble/cli/internal/vault"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	editTestEntryID  = "e1"
	editTestFolderID = "f1"
	editTestURL      = "https://github.com"
)

// A stored entry with everything an edit must not lose: notes, tags, custom
// fields of all kinds and keys only other clients know about.
const editTestStored = `{
	"username": "octo",
	"password": "old-password",
	"notes": "my notes",
	"sign_in_with": "google",
	"totp_secret": "JBSWY3DPEHPK3PXP",
	"future_flag": {"nested": [1, 2]},
	"tags": ["work", "two words"],
	"custom_fields": [
		{"label": "PIN", "value": "1234", "type": "password"},
		{"label": "Scan", "value": "aGVsbG8=", "type": "file", "filename": "scan.png", "mime_type": "image/png", "future_cf": true}
	]
}`

// editServer is the entry API for one entry. Anything that would create or
// delete an entry is recorded in forbidden.
type editServer struct {
	t       *testing.T
	v       *vaultpkg.Vault
	dataKey []byte

	mu        sync.Mutex
	entry     apiclient.EntryResponse
	puts      []apiclient.UpdateEntryRequest
	forbidden []string
	failPut   bool
}

func newEditServer(t *testing.T, entryType, stored string) *editServer {
	t.Helper()
	s := &editServer{t: t}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	s.v = unlockedVault(t, srv.URL)

	dataKey, err := crypto.RandKey()
	if err != nil {
		t.Fatal(err)
	}
	s.dataKey = dataKey
	ciphertext, err := crypto.Encrypt(dataKey, []byte(stored))
	if err != nil {
		t.Fatal(err)
	}
	pubX, _ := crypto.B64Dec(s.v.Config().PubX25519)
	pubM, _ := crypto.B64Dec(s.v.Config().PubMLKEM768)
	wrapped, err := crypto.EncryptDataKey(dataKey, pubX, pubM)
	if err != nil {
		t.Fatal(err)
	}
	folder := editTestFolderID
	s.entry = apiclient.EntryResponse{
		ID: editTestEntryID, Name: "GitHub", URL: editTestURL, Type: entryType, FolderID: &folder,
		EncryptedData: crypto.B64Enc(ciphertext),
		EntryKey:      &apiclient.EntryKey{UserID: "u1", EncryptedKey: crypto.B64Enc(wrapped)},
	}
	return s
}

func (s *editServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	route := r.Method + " " + r.URL.Path
	switch route {
	case "GET /api/v1/entries/" + editTestEntryID:
		_ = json.NewEncoder(w).Encode(s.entry)
	case "GET /api/v1/entries":
		row := s.entry
		row.EncryptedData, row.EntryKey = "", nil
		_ = json.NewEncoder(w).Encode([]apiclient.EntryResponse{row})
	case "GET /api/v1/folders":
		_ = json.NewEncoder(w).Encode([]apiclient.FolderResponse{})
	case "PUT /api/v1/entries/" + editTestEntryID:
		if s.failPut {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var req apiclient.UpdateEntryRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.puts = append(s.puts, req)
		s.entry.Name, s.entry.EncryptedData = req.Name, req.EncryptedData
		w.WriteHeader(http.StatusNoContent)
	default:
		s.forbidden = append(s.forbidden, route)
		w.WriteHeader(http.StatusNotFound)
	}
}

// savedData decrypts the blob of the only update and returns it in comparable
// form. It also proves the update reused the entry's data key.
func (s *editServer) savedData(t *testing.T) any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.forbidden) != 0 {
		t.Fatalf("an edit must not create or delete entries: %v", s.forbidden)
	}
	if len(s.puts) != 1 {
		t.Fatalf("want exactly 1 update, got %d", len(s.puts))
	}
	ciphertext, err := crypto.B64Dec(s.puts[0].EncryptedData)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := crypto.Decrypt(s.dataKey, ciphertext)
	if err != nil {
		t.Fatalf("the update did not reuse the data key: %v", err)
	}
	return canonJSON(t, string(plaintext))
}

func canonJSON(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("%v in %s", err, s)
	}
	return v
}

// withField returns stored (a JSON object) with one key replaced.
func withField(t *testing.T, stored, key string, value any) any {
	t.Helper()
	m := canonJSON(t, stored).(map[string]any)
	m[key] = value
	return m
}

// editModel is an unlocked model on the list, the cursor on the entry, with the
// username unknown as in a real list row: it sits in the encrypted data.
func (s *editServer) editModel(t *testing.T) Model {
	t.Helper()
	m, _ := newTestModel(t, &config.Config{})
	m.vault = s.v
	m.folderStack = []*vaultpkg.Folder{{ID: editTestFolderID, Name: "Work"}}
	folder := editTestFolderID
	m.allEntries = []Entry{{
		ID: editTestEntryID, Service: "GitHub", URL: editTestURL, Type: s.entry.Type, FolderID: &folder,
	}}
	return m
}

// drain runs cmd and feeds what it produces back into the model until nothing
// is left, the way the bubbletea runtime does.
func drain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		msg := next()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		if msg == nil {
			continue
		}
		updated, more := m.Update(msg)
		m = updated.(Model)
		queue = append(queue, more)
	}
	return m
}

func openEdit(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.runAction(actEdit)
	m = drain(t, next.(Model), cmd)
	if !m.showingForm || m.form.Type != EditEntryForm {
		t.Fatalf("the edit form should be open (status %q)", m.status)
	}
	return m
}

func submit(t *testing.T, m Model) Model {
	t.Helper()
	m.form.CurrentField = len(m.form.Fields) - 1
	next, cmd := m.Update(key("enter"))
	m = drain(t, next.(Model), cmd)
	if m.showingForm {
		t.Fatalf("the form should be closed, error: %q", m.form.Error)
	}
	return m
}

func TestEditUpdatesInPlaceAndKeepsEveryField(t *testing.T) {
	s := newEditServer(t, "password", editTestStored)
	m := openEdit(t, s.editModel(t))

	if got := m.form.Fields[1].Value; got != "octo" {
		t.Errorf("the username should be filled in from the entry, got %q", got)
	}
	if got := m.form.Fields[2].Value; got != "" {
		t.Errorf("the password field should start empty, got %q", got)
	}

	m = send(t, m, "tab", "tab")
	m = typeText(t, m, "new-password")
	m = submit(t, m)

	want := withField(t, editTestStored, "password", "new-password")
	if got := s.savedData(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved entry lost data\n got: %v\nwant: %v", got, want)
	}
	put := s.puts[0]
	if put.Name != "GitHub" || put.URL == nil || *put.URL != editTestURL {
		t.Errorf("metadata changed: name %q, url %v", put.Name, put.URL)
	}
	if put.FolderID == nil || *put.FolderID != editTestFolderID {
		t.Errorf("the entry left its folder: %v", put.FolderID)
	}
	if put.MatchPatterns != nil {
		t.Errorf("match patterns should stay untouched, got %v", put.MatchPatterns)
	}
	if m.statusType != "success" {
		t.Errorf("status = %q (%s)", m.status, m.statusType)
	}
}

func TestEditRenameKeepsTheSameEntry(t *testing.T) {
	s := newEditServer(t, "password", editTestStored)
	m := openEdit(t, s.editModel(t))

	m = typeText(t, m, " Work")
	m = send(t, m, "tab")
	m = clearField(t, m)
	m = typeText(t, m, "hubot")
	m = submit(t, m)

	want := withField(t, editTestStored, "username", "hubot")
	if got := s.savedData(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved entry lost data\n got: %v\nwant: %v", got, want)
	}
	if s.puts[0].Name != "GitHub Work" {
		t.Errorf("name = %q", s.puts[0].Name)
	}
	if len(m.allEntries) != 1 || m.allEntries[0].ID != editTestEntryID || m.allEntries[0].Service != "GitHub Work" {
		t.Errorf("the list should show the renamed entry: %+v", m.allEntries)
	}
}

func TestEditWithoutChangesChangesNothing(t *testing.T) {
	s := newEditServer(t, "password", editTestStored)
	m := submit(t, openEdit(t, s.editModel(t)))

	if got, want := s.savedData(t), canonJSON(t, editTestStored); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved entry changed\n got: %v\nwant: %v", got, want)
	}
	if m.statusType != "success" {
		t.Errorf("status = %q (%s)", m.status, m.statusType)
	}
}

func TestEditTOTPStartsFromStoredSettings(t *testing.T) {
	const stored = `{"username":"octo","totp_secret":"JBSWY3DPEHPK3PXP","issuer":"AWS",
		"period":60,"digits":8,"algorithm":"SHA256","notes":"n","future_flag":true}`

	t.Run("unchanged", func(t *testing.T) {
		s := newEditServer(t, "totp", stored)
		m := openEdit(t, s.editModel(t))

		got := map[string]string{}
		for _, f := range m.form.Fields {
			got[f.Label] = f.Value
		}
		want := map[string]string{
			"Service": "GitHub", "Username/Email": "octo", "Issuer": "AWS",
			"TOTP Secret": "", "Algorithm": "SHA256", "Length": "8", "Period": "60",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("form fields = %v, want %v", got, want)
		}

		submit(t, m)
		if got, want := s.savedData(t), canonJSON(t, stored); !reflect.DeepEqual(got, want) {
			t.Fatalf("saved entry changed\n got: %v\nwant: %v", got, want)
		}
	})

	t.Run("new secret", func(t *testing.T) {
		s := newEditServer(t, "totp", stored)
		m := openEdit(t, s.editModel(t))
		m = send(t, m, "tab", "tab", "tab")
		m = typeText(t, m, "GEZDGNBVGY3TQOJQ")
		submit(t, m)

		want := withField(t, stored, "totp_secret", "GEZDGNBVGY3TQOJQ")
		if got := s.savedData(t); !reflect.DeepEqual(got, want) {
			t.Fatalf("saved entry\n got: %v\nwant: %v", got, want)
		}
	})
}

func TestEditKeepsThePasswordOfATOTPEntry(t *testing.T) {
	const stored = `{"username":"octo","password":"hunter2","totp_secret":"JBSWY3DPEHPK3PXP","issuer":"AWS"}`
	s := newEditServer(t, "totp", stored)
	m := s.editModel(t)
	next, cmd := m.openDetail(m.allEntries[0])
	m = drain(t, next.(Model), cmd)
	if m.password != "hunter2" {
		t.Fatalf("password = %q", m.password)
	}

	next, cmd = m.Update(key("e"))
	m = drain(t, next.(Model), cmd)
	if !m.showingForm || m.form.Type != EditEntryForm {
		t.Fatalf("e should open the edit form (status %q)", m.status)
	}
	m = send(t, m, "tab", "tab")
	m = clearField(t, m)
	m = typeText(t, m, "Amazon")
	m = submit(t, m)

	want := withField(t, stored, "issuer", "Amazon")
	if got := s.savedData(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved\n got: %v\nwant: %v", got, want)
	}
	if m.password != "hunter2" {
		t.Errorf("the detail screen lost the password: %q", m.password)
	}
}

func TestEditOtherTypesKeepTheirData(t *testing.T) {
	const stored = `{"card_number":"4111111111111111","holder_name":"Max","cvv":"123","expiry_month":"12","expiry_year":"2030"}`
	s := newEditServer(t, "credit-card", stored)
	submit(t, openEdit(t, s.editModel(t)))

	if got, want := s.savedData(t), canonJSON(t, stored); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved entry changed\n got: %v\nwant: %v", got, want)
	}
}

func TestEditFromDetailScreenFollowsTheEntry(t *testing.T) {
	s := newEditServer(t, "password", editTestStored)
	m := s.editModel(t)
	m.screen = DetailScreen
	m.detailEntry = m.allEntries[0]
	m.password = "old-password"

	next, cmd := m.Update(key("e"))
	m = drain(t, next.(Model), cmd)
	if !m.showingForm || m.form.Type != EditEntryForm {
		t.Fatalf("e should open the edit form (status %q)", m.status)
	}
	m = typeText(t, m, "2")
	m = send(t, m, "tab", "tab")
	m = typeText(t, m, "new-password")
	m = submit(t, m)

	if m.screen != DetailScreen {
		t.Errorf("screen = %v, want DetailScreen", m.screen)
	}
	if m.detailEntry.Service != "GitHub2" || m.detailEntry.Username != "octo" || m.password != "new-password" {
		t.Errorf("the detail screen should show the edited entry: %+v, password %q", m.detailEntry, m.password)
	}
}

func TestEditFailureIsReported(t *testing.T) {
	s := newEditServer(t, "password", editTestStored)
	s.failPut = true
	m := submit(t, openEdit(t, s.editModel(t)))

	if m.statusType != "error" || !strings.Contains(m.status, "Failed to update entry") {
		t.Errorf("status = %q (%s)", m.status, m.statusType)
	}
	if len(s.forbidden) != 0 {
		t.Errorf("a failed edit must not touch anything else: %v", s.forbidden)
	}
}

func TestEditFormIsNotOpenedWhenEntryCannotBeLoaded(t *testing.T) {
	s := newEditServer(t, "password", editTestStored)
	m := s.editModel(t)
	m.allEntries[0].ID = "missing"

	next, cmd := m.runAction(actEdit)
	m = drain(t, next.(Model), cmd)
	if m.showingForm || m.statusType != "error" {
		t.Errorf("form open = %v, status %q (%s)", m.showingForm, m.status, m.statusType)
	}
}

func TestLateEditFormDoesNotOpen(t *testing.T) {
	loaded := EditFormLoadedMsg{
		Entry: Entry{ID: editTestEntryID, Service: "GitHub", Type: "password"},
		Data:  &vaultpkg.EntryData{Username: "octo"},
	}

	t.Run("after a lock", func(t *testing.T) {
		m := modelWithForm(t, CreateAddPasswordForm()).lockVault()
		next, _ := m.Update(loaded)
		if next.(Model).showingForm {
			t.Fatal("a form must not open over the unlock screen")
		}
	})

	t.Run("over another form", func(t *testing.T) {
		m := modelWithForm(t, CreateAddPasswordForm())
		next, _ := m.Update(loaded)
		if got := next.(Model).form.Type; got != AddPasswordForm {
			t.Fatalf("the open form was replaced by %v", got)
		}
	})
}

func TestApplyEntryEdit(t *testing.T) {
	base := func() *vaultpkg.EntryData {
		return &vaultpkg.EntryData{
			Username: " octo ", Password: "pw", TOTPSecret: "JBSWY3DPEHPK3PXP",
			Issuer: "AWS", Algorithm: "SHA1", Digits: 6, Period: 30,
		}
	}

	d := base()
	if err := applyEntryEdit(d, "password", map[string]string{"username": "octo", "password": "  "}); err != nil {
		t.Fatal(err)
	}
	if d.Username != " octo " || d.Password != "pw" {
		t.Errorf("untouched fields changed: %+v", d)
	}

	d = base()
	if err := applyEntryEdit(d, "password", map[string]string{"username": ""}); err != nil {
		t.Fatal(err)
	}
	if d.Username != "" {
		t.Errorf("clearing the username should remove it, got %q", d.Username)
	}

	d = base()
	err := applyEntryEdit(d, "totp", map[string]string{
		"username": "octo", "issuer": "", "algorithm": "sha512", "length": "8", "period": "45",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Issuer != "" || d.Algorithm != "SHA512" || d.Digits != 8 || d.Period != 45 || d.TOTPSecret != "JBSWY3DPEHPK3PXP" {
		t.Errorf("totp edit = %+v", d)
	}

	for _, bad := range []map[string]string{
		{"secret": "not base32!"},
		{"algorithm": "MD5"},
		{"length": "7"},
		{"period": "5"},
	} {
		if err := applyEntryEdit(base(), "totp", bad); err == nil {
			t.Errorf("%v should be rejected", bad)
		}
	}
}

// The old TUI copied the password into totp_secret. After a password change the
// old value would pass for a real secret, so an edit must not write it back.
func TestEditDoesNotWriteAStrayTOTPSecretBack(t *testing.T) {
	const stored = `{"username":"octo","password":"old-password","totp_secret":"old-password","notes":"n"}`
	s := newEditServer(t, "password", stored)
	m := openEdit(t, s.editModel(t))
	m = send(t, m, "tab", "tab")
	m = typeText(t, m, "new-password")
	submit(t, m)

	want := canonJSON(t, `{"username":"octo","password":"new-password","notes":"n"}`)
	if got := s.savedData(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved\n got: %v\nwant: %v", got, want)
	}
}

func TestEditKeepsARealTOTPSecretThroughAPasswordChange(t *testing.T) {
	const stored = `{"username":"octo","password":"old-password","totp_secret":"JBSWY3DPEHPK3PXP",
		"issuer":"AWS","period":60,"digits":8,"algorithm":"SHA256"}`
	s := newEditServer(t, "password", stored)
	m := openEdit(t, s.editModel(t))
	m = send(t, m, "tab", "tab")
	m = typeText(t, m, "new-password")
	submit(t, m)

	want := withField(t, stored, "password", "new-password")
	if got := s.savedData(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved\n got: %v\nwant: %v", got, want)
	}
}

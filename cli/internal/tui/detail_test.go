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
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
	vaultpkg "github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/Gerry3010/passbubble/cli/pkg/backup"
	"github.com/Gerry3010/passbubble/cli/pkg/keyring"
	"github.com/Gerry3010/passbubble/cli/pkg/totp"
	"github.com/pquerna/otp"
)

// listServer serves several entries, as a vault with same-named entries does.
type listServer struct {
	v *vaultpkg.Vault

	mu       sync.Mutex
	entries  []apiclient.EntryResponse
	deleted  []string
	searched int
	created  []string // IDs of the entries a client added
}

type listEntry struct {
	id, name, entryType, plaintext string
}

func newListServer(t *testing.T, entries ...listEntry) *listServer {
	t.Helper()
	s := &listServer{}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	s.v = unlockedVault(t, srv.URL)
	pubX, _ := crypto.B64Dec(s.v.Config().PubX25519)
	pubM, _ := crypto.B64Dec(s.v.Config().PubMLKEM768)
	for _, e := range entries {
		dataKey, err := crypto.RandKey()
		if err != nil {
			t.Fatal(err)
		}
		ciphertext, err := crypto.Encrypt(dataKey, []byte(e.plaintext))
		if err != nil {
			t.Fatal(err)
		}
		wrapped, err := crypto.EncryptDataKey(dataKey, pubX, pubM)
		if err != nil {
			t.Fatal(err)
		}
		s.entries = append(s.entries, apiclient.EntryResponse{
			ID: e.id, Name: e.name, Type: e.entryType, EncryptedData: crypto.B64Enc(ciphertext),
			EntryKey: &apiclient.EntryKey{UserID: "u1", EncryptedKey: crypto.B64Enc(wrapped)},
		})
	}
	return s
}

func (s *listServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := func() []apiclient.EntryResponse {
		out := []apiclient.EntryResponse{}
		for _, e := range s.entries {
			if slices.Contains(s.deleted, e.ID) {
				continue
			}
			e.EncryptedData, e.EntryKey = "", nil
			out = append(out, e)
		}
		return out
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/entries/")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/entries":
		_ = json.NewEncoder(w).Encode(rows())
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/entries/search":
		s.searched++
		_ = json.NewEncoder(w).Encode(rows())
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/folders":
		_ = json.NewEncoder(w).Encode([]apiclient.FolderResponse{})
	case r.Method == http.MethodGet:
		for _, e := range s.entries {
			if e.ID == id {
				_ = json.NewEncoder(w).Encode(e)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	case r.Method == http.MethodDelete:
		s.deleted = append(s.deleted, id)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/entries":
		var req apiclient.CreateEntryRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		newID := fmt.Sprintf("new-%d", len(s.created)+1)
		s.created = append(s.created, newID)
		s.entries = append(s.entries, apiclient.EntryResponse{
			ID: newID, Name: req.Name, Type: req.Type, URL: req.URL,
			EncryptedData: req.EncryptedData, EntryKey: &req.EntryKeys[0],
		})
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": newID})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// listModel is an unlocked model on the list. The keyring shim points at the
// same vault, as it does in the real program.
func (s *listServer) listModel(t *testing.T) Model {
	t.Helper()
	keyring.SetGlobal(vaultpkg.NewKeyringAdapter(s.v))
	t.Cleanup(func() { keyring.SetGlobal(nil) })

	m, _ := newTestModel(t, &config.Config{})
	m.vault = s.v
	for _, e := range s.entries {
		m.allEntries = append(m.allEntries, Entry{ID: e.ID, Service: e.Name, Type: e.Type})
	}
	return m
}

// press sends keys and runs whatever they start, as the runtime does.
func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, cmd := m.Update(key(k))
		m = drain(t, next.(Model), cmd)
	}
	return m
}

func twoGitHubs() []listEntry {
	return []listEntry{
		{"e1", "GitHub", "password", `{"username":"octo","password":"pw-first"}`},
		{"e2", "GitHub", "password", `{"username":"hubot","password":"pw-second"}`},
	}
}

func TestDetailShowsPasswordOfTheSelectedEntry(t *testing.T) {
	s := newListServer(t, twoGitHubs()...)
	m := s.listModel(t)

	m = press(t, m, "j", "enter")
	if m.screen != DetailScreen || m.detailEntry.ID != "e2" {
		t.Fatalf("expected the second entry's detail screen, got %v / %q", m.screen, m.detailEntry.ID)
	}
	if m.password != "pw-second" {
		t.Fatalf("detail shows %q, want the password of the selected entry", m.password)
	}
}

func TestDetailDoesNotShowThePreviousPasswordWhileLoading(t *testing.T) {
	s := newListServer(t, twoGitHubs()...)
	m := s.listModel(t)

	m = press(t, m, "enter")
	if m.password != "pw-first" {
		t.Fatalf("password = %q", m.password)
	}
	m = press(t, m, "esc", "j")

	next, _ := m.Update(key("enter")) // the load has not come back yet
	m = next.(Model)
	if m.detailEntry.ID != "e2" {
		t.Fatalf("detail entry = %q", m.detailEntry.ID)
	}
	if m.password != "" {
		t.Fatalf("the previous entry's password %q is on the new detail screen", m.password)
	}
}

func TestDetailTOTPUsesTheEntrysSecretAndSettings(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"
	s := newListServer(t, listEntry{"e1", "AWS", "totp",
		`{"totp_secret":"` + secret + `","period":300,"digits":8,"algorithm":"SHA256"}`})
	m := s.listModel(t)
	opts := &totp.GenerateOptions{Period: 300, Digits: otp.DigitsEight, Algorithm: otp.AlgorithmSHA256}

	want := func() string {
		code, err := totp.GenerateCode(secret, opts)
		if err != nil {
			t.Fatal(err)
		}
		return totp.FormatCode(code)
	}
	before := want()
	m = press(t, m, "enter", "s")
	after := want() // a code changes only every 300 s; both sides cover the edge

	if m.statusType == "error" {
		t.Fatalf("status: %s", m.status)
	}
	if m.totpCode != before && m.totpCode != after {
		t.Fatalf("code = %q, want %q", m.totpCode, after)
	}
	if !strings.Contains(m.renderDetailScreen(), m.totpCode) {
		t.Errorf("the code should be on screen:\n%s", m.renderDetailScreen())
	}
}

// Entries stored as totp before the importers learned better hold a password too.
func TestDetailShowsThePasswordOfATOTPEntryMasked(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"
	s := newListServer(t, listEntry{"e1", "AWS", "totp",
		`{"username":"root","password":"hunter2","totp_secret":"` + secret + `"}`})
	m := s.listModel(t)

	m = press(t, m, "enter")
	if m.password != "hunter2" {
		t.Fatalf("the password was not loaded: %q", m.password)
	}
	screen := m.renderDetailScreen()
	if !strings.Contains(screen, "Password:") || !strings.Contains(screen, "•••••••") {
		t.Errorf("the password should be there, masked:\n%s", screen)
	}
	if strings.Contains(screen, "hunter2") {
		t.Errorf("the password must start masked:\n%s", screen)
	}
	if strings.Contains(screen, "TOTP Code") {
		t.Errorf("the code must start hidden:\n%s", screen)
	}

	m = press(t, m, "s")
	screen = m.renderDetailScreen()
	if !strings.Contains(screen, "hunter2") || !strings.Contains(screen, m.totpCode) || m.totpCode == "" {
		t.Errorf("s should reveal password and code (%q):\n%s", m.totpCode, screen)
	}

	m = press(t, m, "s")
	screen = m.renderDetailScreen()
	if strings.Contains(screen, "hunter2") || strings.Contains(screen, "TOTP Code") {
		t.Errorf("s again should hide both:\n%s", screen)
	}

	// Not run: that would write to the real clipboard.
	if _, cmd := m.Update(key("p")); cmd == nil {
		t.Error("p should copy the password")
	}
}

func TestDetailOfATOTPEntryWithoutPasswordShowsNoPasswordLine(t *testing.T) {
	s := newListServer(t, listEntry{"e1", "AWS", "totp", `{"totp_secret":"JBSWY3DPEHPK3PXP"}`})
	m := s.listModel(t)

	m = press(t, m, "enter")
	screen := m.renderDetailScreen()
	if strings.Contains(screen, "Password") {
		t.Errorf("no password, no password line:\n%s", screen)
	}
	if _, cmd := m.Update(key("p")); cmd != nil {
		t.Error("p has nothing to copy here")
	}
}

// The importers keep a login with 2FA a password entry, and it must not lose its
// code over that.
func TestDetailShowsTheCodeOfALoginWithASecret(t *testing.T) {
	s := newListServer(t, listEntry{"e1", "GitHub", "password",
		`{"username":"octo","password":"hunter2","totp_secret":"JBSWY3DPEHPK3PXP"}`})
	m := s.listModel(t)

	m = press(t, m, "enter")
	screen := m.renderDetailScreen()
	if strings.Contains(screen, "hunter2") || strings.Contains(screen, "TOTP Code") {
		t.Errorf("both must start hidden:\n%s", screen)
	}
	if !strings.Contains(screen, "TOTP code hidden") {
		t.Errorf("the screen should say a code exists:\n%s", screen)
	}

	m = press(t, m, "s")
	screen = m.renderDetailScreen()
	if m.totpCode == "" || !strings.Contains(screen, m.totpCode) || !strings.Contains(screen, "hunter2") {
		t.Errorf("s should reveal password and code (%q):\n%s", m.totpCode, screen)
	}

	// The tick keeps the code current on this entry too.
	m.totpRemaining = 5
	next, _ := m.Update(TickMsg{})
	m = next.(Model)
	if m.totpRemaining != 4 {
		t.Errorf("the tick should count the code down, remaining %d", m.totpRemaining)
	}

	// Not run: that would write to the real clipboard.
	if _, cmd := m.Update(key("o")); cmd == nil {
		t.Error("o should copy the code")
	}

	m = press(t, m, "s")
	screen = m.renderDetailScreen()
	if strings.Contains(screen, "hunter2") || strings.Contains(screen, "TOTP Code") || m.totpCode != "" {
		t.Errorf("s again should hide both:\n%s", screen)
	}
}

func TestDetailOfALoginWithoutSecretShowsNoCode(t *testing.T) {
	s := newListServer(t, listEntry{"e1", "GitHub", "password", `{"username":"octo","password":"hunter2"}`})
	m := s.listModel(t)

	m = press(t, m, "enter", "s")
	screen := m.renderDetailScreen()
	if !strings.Contains(screen, "hunter2") {
		t.Errorf("s should reveal the password:\n%s", screen)
	}
	if strings.Contains(screen, "TOTP Code") || strings.Contains(screen, "TOTP code hidden") {
		t.Errorf("no secret, no code:\n%s", screen)
	}
}

// Stray secrets are dropped on decryption, so no code appears here, even for a
// password that happens to be valid base32 once cleaned up.
func TestDetailShowsNoCodeForPasswordInTOTPSecret(t *testing.T) {
	s := newListServer(t, listEntry{"e1", "Old", "password",
		`{"username":"octo","password":"correct-horse","totp_secret":"correct-horse"}`})
	m := s.listModel(t)

	m = press(t, m, "enter", "s")
	if m.password != "correct-horse" {
		t.Fatalf("password = %q", m.password)
	}
	if m.totpCode != "" || strings.Contains(m.renderDetailScreen(), "TOTP Code") {
		t.Fatalf("a bogus TOTP code appeared (%q):\n%s", m.totpCode, m.renderDetailScreen())
	}
}

func TestDeleteRemovesTheSelectedEntry(t *testing.T) {
	s := newListServer(t, twoGitHubs()...)
	m := s.listModel(t)

	m = press(t, m, "j", "d")
	if !m.showingForm || m.form.Type != ConfirmDeleteForm {
		t.Fatal("d should ask for confirmation")
	}
	m = press(t, m, "y")

	if !slices.Equal(s.deleted, []string{"e2"}) {
		t.Fatalf("deleted %v, want the selected entry e2", s.deleted)
	}
	if m.statusType == "error" {
		t.Errorf("status: %s", m.status)
	}
	if len(m.allEntries) != 1 || m.allEntries[0].ID != "e1" {
		t.Errorf("list after delete: %+v", m.allEntries)
	}
}

func TestDeleteCancelledDeletesNothing(t *testing.T) {
	s := newListServer(t, twoGitHubs()...)
	m := s.listModel(t)

	m = press(t, m, "d", "n")
	if len(s.deleted) != 0 || m.showingForm {
		t.Fatalf("deleted %v, form open %v", s.deleted, m.showingForm)
	}
}

func TestLateAnswerForAnotherEntryIsDropped(t *testing.T) {
	s := newListServer(t, twoGitHubs()...)
	m := s.listModel(t)
	m.screen = DetailScreen
	m.detailEntry = Entry{ID: "e2", Service: "GitHub", Type: "password"}

	next, _ := m.Update(SecretLoadMsg{EntryID: "e1", Password: "pw-first"})
	m = next.(Model)
	if m.password != "" {
		t.Fatalf("the password of e1 landed on the detail screen of e2: %q", m.password)
	}
	next, _ = m.Update(TOTPUpdateMsg{EntryID: "e1", Code: "123 456", Remaining: 20})
	m = next.(Model)
	if m.totpCode != "" {
		t.Fatalf("the code of e1 landed on the detail screen of e2: %q", m.totpCode)
	}

	next, _ = m.Update(SecretLoadMsg{EntryID: "e2", Password: "pw-second"})
	if got := next.(Model).password; got != "pw-second" {
		t.Fatalf("password = %q", got)
	}
}

func TestDetailTOTPWithoutSecretShowsNoCode(t *testing.T) {
	s := newListServer(t, listEntry{"e1", "AWS", "totp", `{"username":"root"}`})
	m := s.listModel(t)

	m = press(t, m, "enter", "s")
	if m.totpCode != "" {
		t.Fatalf("an empty secret produced the code %q", m.totpCode)
	}
	if m.statusType != "error" || !strings.Contains(m.status, "no TOTP secret") {
		t.Errorf("status = %q (%s)", m.status, m.statusType)
	}
}

// The detail screen and delete find entries by ID; nothing may fall back to a
// lookup by name, which cannot tell these two apart.
func TestDetailAndDeleteNeverSearchByName(t *testing.T) {
	s := newListServer(t, twoGitHubs()...)
	m := s.listModel(t)

	press(t, m, "j", "enter", "esc", "d", "y")
	if s.searched != 0 {
		t.Fatalf("%d lookups by name", s.searched)
	}
	if !slices.Equal(s.deleted, []string{"e2"}) {
		t.Fatalf("deleted %v", s.deleted)
	}
}

func TestTOTPEntryWithStraySecretShowsNoCode(t *testing.T) {
	s := newListServer(t, listEntry{"e1", "AWS", "totp", `{"password":"JBSWY3DPEHPK3PXP","totp_secret":"JBSWY3DPEHPK3PXP"}`})
	m := s.listModel(t)

	m = press(t, m, "enter", "s")
	if m.totpCode != "" || m.statusType != "error" {
		t.Fatalf("code %q, status %q (%s)", m.totpCode, m.status, m.statusType)
	}
}

func TestSameNamedRowsAreDistinguishable(t *testing.T) {
	m, _ := newTestModel(t, &config.Config{})
	m.height = 40
	m.allEntries = []Entry{
		{ID: "11111111-aaaa", Service: "GitHub", Type: "password", URL: "https://github.com/login"},
		{ID: "22222222-bbbb", Service: "GitHub", Type: "password", URL: "https://github.example"},
		{ID: "33333333-cccc", Service: "GitHub", Type: "password"},
		{ID: "44444444-dddd", Service: "Twin", Type: "password", URL: "https://twin.example"},
		{ID: "55555555-eeee", Service: "Twin", Type: "password", URL: "https://twin.example/other"},
		{ID: "66666666-ffff", Service: "Bank", Type: "password", URL: "https://bank.example"},
	}

	out := m.renderMainScreen()
	for _, want := range []string{
		"GitHub  · github.com",
		"GitHub  · github.example",
		"GitHub  · #33333333",
		"Twin  · twin.example #44444444",
		"Twin  · twin.example #55555555",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("list should show %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "bank.example") || strings.Contains(out, "#66666666") {
		t.Errorf("a name that occurs once needs no hint:\n%s", out)
	}
}

func TestSameNameHintsCountTheWholeVault(t *testing.T) {
	folder := "f1"
	hints := duplicateNameHints([]Entry{
		{ID: "a", Service: "GitHub", URL: "https://github.com"},
		{ID: "b", Service: "GitHub", URL: "https://work.example", FolderID: &folder},
	})
	if hints["a"] != "github.com" || hints["b"] != "work.example" {
		t.Fatalf("hints = %v", hints)
	}
}

func TestIncompleteBackupSaysWhatWasSavedAndSkipped(t *testing.T) {
	m, _ := newTestModel(t, &config.Config{})
	next, _ := m.Update(BackupCreatedMsg{
		Filename: "/tmp/b.tar.gz",
		Error:    &backup.IncompleteError{Skipped: []string{"Broken (id e9): decrypt failed"}},
	})
	m = next.(Model)
	if m.statusType != "error" || !strings.Contains(m.status, "/tmp/b.tar.gz") || !strings.Contains(m.status, "Broken (id e9)") {
		t.Fatalf("status = %q (%s)", m.status, m.statusType)
	}
	if strings.Contains(m.status, "Backup failed") {
		t.Errorf("the rest was saved, so this is not a plain failure: %q", m.status)
	}
}

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

package vault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
)

// unlockedTestVault returns a vault with a fresh hybrid key pair already in
// memory, talking to serverURL.
func unlockedTestVault(t *testing.T, serverURL string) *Vault {
	t.Helper()
	privX, pubX, err := crypto.GenerateX25519()
	if err != nil {
		t.Fatal(err)
	}
	privM, pubM, err := crypto.GenerateMLKEM768()
	if err != nil {
		t.Fatal(err)
	}
	v := New(&config.Config{
		ServerURL:   serverURL,
		UserID:      "u1",
		PubX25519:   crypto.B64Enc(pubX),
		PubMLKEM768: crypto.B64Enc(pubM),
	}, "")
	v.privX25519, v.privMLKEM = privX, privM
	return v
}

// sealEntry encrypts plaintext the way every client stores an entry and
// returns the encrypted_data blob plus the wrapped data key for the vault.
func sealEntry(t *testing.T, v *Vault, plaintext string) (encData, encKey string, dataKey []byte) {
	t.Helper()
	dataKey, err := crypto.RandKey()
	if err != nil {
		t.Fatal(err)
	}
	ct, err := crypto.Encrypt(dataKey, []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}
	pubX, _ := crypto.B64Dec(v.cfg.PubX25519)
	pubM, _ := crypto.B64Dec(v.cfg.PubMLKEM768)
	wrapped, err := crypto.EncryptDataKey(dataKey, pubX, pubM)
	if err != nil {
		t.Fatal(err)
	}
	return crypto.B64Enc(ct), crypto.B64Enc(wrapped), dataKey
}

func openBlob(t *testing.T, dataKey []byte, encData string) string {
	t.Helper()
	ct, err := crypto.B64Dec(encData)
	if err != nil {
		t.Fatal(err)
	}
	pt, err := crypto.Decrypt(dataKey, ct)
	if err != nil {
		t.Fatal(err)
	}
	return string(pt)
}

// Editing an entry from the CLI (decrypt → change → re-encrypt) must keep the
// keys the app wrote that the CLI does not model.
func TestUpdateEntryKeepsUnknownKeys(t *testing.T) {
	const stored = `{"username":"octo","password":"old","future_flag":true,
		"custom_fields":[
			{"label":"PIN","value":"1234","type":"password"},
			{"label":"Scan","value":"aGVsbG8=","type":"file","filename":"scan.png","mime_type":"image/png"}]}`

	var (
		v       *Vault
		encData string
		encKey  string
		dataKey []byte
		put     apiclient.UpdateEntryRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/entries/e1":
			_ = json.NewEncoder(w).Encode(apiclient.EntryResponse{
				ID: "e1", Name: "GitHub", Type: "password",
				EncryptedData: encData,
				EntryKey:      &apiclient.EntryKey{UserID: "u1", EncryptedKey: encKey},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/entries/e1":
			_ = json.NewDecoder(r.Body).Decode(&put)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	v = unlockedTestVault(t, srv.URL)
	encData, encKey, dataKey = sealEntry(t, v, stored)

	entry, err := v.GetEntry("e1")
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if got := entry.Data.CustomFields[1]; got.Type != "file" || got.Filename != "scan.png" {
		t.Fatalf("file field not decoded: %+v", got)
	}
	entry.Data.Password = "new"
	entry.Data.Tags, err = EditTags(entry.Data.Tags, false, nil, []string{"work"})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.UpdateEntry("e1", entry.Name, entry.URL, entry.Data, nil); err != nil {
		t.Fatalf("UpdateEntry: %v", err)
	}

	got := openBlob(t, dataKey, put.EncryptedData)
	want := `{"username":"octo","password":"new","future_flag":true,"tags":["work"],
		"custom_fields":[
			{"label":"PIN","value":"1234","type":"password"},
			{"label":"Scan","value":"aGVsbG8=","type":"file","filename":"scan.png","mime_type":"image/png"}]}`
	if !reflect.DeepEqual(canon(t, got), canon(t, want)) {
		t.Fatalf("saved entry lost data\n got: %s\nwant: %s", got, want)
	}
}

// The server answers a create with {"id": ...} only; the returned entry must
// still carry the name ("pwmgr add" used to print "Entry ” saved").
func TestCreateEntryReturnsRequestMetadata(t *testing.T) {
	var post apiclient.CreateEntryRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/entries" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&post)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "new-id"})
	}))
	defer srv.Close()
	v := unlockedTestVault(t, srv.URL)

	folder := "f1"
	data := &EntryData{Username: "octo", Tags: []string{"work", "dev"}}
	entry, err := v.CreateEntry("GitHub", "password", "https://github.com", data, &folder, "", "", nil)
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if entry.ID != "new-id" || entry.Name != "GitHub" || entry.Type != "password" ||
		entry.URL != "https://github.com" || entry.FolderID == nil || *entry.FolderID != "f1" {
		t.Fatalf("returned entry = %+v", entry)
	}

	// The tags travel inside the encrypted blob, readable with our own key.
	wrapped, _ := crypto.B64Dec(post.EntryKeys[0].EncryptedKey)
	dataKey, err := crypto.DecryptDataKey(wrapped, v.privX25519, v.privMLKEM)
	if err != nil {
		t.Fatal(err)
	}
	var saved EntryData
	if err := json.Unmarshal([]byte(openBlob(t, dataKey, post.EncryptedData)), &saved); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(saved.Tags, []string{"work", "dev"}) {
		t.Fatalf("saved tags = %q", saved.Tags)
	}
}

// entryServer serves one sealed entry and returns the raw JSON of each update.
func entryServer(t *testing.T, entryURL, plaintext string) (v *Vault, dataKey []byte, puts *[]map[string]any) {
	t.Helper()
	var (
		encData, encKey string
		got             []map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(apiclient.EntryResponse{
				ID: "e1", Name: "GitHub", URL: entryURL, Type: "password",
				EncryptedData: encData,
				EntryKey:      &apiclient.EntryKey{UserID: "u1", EncryptedKey: encKey},
			})
		case http.MethodPut:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			got = append(got, body)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	v = unlockedTestVault(t, srv.URL)
	encData, encKey, dataKey = sealEntry(t, v, plaintext)
	return v, dataKey, &got
}

func TestUpdateOmitsAnEmptyURLAndReplaceSendsIt(t *testing.T) {
	v, _, puts := entryServer(t, "https://github.com", `{"password":"pw"}`)
	data := &EntryData{Password: "pw"}

	if err := v.UpdateEntry("e1", "GitHub", "", data, nil); err != nil {
		t.Fatal(err)
	}
	if _, sent := (*puts)[0]["url"]; sent {
		t.Errorf("UpdateEntry with an empty url must keep the stored one: %v", (*puts)[0]["url"])
	}

	if err := v.UpdateEntry("e1", "GitHub", "https://new.example", data, nil); err != nil {
		t.Fatal(err)
	}
	if got := (*puts)[1]["url"]; got != "https://new.example" {
		t.Errorf("UpdateEntry url = %v", got)
	}

	if err := v.ReplaceEntry("e1", "GitHub", "", data); err != nil {
		t.Fatal(err)
	}
	if got, sent := (*puts)[2]["url"]; !sent || got != "" {
		t.Errorf(`ReplaceEntry with an empty url must send "", got %v (sent %v)`, got, sent)
	}
	if _, sent := (*puts)[2]["match_patterns"]; sent {
		t.Error("ReplaceEntry must leave the match patterns alone")
	}

	if err := v.MoveEntry("e1", nil); err != nil {
		t.Fatal(err)
	}
	if _, sent := (*puts)[3]["url"]; sent {
		t.Error("MoveEntry must not touch the URL")
	}
}

func TestReadingAnEntryIgnoresAStrayTOTPSecret(t *testing.T) {
	v, _, _ := entryServer(t, "", `{"password":"hunter2","totp_secret":"hunter2"}`)
	entry, err := v.GetEntry("e1")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Data.TOTPSecret != "" || entry.Data.Password != "hunter2" {
		t.Fatalf("data = %+v", entry.Data)
	}
}

func TestReadingAnEntryKeepsARealTOTPSecret(t *testing.T) {
	v, _, _ := entryServer(t, "", `{"password":"hunter2","totp_secret":"JBSWY3DPEHPK3PXP"}`)
	entry, err := v.GetEntry("e1")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Data.TOTPSecret != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("a real secret must stay, got %q", entry.Data.TOTPSecret)
	}
}

// After a password change the old value would pass for a real secret, so saving
// must not carry it along.
func TestSavingDoesNotWriteAStrayTOTPSecretBack(t *testing.T) {
	v, dataKey, puts := entryServer(t, "", `{"password":"old","totp_secret":"old","future_flag":true}`)
	entry, err := v.GetEntry("e1")
	if err != nil {
		t.Fatal(err)
	}
	entry.Data.Password = "new"
	if err := v.UpdateEntry("e1", entry.Name, "", entry.Data, nil); err != nil {
		t.Fatal(err)
	}
	got := openBlob(t, dataKey, (*puts)[0]["encrypted_data"].(string))
	if want := `{"password":"new","future_flag":true}`; !reflect.DeepEqual(canon(t, got), canon(t, want)) {
		t.Fatalf("saved %s, want %s", got, want)
	}
}

func TestIsStrayTOTPSecret(t *testing.T) {
	for _, tc := range []struct {
		secret, password string
		want             bool
	}{
		{"hunter2", "hunter2", true},
		{"JBSWY3DPEHPK3PXP", "hunter2", false},
		{"JBSWY3DPEHPK3PXP", "", false},
		{"", "", false},
		{"", "hunter2", false},
	} {
		if got := IsStrayTOTPSecret(tc.secret, tc.password); got != tc.want {
			t.Errorf("IsStrayTOTPSecret(%q, %q) = %v", tc.secret, tc.password, got)
		}
	}
}

func TestURLHost(t *testing.T) {
	for raw, want := range map[string]string{
		"https://github.com/login":  "github.com",
		"http://GitHub.com:8080/x":  "GitHub.com",
		"github.com/login":          "github.com",
		"  https://a.example  ":     "a.example",
		"androidapp://com.foo.bar":  "com.foo.bar",
		"":                          "",
		"   ":                       "",
		"https://exa mple.com/path": "",
	} {
		if got := URLHost(raw); got != want {
			t.Errorf("URLHost(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestFindDuplicatesReturnsAllMatches(t *testing.T) {
	existing := []EntryRecord{
		{ID: "e1", Name: "GitHub", Data: EntryData{Username: "octo"}},
		{ID: "e2", Name: "github", Data: EntryData{Username: "OCTO"}},
		{ID: "e3", Name: "GitHub", Data: EntryData{Username: "hubot"}},
	}
	if got := FindDuplicates("GitHub", "octo", "password", existing); !slices.Equal(got, []string{"e1", "e2"}) {
		t.Errorf("got %q", got)
	}
	if got := FindDuplicates("GitHub", "nobody", "password", existing); got != nil {
		t.Errorf("got %q", got)
	}
}

func TestFindDuplicatesStaysInTheTypeFamily(t *testing.T) {
	for _, tc := range []struct {
		incoming, stored string
		want             bool
	}{
		{"password", "password", true},
		{"password", "totp", true},
		{"totp", "password", true},
		{"totp", "totp", true},
		{"", "password", true},
		{"password", "", true},
		{"", "totp", true},
		{"totp", "", true},
		{"", "", true},
		{"credit-card", "credit-card", true},
		{"note", "note", true},
		{"api-key", "api-key", true},
		{"future-type", "future-type", true},
		{"credit-card", "password", false},
		{"password", "credit-card", false},
		{"credit-card", "totp", false},
		{"totp", "note", false},
		{"", "note", false},
		{"note", "", false},
		{"credit-card", "identity", false},
		{"api-key", "password", false},
		{"password", "ssh-key", false},
		{"Password", "password", false},
	} {
		existing := []EntryRecord{{ID: "e1", Name: "Visa", Type: tc.stored}}
		got := FindDuplicates("Visa", "", tc.incoming, existing)
		if (len(got) == 1) != tc.want {
			t.Errorf("%q over %q: got %q, want a match: %v", tc.incoming, tc.stored, got, tc.want)
		}
	}
}

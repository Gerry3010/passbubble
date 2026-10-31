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
// still carry the name ("pwmgr add" used to print "Entry '' saved").
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

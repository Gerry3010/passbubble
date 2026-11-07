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
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
	"github.com/Gerry3010/passbubble/cli/pkg/keyring"
)

type adapterEntry struct {
	id, name, entryType, plaintext string
}

// adapterVault serves the entries below and records what would change them.
type adapterVault struct {
	mu      sync.Mutex
	entries []apiclient.EntryResponse
	deleted []string
	created []apiclient.CreateEntryRequest
}

func newAdapter(t *testing.T, entries ...adapterEntry) (*KeyringAdapter, *Vault, *adapterVault) {
	t.Helper()
	s := &adapterVault{}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	v := unlockedTestVault(t, srv.URL)
	for _, e := range entries {
		encData, encKey, _ := sealEntry(t, v, e.plaintext)
		s.entries = append(s.entries, apiclient.EntryResponse{
			ID: e.id, Name: e.name, Type: e.entryType, EncryptedData: encData,
			EntryKey: &apiclient.EntryKey{UserID: "u1", EncryptedKey: encKey},
		})
	}
	return NewKeyringAdapter(v), v, s
}

func (s *adapterVault) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	metadata := func(match func(apiclient.EntryResponse) bool) []apiclient.EntryResponse {
		out := []apiclient.EntryResponse{}
		for _, e := range s.entries {
			if match(e) {
				e.EncryptedData, e.EntryKey = "", nil
				out = append(out, e)
			}
		}
		return out
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/entries/")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/entries":
		_ = json.NewEncoder(w).Encode(metadata(func(apiclient.EntryResponse) bool { return true }))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/entries/search":
		q := r.URL.Query().Get("q")
		_ = json.NewEncoder(w).Encode(metadata(func(e apiclient.EntryResponse) bool { return strings.Contains(e.Name, q) }))
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
		s.created = append(s.created, req)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "new-id"})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func sameNameEntries() []adapterEntry {
	return []adapterEntry{
		{"e1", "GitHub", "password", `{"username":"octo","password":"pw-1"}`},
		{"e2", "GitHub", "password", `{"username":"hubot","password":"pw-2"}`},
		{"e3", "Bank", "password", `{"username":"max","password":"pw-3"}`},
		{"e4", "AWS", "totp", `{"username":"root","totp_secret":"JBSWY3DPEHPK3PXP"}`},
		{"e5", "Twin", "password", `{"username":"same","password":"pw-5"}`},
		{"e6", "Twin", "password", `{"username":"same","password":"pw-6"}`},
	}
}

func TestAdapterGetNeverReturnsAnotherEntrysSecret(t *testing.T) {
	a, _, _ := newAdapter(t, sameNameEntries()...)

	for _, tc := range []struct {
		name, service, username, want string
		wantErr                       bool
	}{
		{"unknown username, two candidates", "GitHub", "", "", true},
		{"known username", "GitHub", "hubot", "pw-2", false},
		{"other known username", "GitHub", "octo", "pw-1", false},
		{"username of nobody", "GitHub", "nobody", "", true},
		{"unique name, unknown username", "Bank", "", "pw-3", false},
		{"exact twins", "Twin", "same", "", true},
		{"totp entry", "AWS", "", "JBSWY3DPEHPK3PXP", false},
		{"no such entry", "Nope", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := a.Get(tc.service, tc.username)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("Get = %q, want %q", got, tc.want)
			}

			entry, err := a.GetEntry(tc.service, tc.username)
			if (err != nil) != tc.wantErr {
				t.Fatalf("GetEntry err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && entry.Password != tc.want {
				t.Fatalf("GetEntry secret = %q, want %q", entry.Password, tc.want)
			}
		})
	}
}

func TestAdapterDeleteNeverGuesses(t *testing.T) {
	for _, tc := range []struct {
		name, service, username string
		want                    []string
		wantErr                 bool
	}{
		{"unknown username, two candidates", "GitHub", "", nil, true},
		{"known username", "GitHub", "hubot", []string{"e2"}, false},
		{"unique name, unknown username", "Bank", "", []string{"e3"}, false},
		{"username of nobody", "GitHub", "nobody", nil, true},
		{"exact twins", "Twin", "same", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, s := newAdapter(t, sameNameEntries()...)
			err := a.Delete(tc.service, tc.username)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !slices.Equal(s.deleted, tc.want) {
				t.Fatalf("deleted %v, want %v", s.deleted, tc.want)
			}
		})
	}
}

// createdBlob decrypts what StoreEntry uploaded.
func createdBlob(t *testing.T, v *Vault, req apiclient.CreateEntryRequest) any {
	t.Helper()
	wrapped, err := crypto.B64Dec(req.EntryKeys[0].EncryptedKey)
	if err != nil {
		t.Fatal(err)
	}
	dataKey, err := crypto.DecryptDataKey(wrapped, v.privX25519, v.privMLKEM)
	if err != nil {
		t.Fatal(err)
	}
	return canon(t, openBlob(t, dataKey, req.EncryptedData))
}

func TestStoreEntryKeepsPasswordAndTOTPSecretApart(t *testing.T) {
	a, v, s := newAdapter(t)

	if err := a.StoreEntry(keyring.Entry{Service: "GitHub", Username: "octo", Password: "hunter2", SecretType: keyring.SecretTypePassword, Notes: "n"}); err != nil {
		t.Fatal(err)
	}
	if err := a.StoreEntry(keyring.Entry{Service: "AWS", Password: "JBSWY3DPEHPK3PXP", SecretType: keyring.SecretTypeTOTP, Issuer: "Amazon", Period: 30, Digits: 6, Algorithm: "SHA1"}); err != nil {
		t.Fatal(err)
	}
	if err := a.StoreEntry(keyring.Entry{Service: "Untyped", Password: "s3cret"}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		`{"username":"octo","password":"hunter2","notes":"n"}`,
		`{"totp_secret":"JBSWY3DPEHPK3PXP","issuer":"Amazon","period":30,"digits":6,"algorithm":"SHA1"}`,
		`{"password":"s3cret"}`,
	}
	wantTypes := []string{"password", "totp", "password"}
	if len(s.created) != len(want) {
		t.Fatalf("created %d entries, want %d", len(s.created), len(want))
	}
	for i, req := range s.created {
		if got := createdBlob(t, v, req); !reflect.DeepEqual(got, canon(t, want[i])) {
			t.Errorf("entry %d\n got: %v\nwant: %s", i, got, want[i])
		}
		if req.Type != wantTypes[i] {
			t.Errorf("entry %d type = %q, want %q", i, req.Type, wantTypes[i])
		}
	}
}

func TestAdapterReportsAmbiguousEntries(t *testing.T) {
	a, _, s := newAdapter(t, sameNameEntries()...)

	if _, err := a.Get("GitHub", ""); !errors.Is(err, keyring.ErrAmbiguous) {
		t.Errorf("Get: %v", err)
	}
	if _, err := a.GetEntry("Twin", "same"); !errors.Is(err, keyring.ErrAmbiguous) {
		t.Errorf("GetEntry: %v", err)
	}
	if err := a.Delete("GitHub", ""); !errors.Is(err, keyring.ErrAmbiguous) {
		t.Errorf("Delete: %v", err)
	}
	if len(s.deleted) != 0 {
		t.Errorf("deleted %v", s.deleted)
	}
}

func TestAdapterRefusesWhileLocked(t *testing.T) {
	a, v, s := newAdapter(t, sameNameEntries()...)
	v.Lock()

	if _, err := a.Get("Bank", ""); err == nil {
		t.Error("Get should fail while locked")
	}
	if err := a.Delete("Bank", ""); err == nil {
		t.Error("Delete should fail while locked")
	}
	if len(s.deleted) != 0 {
		t.Errorf("deleted %v", s.deleted)
	}
}

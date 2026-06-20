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
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
	"golang.org/x/crypto/curve25519"
)

// legacyEntryKey forges a Flutter X25519-only entry key:
// ephPub(32) || AES-256-GCM(rawSharedSecret, dataKey). Mirrors what the Flutter
// app produces, so we can verify the upgrade re-wraps real legacy entries.
func legacyEntryKey(t *testing.T, dataKey, recipPubX []byte) []byte {
	t.Helper()
	ephPriv := make([]byte, 32)
	if _, err := rand.Read(ephPriv); err != nil {
		t.Fatal(err)
	}
	ephPub, err := curve25519.X25519(ephPriv, curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := curve25519.X25519(ephPriv, recipPubX)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := crypto.Encrypt(shared, dataKey)
	if err != nil {
		t.Fatal(err)
	}
	return append(ephPub, enc...)
}

func TestNeedsKeyUpgrade(t *testing.T) {
	placeholder := make([]byte, 32) // X25519-only account stores a 32-byte placeholder
	real := make([]byte, mlkem768PubLen)

	cases := []struct {
		name string
		pub  string
		want bool
	}{
		{"placeholder (X25519-only)", crypto.B64Enc(placeholder), true},
		{"real ml-kem key", crypto.B64Enc(real), false},
		{"invalid base64", "!!!not-base64!!!", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := &Vault{cfg: &config.Config{PubMLKEM768: c.pub}}
			if got := v.NeedsKeyUpgrade(); got != c.want {
				t.Fatalf("NeedsKeyUpgrade() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestUpgradeToHybrid drives the full retrofit against a mock server: it should
// upload a real 1184-byte ML-KEM key (keeping X25519) and re-wrap each entry's
// data key to hybrid such that the new keys still recover the original data key.
func TestUpgradeToHybrid(t *testing.T) {
	const masterPassword = "master-pw"

	// Build an X25519-only account: real X25519 keypair + a placeholder ML-KEM key
	// (the encrypted X25519 private key, exactly as the Flutter app stores it).
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	kdf := &crypto.KDFParams{Salt: salt, Time: 1, Memory: 8192}
	masterKey := crypto.DeriveKey(masterPassword, kdf)

	privX, pubX, err := crypto.GenerateX25519()
	if err != nil {
		t.Fatal(err)
	}
	encX, err := crypto.Encrypt(masterKey, privX)
	if err != nil {
		t.Fatal(err)
	}
	placeholderMLKEMPub := make([]byte, 32) // 32-byte placeholder → not a real ML-KEM key

	// A legacy (X25519-only) entry key: EncryptDataKey falls back to legacy when the
	// ML-KEM pub isn't 1184 bytes.
	dataKey, err := crypto.RandKey()
	if err != nil {
		t.Fatal(err)
	}
	legacyEncKey := legacyEntryKey(t, dataKey, pubX)

	var (
		patchBody apiclient.UpdateKeysRequest
		putBody   apiclient.UpdateEntryRequest
		patchSeen bool
		putSeen   bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/auth/me/keys":
			_ = json.NewDecoder(r.Body).Decode(&patchBody)
			patchSeen = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/entries":
			_ = json.NewEncoder(w).Encode([]apiclient.EntryResponse{{ID: "e1", Name: "Example"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/entries/e1":
			_ = json.NewEncoder(w).Encode(apiclient.EntryResponse{
				ID:   "e1",
				Name: "Example",
				EntryKey: &apiclient.EntryKey{
					UserID:       "u1",
					EncryptedKey: crypto.B64Enc(legacyEncKey),
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/entries/e1":
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			putSeen = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &config.Config{
		ServerURL:       srv.URL,
		UserID:          "u1",
		KDFSalt:         crypto.B64Enc(salt),
		KDFTime:         1,
		KDFMemory:       8192,
		PubX25519:       crypto.B64Enc(pubX),
		PubMLKEM768:     crypto.B64Enc(placeholderMLKEMPub),
		EncPrivX25519:   crypto.B64Enc(encX),
		EncPrivMLKEM768: crypto.B64Enc(encX), // placeholder mirrors Flutter
	}
	v := New(cfg, filepath.Join(t.TempDir(), "config.yaml"))

	res, err := v.UpgradeToHybrid(masterPassword)
	if err != nil {
		t.Fatalf("UpgradeToHybrid: %v", err)
	}
	if res.Rewrapped != 1 || len(res.Failed) != 0 {
		t.Fatalf("expected 1 re-wrapped, 0 failed; got %d / %v", res.Rewrapped, res.Failed)
	}
	if !patchSeen || !putSeen {
		t.Fatalf("expected PATCH and PUT to be called (patch=%v put=%v)", patchSeen, putSeen)
	}

	// Account is now hybrid.
	if v.NeedsKeyUpgrade() {
		t.Fatal("account should no longer need upgrade")
	}
	newPub, _ := crypto.B64Dec(patchBody.PubMLKEM768)
	if len(newPub) != mlkem768PubLen {
		t.Fatalf("uploaded ml-kem pub should be %d bytes, got %d", mlkem768PubLen, len(newPub))
	}
	if patchBody.PubX25519 != cfg.PubX25519 {
		t.Fatal("X25519 public key must be preserved")
	}

	// The re-wrapped key must decrypt to the original data key under the new keys.
	if len(putBody.EntryKeys) != 1 {
		t.Fatalf("expected 1 re-wrapped entry key, got %d", len(putBody.EntryKeys))
	}
	newEncKey, err := crypto.B64Dec(putBody.EntryKeys[0].EncryptedKey)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := crypto.DecryptDataKey(newEncKey, v.privX25519, v.privMLKEM)
	if err != nil {
		t.Fatalf("decrypt re-wrapped key: %v", err)
	}
	if !bytes.Equal(recovered, dataKey) {
		t.Fatal("re-wrapped key did not recover the original data key")
	}
}

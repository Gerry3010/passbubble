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

package crypto_test

import (
	"bytes"
	"testing"

	"github.com/Gerry3010/passbubble/backend/pkg/crypto"
)

func TestAESGCMRoundtrip(t *testing.T) {
	key, err := crypto.RandKey()
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("Hello, Passbubble!")
	ct, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	got, err := crypto.Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("want %q, got %q", plaintext, got)
	}
}

func TestAESGCMTampering(t *testing.T) {
	key, _ := crypto.RandKey()
	ct, _ := crypto.Encrypt(key, []byte("secret"))
	ct[len(ct)-1] ^= 0xff // flip a bit
	if _, err := crypto.Decrypt(key, ct); err == nil {
		t.Fatal("expected error on tampered ciphertext")
	}
}

func TestArgon2KDF(t *testing.T) {
	params, err := crypto.NewKDFParams()
	if err != nil {
		t.Fatal(err)
	}
	k1 := crypto.DeriveKey("my-password", params)
	k2 := crypto.DeriveKey("my-password", params)
	if !bytes.Equal(k1, k2) {
		t.Fatal("KDF is not deterministic with same salt")
	}
	k3 := crypto.DeriveKey("wrong-password", params)
	if bytes.Equal(k1, k3) {
		t.Fatal("different passwords produced same key")
	}
}

func TestHybridKEMRoundtrip(t *testing.T) {
	// Generate recipient keys
	recipPrivX25519, recipPubX25519, err := crypto.GenerateX25519()
	if err != nil {
		t.Fatal("GenerateX25519:", err)
	}
	recipPrivMLKEM, recipPubMLKEM, err := crypto.GenerateMLKEM768()
	if err != nil {
		t.Fatal("GenerateMLKEM768:", err)
	}

	// Generate a random data key
	dataKey, err := crypto.RandKey()
	if err != nil {
		t.Fatal("RandKey:", err)
	}

	// Encrypt data key for recipient
	encKey, err := crypto.EncryptDataKey(dataKey, recipPubX25519, recipPubMLKEM)
	if err != nil {
		t.Fatal("EncryptDataKey:", err)
	}

	// Decrypt by recipient
	got, err := crypto.DecryptDataKey(encKey, recipPrivX25519, recipPrivMLKEM)
	if err != nil {
		t.Fatal("DecryptDataKey:", err)
	}
	if !bytes.Equal(got, dataKey) {
		t.Fatalf("decrypted key does not match original")
	}
}

func TestDecryptDataKeyLegacyX25519Only(t *testing.T) {
	// A Flutter X25519-only account: a real X25519 keypair, no ML-KEM key.
	privX, pubX, err := crypto.GenerateX25519()
	if err != nil {
		t.Fatal("GenerateX25519:", err)
	}
	dataKey, _ := crypto.RandKey()

	encKey, err := crypto.EncryptDataKeyX25519Only(dataKey, pubX)
	if err != nil {
		t.Fatal("EncryptDataKeyX25519Only:", err)
	}
	// Legacy blobs are far shorter than a hybrid key.
	if len(encKey) >= 32+1088 {
		t.Fatalf("legacy key unexpectedly long: %d", len(encKey))
	}

	// DecryptDataKey must auto-detect the legacy format (privMLKEM is irrelevant).
	got, err := crypto.DecryptDataKey(encKey, privX, nil)
	if err != nil {
		t.Fatal("DecryptDataKey (legacy):", err)
	}
	if !bytes.Equal(got, dataKey) {
		t.Fatal("legacy decrypted key does not match original")
	}
}

func TestEncryptDataKeyFallsBackWithoutMLKEM(t *testing.T) {
	privX, pubX, _ := crypto.GenerateX25519()
	dataKey, _ := crypto.RandKey()

	// A 32-byte placeholder (what the Flutter app stores) is not a valid ML-KEM
	// key, so EncryptDataKey must fall back to the legacy X25519-only format.
	placeholder := make([]byte, 32)
	encKey, err := crypto.EncryptDataKey(dataKey, pubX, placeholder)
	if err != nil {
		t.Fatal("EncryptDataKey (fallback):", err)
	}
	if len(encKey) >= 32+1088 {
		t.Fatalf("expected legacy-length key, got %d", len(encKey))
	}
	got, err := crypto.DecryptDataKey(encKey, privX, nil)
	if err != nil {
		t.Fatal("DecryptDataKey:", err)
	}
	if !bytes.Equal(got, dataKey) {
		t.Fatal("fallback decrypted key does not match original")
	}
}

func TestHybridKEMWrongKey(t *testing.T) {
	_, recipPubX25519, _ := crypto.GenerateX25519()
	_, recipPubMLKEM, _ := crypto.GenerateMLKEM768()
	wrongPrivX25519, _, _ := crypto.GenerateX25519()
	wrongPrivMLKEM, _, _ := crypto.GenerateMLKEM768()

	dataKey, _ := crypto.RandKey()
	encKey, err := crypto.EncryptDataKey(dataKey, recipPubX25519, recipPubMLKEM)
	if err != nil {
		t.Fatal(err)
	}

	// Decapsulation with wrong private key should either fail or return wrong key
	got, err := crypto.DecryptDataKey(encKey, wrongPrivX25519, wrongPrivMLKEM)
	if err == nil && bytes.Equal(got, dataKey) {
		t.Fatal("wrong key should not decrypt successfully")
	}
}

func TestEncryptPrivateKeyWithMasterKey(t *testing.T) {
	params, _ := crypto.NewKDFParams()
	masterKey := crypto.DeriveKey("supersecret", params)

	privKey := make([]byte, 64)
	for i := range privKey {
		privKey[i] = byte(i)
	}

	enc, err := crypto.Encrypt(masterKey, privKey)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := crypto.Decrypt(masterKey, enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, privKey) {
		t.Fatal("private key encryption roundtrip failed")
	}
}

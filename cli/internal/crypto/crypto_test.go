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

package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key, err := RandKey()
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("hello passbubble")
	ct, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("decrypt mismatch: got %q, want %q", got, plaintext)
	}
}

func TestDecryptTamper(t *testing.T) {
	key, _ := RandKey()
	ct, _ := Encrypt(key, []byte("secret"))
	ct[len(ct)-1] ^= 0xff
	if _, err := Decrypt(key, ct); err == nil {
		t.Fatal("expected error on tampered ciphertext")
	}
}

func TestArgon2Deterministic(t *testing.T) {
	salt := make([]byte, 32)
	params := &KDFParams{Salt: salt, Time: 1, Memory: 1024}
	k1 := DeriveKey("password", params)
	k2 := DeriveKey("password", params)
	if !bytes.Equal(k1, k2) {
		t.Fatal("Argon2id not deterministic")
	}
	k3 := DeriveKey("other", params)
	if bytes.Equal(k1, k3) {
		t.Fatal("different passwords produced same key")
	}
}

func TestHybridKEMRoundtrip(t *testing.T) {
	privX, pubX, err := GenerateX25519()
	if err != nil {
		t.Fatal(err)
	}
	privM, pubM, err := GenerateMLKEM768()
	if err != nil {
		t.Fatal(err)
	}

	dataKey, err := RandKey()
	if err != nil {
		t.Fatal(err)
	}

	enc, err := EncryptDataKey(dataKey, pubX, pubM)
	if err != nil {
		t.Fatal(err)
	}

	got, err := DecryptDataKey(enc, privX, privM)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, dataKey) {
		t.Fatal("hybrid KEM round-trip failed")
	}
}

func TestHybridKEMWrongKey(t *testing.T) {
	_, pubX, _ := GenerateX25519()
	_, pubM, _ := GenerateMLKEM768()
	dataKey, _ := RandKey()
	enc, _ := EncryptDataKey(dataKey, pubX, pubM)

	// Use a different private key
	privX2, _, _ := GenerateX25519()
	privM2, _, _ := GenerateMLKEM768()
	if _, err := DecryptDataKey(enc, privX2, privM2); err == nil {
		t.Fatal("expected error with wrong private key")
	}
}

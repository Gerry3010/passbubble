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

// gen-test-vectors generates interop test vectors for the TypeScript shared-ts
// crypto library, produced by the authoritative Go implementation.
//
// Usage:
//
//	cd cli && go run ./cmd/gen-test-vectors -out ../packages/shared-ts/testdata/vectors.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Gerry3010/passbubble/cli/internal/crypto"
	gox25519 "golang.org/x/crypto/curve25519"
)

type argon2Vec struct {
	Password string `json:"password"`
	Salt     string `json:"salt"`
	Time     uint32 `json:"time"`
	Memory   uint32 `json:"memory"`
	Expected string `json:"expected"`
}

type aesGCMVec struct {
	Key        string `json:"key"`
	Plaintext  string `json:"plaintext"`
	Ciphertext string `json:"ciphertext"`
}

type x25519Vec struct {
	AlicePriv    string `json:"alice_priv"`
	BobPub       string `json:"bob_pub"`
	SharedSecret string `json:"shared_secret"`
}

type hybridKEMVec struct {
	RecipientX25519Pub  string `json:"recipient_x25519_pub"`
	RecipientMLKEMPub   string `json:"recipient_mlkem_pub"`
	RecipientX25519Priv string `json:"recipient_x25519_priv"`
	RecipientMLKEMPriv  string `json:"recipient_mlkem_priv"`
	DataKey             string `json:"data_key"`
	EncryptedKey        string `json:"encrypted_key"`
}

type fullEntryVec struct {
	MasterPassword     string `json:"master_password"`
	KDFSalt            string `json:"kdf_salt"`
	KDFTime            uint32 `json:"kdf_time"`
	KDFMemory          uint32 `json:"kdf_memory"`
	EncPrivX25519      string `json:"enc_priv_x25519"`
	EncPrivMLKEM       string `json:"enc_priv_mlkem"`
	PubX25519          string `json:"pub_x25519"`
	PubMLKEM           string `json:"pub_mlkem"`
	EntryEncryptedData string `json:"entry_encrypted_data"`
	EntryEncryptedKey  string `json:"entry_encrypted_key"`
	ExpectedUsername   string `json:"expected_username"`
	ExpectedPassword   string `json:"expected_password"`
}

type vectors struct {
	Argon2    argon2Vec    `json:"argon2"`
	AESGCM    aesGCMVec    `json:"aes_gcm"`
	X25519    x25519Vec    `json:"x25519"`
	HybridKEM hybridKEMVec `json:"hybrid_kem"`
	FullEntry fullEntryVec `json:"full_entry"`
}

func mustEncrypt(key, pt []byte) []byte {
	ct, err := crypto.Encrypt(key, pt)
	if err != nil {
		panic(fmt.Sprintf("encrypt: %v", err))
	}
	return ct
}

func run(outPath string) error {
	var v vectors

	// ─── Argon2id ────────────────────────────────────────────────────────────
	argon2Salt := make([]byte, 32)
	for i := range argon2Salt {
		argon2Salt[i] = 0xAA
	}
	masterPwd := "correct-horse-battery-staple"
	kdfParams := &crypto.KDFParams{Salt: argon2Salt, Time: 3, Memory: 64 * 1024}
	masterKey := crypto.DeriveKey(masterPwd, kdfParams)

	v.Argon2 = argon2Vec{
		Password: masterPwd,
		Salt:     crypto.B64Enc(argon2Salt),
		Time:     3,
		Memory:   64 * 1024,
		Expected: crypto.B64Enc(masterKey),
	}

	// ─── AES-GCM ─────────────────────────────────────────────────────────────
	aesPlaintext := []byte(`{"username":"alice","password":"s3cr3t!"}`)
	aesCT := mustEncrypt(masterKey, aesPlaintext)
	v.AESGCM = aesGCMVec{
		Key:        crypto.B64Enc(masterKey),
		Plaintext:  crypto.B64Enc(aesPlaintext),
		Ciphertext: crypto.B64Enc(aesCT),
	}

	// ─── X25519 ──────────────────────────────────────────────────────────────
	alicePriv, _, err := crypto.GenerateX25519()
	if err != nil {
		return fmt.Errorf("gen alice x25519: %w", err)
	}
	_, bobPub, err := crypto.GenerateX25519()
	if err != nil {
		return fmt.Errorf("gen bob x25519: %w", err)
	}
	ss, err := gox25519.X25519(alicePriv, bobPub)
	if err != nil {
		return fmt.Errorf("x25519 dh: %w", err)
	}
	v.X25519 = x25519Vec{
		AlicePriv:    crypto.B64Enc(alicePriv),
		BobPub:       crypto.B64Enc(bobPub),
		SharedSecret: crypto.B64Enc(ss),
	}

	// ─── Hybrid KEM ──────────────────────────────────────────────────────────
	recipPrivX, recipPubX, err := crypto.GenerateX25519()
	if err != nil {
		return fmt.Errorf("gen recip x25519: %w", err)
	}
	recipPrivM, recipPubM, err := crypto.GenerateMLKEM768()
	if err != nil {
		return fmt.Errorf("gen recip mlkem: %w", err)
	}
	// Use a copy of the derived key as the data key (deterministic)
	dataKey := make([]byte, 32)
	copy(dataKey, masterKey)
	encKey, err := crypto.EncryptDataKey(dataKey, recipPubX, recipPubM)
	if err != nil {
		return fmt.Errorf("encrypt data key: %w", err)
	}
	v.HybridKEM = hybridKEMVec{
		RecipientX25519Pub:  crypto.B64Enc(recipPubX),
		RecipientMLKEMPub:   crypto.B64Enc(recipPubM),
		RecipientX25519Priv: crypto.B64Enc(recipPrivX),
		RecipientMLKEMPriv:  crypto.B64Enc(recipPrivM),
		DataKey:             crypto.B64Enc(dataKey),
		EncryptedKey:        crypto.B64Enc(encKey),
	}

	// ─── Full entry ──────────────────────────────────────────────────────────
	userPrivX, userPubX, err := crypto.GenerateX25519()
	if err != nil {
		return fmt.Errorf("gen user x25519: %w", err)
	}
	userPrivM, userPubM, err := crypto.GenerateMLKEM768()
	if err != nil {
		return fmt.Errorf("gen user mlkem: %w", err)
	}
	encPrivX := mustEncrypt(masterKey, userPrivX)
	encPrivM := mustEncrypt(masterKey, userPrivM)

	entryUsername := "testuser@example.com"
	entryPassword := "MySecretPassword123!"
	entryDataKey, err := crypto.RandKey()
	if err != nil {
		return fmt.Errorf("rand key: %w", err)
	}
	entryPlaintext, err := json.Marshal(map[string]string{
		"username": entryUsername,
		"password": entryPassword,
	})
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	entryEncData := mustEncrypt(entryDataKey, entryPlaintext)
	entryEncKey, err := crypto.EncryptDataKey(entryDataKey, userPubX, userPubM)
	if err != nil {
		return fmt.Errorf("encrypt entry key: %w", err)
	}

	v.FullEntry = fullEntryVec{
		MasterPassword:     masterPwd,
		KDFSalt:            crypto.B64Enc(argon2Salt),
		KDFTime:            3,
		KDFMemory:          64 * 1024,
		EncPrivX25519:      crypto.B64Enc(encPrivX),
		EncPrivMLKEM:       crypto.B64Enc(encPrivM),
		PubX25519:          crypto.B64Enc(userPubX),
		PubMLKEM:           crypto.B64Enc(userPubM),
		EntryEncryptedData: crypto.B64Enc(entryEncData),
		EntryEncryptedKey:  crypto.B64Enc(entryEncKey),
		ExpectedUsername:   entryUsername,
		ExpectedPassword:   entryPassword,
	}

	// ─── Write output ─────────────────────────────────────────────────────────
	if err := os.MkdirAll(dirOf(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %d bytes to %s\n", len(data), outPath)
	return nil
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}

func main() {
	out := flag.String("out", "testdata/vectors.json", "output path for vectors JSON")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

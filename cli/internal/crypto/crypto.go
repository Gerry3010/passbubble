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

// Package crypto provides the client-side E2E encryption operations.
// Uses the same algorithms as the backend: X25519 + ML-KEM-768 hybrid KEM,
// AES-256-GCM for symmetric encryption, Argon2id for key derivation.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/cloudflare/circl/kem/mlkem/mlkem768"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"

	gocrypto "crypto/sha256"
)

// KDFParams holds Argon2id parameters. They are stored with the user account.
type KDFParams struct {
	Salt   []byte
	Time   uint32
	Memory uint32
}

// NewKDFParams generates a fresh 32-byte random salt with default parameters.
func NewKDFParams() (*KDFParams, error) {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return &KDFParams{Salt: salt, Time: 3, Memory: 64 * 1024}, nil
}

// DeriveKey derives a 32-byte AES key from a master password using Argon2id.
func DeriveKey(password string, p *KDFParams) []byte {
	return argon2.IDKey([]byte(password), p.Salt, p.Time, p.Memory, 4, 32)
}

// Encrypt encrypts plaintext with AES-256-GCM. Returns nonce||ciphertext.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nonce, nonce, plaintext, nil)
	return ct, nil
}

// Decrypt decrypts nonce||ciphertext produced by Encrypt.
func Decrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, data[:ns], data[ns:], nil)
}

// GenerateX25519 generates a fresh X25519 key pair.
func GenerateX25519() (priv, pub []byte, err error) {
	priv = make([]byte, 32)
	if _, err = rand.Read(priv); err != nil {
		return
	}
	pub, err = curve25519.X25519(priv, curve25519.Basepoint)
	return
}

// GenerateMLKEM768 generates a fresh ML-KEM-768 key pair.
func GenerateMLKEM768() (privBytes, pubBytes []byte, err error) {
	pub, priv, err := mlkem768.Scheme().GenerateKeyPair()
	if err != nil {
		return nil, nil, err
	}
	pubBytes, err = pub.MarshalBinary()
	if err != nil {
		return
	}
	privBytes, err = priv.MarshalBinary()
	return
}

// EncryptDataKey encrypts a 32-byte data key for a recipient using hybrid KEM.
// Wire format: ephemeral_x25519_pub(32) || mlkem768_ct(CiphertextSize) || nonce||ciphertext
func EncryptDataKey(dataKey, recipX25519Pub, recipMLKEMPub []byte) ([]byte, error) {
	// X25519 ECDH
	ephPriv := make([]byte, 32)
	if _, err := rand.Read(ephPriv); err != nil {
		return nil, err
	}
	ephPub, err := curve25519.X25519(ephPriv, curve25519.Basepoint)
	if err != nil {
		return nil, err
	}
	shared25519, err := curve25519.X25519(ephPriv, recipX25519Pub)
	if err != nil {
		return nil, err
	}

	// ML-KEM-768
	pk, err := mlkem768.Scheme().UnmarshalBinaryPublicKey(recipMLKEMPub)
	if err != nil {
		return nil, fmt.Errorf("unmarshal mlkem pub: %w", err)
	}
	ct, sharedPQ, err := mlkem768.Scheme().Encapsulate(pk)
	if err != nil {
		return nil, fmt.Errorf("mlkem encapsulate: %w", err)
	}

	// Combine secrets
	combined := hybridKDF(shared25519, sharedPQ)

	// Encrypt data key
	encKey, err := Encrypt(combined, dataKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt data key: %w", err)
	}

	// Wire: ephPub(32) || mlkem_ct || encKey
	result := make([]byte, 0, 32+len(ct)+len(encKey))
	result = append(result, ephPub...)
	result = append(result, ct...)
	result = append(result, encKey...)
	return result, nil
}

// DecryptDataKey decrypts a data key previously encrypted with EncryptDataKey.
func DecryptDataKey(encKey, privX25519, privMLKEM []byte) ([]byte, error) {
	ctSize := mlkem768.Scheme().CiphertextSize()
	if len(encKey) < 32+ctSize {
		return nil, fmt.Errorf("encrypted key too short")
	}

	ephPub := encKey[:32]
	mlkemCT := encKey[32 : 32+ctSize]
	remainder := encKey[32+ctSize:]

	// X25519 ECDH
	shared25519, err := curve25519.X25519(privX25519, ephPub)
	if err != nil {
		return nil, fmt.Errorf("x25519: %w", err)
	}

	// ML-KEM-768
	sk, err := mlkem768.Scheme().UnmarshalBinaryPrivateKey(privMLKEM)
	if err != nil {
		return nil, fmt.Errorf("unmarshal mlkem priv: %w", err)
	}
	sharedPQ, err := mlkem768.Scheme().Decapsulate(sk, mlkemCT)
	if err != nil {
		return nil, fmt.Errorf("mlkem decapsulate: %w", err)
	}

	combined := hybridKDF(shared25519, sharedPQ)
	return Decrypt(combined, remainder)
}

// RandKey generates a random 32-byte key.
func RandKey() ([]byte, error) {
	k := make([]byte, 32)
	_, err := rand.Read(k)
	return k, err
}

// B64Enc encodes bytes as standard base64.
func B64Enc(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// B64Dec decodes standard base64.
func B64Dec(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }

func hybridKDF(classical, pq []byte) []byte {
	combined := append(classical, pq...)
	r := hkdf.New(gocrypto.New, combined, nil, []byte("passbubble-hybrid-kem-v1"))
	key := make([]byte, 32)
	io.ReadFull(r, key)
	return key
}

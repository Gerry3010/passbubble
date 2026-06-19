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

// Package crypto implements E2E encryption for Passbubble.
//
// Encryption scheme:
//   - Key derivation:  Argon2id (master password → master key)
//   - Key exchange:    Hybrid KEM = X25519 ECDH + ML-KEM-768 (post-quantum, NIST FIPS 203)
//   - Symmetric enc:   AES-256-GCM
//   - KDF for hybrid:  HKDF-SHA256
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/cloudflare/circl/dh/x25519"
	mlkem "github.com/cloudflare/circl/kem/mlkem/mlkem768"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"
)

const (
	// Argon2DefaultTime and Argon2DefaultMemory are the default Argon2id cost
	// parameters used wherever a password is hashed (master key derivation,
	// share-link password protection, ...). Exported so callers building a
	// KDFParams from stored Time/Memory values use the same defaults.
	Argon2DefaultTime   uint32 = 3
	Argon2DefaultMemory uint32 = 64 * 1024
	argon2Threads       uint8  = 4
	argon2KeyLen        uint32 = 32
	SaltLen                    = 32
)

// KDFParams holds Argon2id parameters stored per user.
type KDFParams struct {
	Salt   []byte
	Time   uint32
	Memory uint32
}

// NewKDFParams generates random salt with default Argon2id parameters.
func NewKDFParams() (*KDFParams, error) {
	salt := make([]byte, SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return &KDFParams{Salt: salt, Time: Argon2DefaultTime, Memory: Argon2DefaultMemory}, nil
}

// DeriveKey derives a 32-byte key from a password using Argon2id.
func DeriveKey(password string, p *KDFParams) []byte {
	return argon2.IDKey([]byte(password), p.Salt, p.Time, p.Memory, argon2Threads, argon2KeyLen)
}

// ─── AES-256-GCM ──────────────────────────────────────────────────────────

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
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
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
		return nil, errors.New("crypto: ciphertext too short")
	}
	return gcm.Open(nil, data[:ns], data[ns:], nil)
}

// ─── Key Generation ───────────────────────────────────────────────────────

// GenerateX25519 generates a new X25519 key pair, returning (private, public) bytes.
func GenerateX25519() (priv []byte, pub []byte, err error) {
	var privKey x25519.Key
	if _, err := rand.Read(privKey[:]); err != nil {
		return nil, nil, err
	}
	var pubKey x25519.Key
	x25519.KeyGen(&pubKey, &privKey)
	return privKey[:], pubKey[:], nil
}

// GenerateMLKEM768 generates a new ML-KEM-768 key pair, returning (private, public) bytes.
func GenerateMLKEM768() (priv []byte, pub []byte, err error) {
	pubKey, privKey, err := mlkem.GenerateKeyPair(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	privBytes, err := privKey.MarshalBinary()
	if err != nil {
		return nil, nil, err
	}
	pubBytes, err := pubKey.MarshalBinary()
	if err != nil {
		return nil, nil, err
	}
	return privBytes, pubBytes, nil
}

// ─── Hybrid KEM (X25519 + ML-KEM-768) ────────────────────────────────────
//
// Wire format for EncryptedKey:
//   [ephemeral_x25519_pub(32)] [mlkem768_ct(CiphertextSize)] [nonce(12)+ciphertext]

// EncryptDataKey encrypts a 32-byte data key for a recipient using hybrid KEM.
func EncryptDataKey(dataKey, recipX25519Pub, recipMLKEMPub []byte) ([]byte, error) {
	// X25519 ephemeral key exchange
	var ephemPriv, ephemPub x25519.Key
	if _, err := rand.Read(ephemPriv[:]); err != nil {
		return nil, fmt.Errorf("gen ephemeral x25519: %w", err)
	}
	x25519.KeyGen(&ephemPub, &ephemPriv)

	var recipPub x25519.Key
	copy(recipPub[:], recipX25519Pub)
	var sharedX25519 x25519.Key
	if ok := x25519.Shared(&sharedX25519, &ephemPriv, &recipPub); !ok {
		return nil, errors.New("crypto: x25519 shared secret is zero (invalid public key)")
	}

	// ML-KEM-768 encapsulation
	pubKey, err := mlkem.Scheme().UnmarshalBinaryPublicKey(recipMLKEMPub)
	if err != nil {
		return nil, fmt.Errorf("parse mlkem768 pub key: %w", err)
	}
	ct, sharedMLKEM, err := mlkem.Scheme().Encapsulate(pubKey)
	if err != nil {
		return nil, fmt.Errorf("mlkem768 encapsulate: %w", err)
	}

	// Combine secrets via HKDF-SHA256
	combined := hybridKDF(sharedX25519[:], sharedMLKEM)

	// Encrypt data key with combined secret
	encDataKey, err := Encrypt(combined, dataKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt data key: %w", err)
	}

	// Pack: ephemeral_x25519_pub || mlkem768_ct || encrypted_data_key
	out := make([]byte, 0, 32+len(ct)+len(encDataKey))
	out = append(out, ephemPub[:]...)
	out = append(out, ct...)
	out = append(out, encDataKey...)
	return out, nil
}

// DecryptDataKey decrypts an EncryptedKey using the recipient's private keys.
func DecryptDataKey(encKey, privX25519, privMLKEM []byte) ([]byte, error) {
	ctSize := mlkem.Scheme().CiphertextSize()
	const x25519PubLen = 32
	if len(encKey) <= x25519PubLen+ctSize {
		return nil, errors.New("crypto: encrypted key too short")
	}

	ephemPubBytes := encKey[:x25519PubLen]
	mlkemCT := encKey[x25519PubLen : x25519PubLen+ctSize]
	encDataKey := encKey[x25519PubLen+ctSize:]

	// X25519 ECDH
	var ephemPub, privKey x25519.Key
	copy(ephemPub[:], ephemPubBytes)
	copy(privKey[:], privX25519)
	var sharedX25519 x25519.Key
	if ok := x25519.Shared(&sharedX25519, &privKey, &ephemPub); !ok {
		return nil, errors.New("crypto: x25519 shared secret is zero")
	}

	// ML-KEM-768 decapsulation
	sk, err := mlkem.Scheme().UnmarshalBinaryPrivateKey(privMLKEM)
	if err != nil {
		return nil, fmt.Errorf("parse mlkem768 priv key: %w", err)
	}
	sharedMLKEM, err := mlkem.Scheme().Decapsulate(sk, mlkemCT)
	if err != nil {
		return nil, fmt.Errorf("mlkem768 decapsulate: %w", err)
	}

	combined := hybridKDF(sharedX25519[:], sharedMLKEM)
	return Decrypt(combined, encDataKey)
}

func hybridKDF(classical, pq []byte) []byte {
	info := []byte("passbubble-hybrid-kem-v1")
	ikm := append(classical, pq...)
	r := hkdf.New(sha256.New, ikm, nil, info)
	out := make([]byte, 32)
	_, _ = io.ReadFull(r, out)
	return out
}

// ─── Utilities ────────────────────────────────────────────────────────────

// RandKey generates a random 32-byte AES data key.
func RandKey() ([]byte, error) {
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	return k, nil
}

// B64Enc encodes bytes to standard base64 (with padding).
func B64Enc(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// B64Dec decodes standard base64.
func B64Dec(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }

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

package importers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// coseES256 is the COSE algorithm of ECDSA P-256 with SHA-256, the only one
// Passbubble handles (docs/passkeys.md).
const coseES256 = -7

// maxCredentialIDLength is the WebAuthn limit for credential ids.
const maxCredentialIDLength = 1023

// decodeBase64Any decodes base64url or standard base64, with or without
// padding: the readers of docs/passkeys.md accept all of them.
func decodeBase64Any(s string) ([]byte, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	if strings.ContainsAny(s, "+/") {
		return base64.RawStdEncoding.DecodeString(s)
	}
	return base64.RawURLEncoding.DecodeString(s)
}

// encodeBase64URL is the stored form: base64url without padding.
func encodeBase64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// normaliseBase64URL re-encodes base64 of either alphabet as base64url without
// padding.
func normaliseBase64URL(s string) (string, error) {
	b, err := decodeBase64Any(s)
	if err != nil {
		return "", err
	}
	return encodeBase64URL(b), nil
}

// normalisePasskeyPrivateKey reads a P-256 private key, PKCS#8 with or without
// the optional parameters and public key (SEC 1 is tolerated as well), and
// returns it as base64url PKCS#8 in the long form, the 138 bytes WebCrypto
// exports. Go's MarshalPKCS8PrivateKey writes exactly that form.
func normalisePasskeyPrivateKey(encoded string) (string, error) {
	der, err := decodeBase64Any(encoded)
	if err != nil {
		return "", fmt.Errorf("private key is not base64: %w", err)
	}
	var key *ecdsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		k, ok := parsed.(*ecdsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("private key is %T, not ECDSA", parsed)
		}
		key = k
	} else if k, secErr := x509.ParseECPrivateKey(der); secErr == nil {
		key = k
	} else {
		return "", fmt.Errorf("private key is not PKCS#8: %w", err)
	}
	if key.Curve != elliptic.P256() {
		return "", fmt.Errorf("private key is on %s, not P-256", key.Curve.Params().Name)
	}
	long, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	return encodeBase64URL(long), nil
}

// bitwardenCredentialID decodes Bitwarden's credentialId: "b64.<base64url>" for
// the raw bytes, otherwise a GUID that stands for its 16 bytes in the order it
// is written (Bitwarden's guidToRawFormat).
func bitwardenCredentialID(id string) ([]byte, error) {
	id = strings.TrimSpace(id)
	var raw []byte
	if rest, ok := strings.CutPrefix(id, "b64."); ok {
		b, err := decodeBase64Any(rest)
		if err != nil {
			return nil, fmt.Errorf("credential id is not base64: %w", err)
		}
		raw = b
	} else {
		b, err := parseGUID(id)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	if len(raw) == 0 {
		return nil, errors.New("credential id is empty")
	}
	if len(raw) > maxCredentialIDLength {
		return nil, fmt.Errorf("credential id is %d bytes, more than %d", len(raw), maxCredentialIDLength)
	}
	return raw, nil
}

// parseGUID reads xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx into its 16 bytes.
func parseGUID(s string) ([]byte, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 ||
		len(parts[3]) != 4 || len(parts[4]) != 12 {
		return nil, fmt.Errorf("credential id %q is neither a GUID nor b64.<base64url>", s)
	}
	b, err := hex.DecodeString(strings.Join(parts, ""))
	if err != nil {
		return nil, fmt.Errorf("credential id %q is neither a GUID nor b64.<base64url>", s)
	}
	return b, nil
}

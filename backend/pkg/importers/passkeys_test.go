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
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"strings"
	"testing"
)

var (
	oidECPublicKey = asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1}
	oidP256        = asn1.ObjectIdentifier{1, 2, 840, 10045, 3, 1, 7}
)

// The PKCS#8 and SEC 1 layouts, written out so the tests can build the short
// forms other exporters write.
type testPKCS8 struct {
	Version    int
	Algo       pkix.AlgorithmIdentifier
	PrivateKey []byte
}

type testECPrivateKey struct {
	Version       int
	PrivateKey    []byte
	NamedCurveOID asn1.ObjectIdentifier `asn1:"optional,explicit,tag:0"`
	PublicKey     asn1.BitString        `asn1:"optional,explicit,tag:1"`
}

func newP256Key(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// pkcs8 wraps the SEC 1 structure the way the variant asks for.
func pkcs8(t *testing.T, k *ecdsa.PrivateKey, withPublicKey, withInnerCurve bool) []byte {
	t.Helper()
	d, err := k.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	inner := testECPrivateKey{Version: 1, PrivateKey: d}
	if withInnerCurve {
		inner.NamedCurveOID = oidP256
	}
	if withPublicKey {
		pub, err := k.PublicKey.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		inner.PublicKey = asn1.BitString{Bytes: pub, BitLength: 8 * len(pub)}
	}
	innerDER, err := asn1.Marshal(inner)
	if err != nil {
		t.Fatal(err)
	}
	curve, err := asn1.Marshal(oidP256)
	if err != nil {
		t.Fatal(err)
	}
	der, err := asn1.Marshal(testPKCS8{
		Algo:       pkix.AlgorithmIdentifier{Algorithm: oidECPublicKey, Parameters: asn1.RawValue{FullBytes: curve}},
		PrivateKey: innerDER,
	})
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestNormalisePasskeyPrivateKeyWritesTheLongForm(t *testing.T) {
	k := newP256Key(t)
	long, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	if len(long) != 138 {
		t.Fatalf("Go's PKCS#8 is %d bytes, the contract wants the 138-byte form", len(long))
	}
	sec1, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	for name, der := range map[string][]byte{
		"long form":                     long,
		"without public key":            pkcs8(t, k, false, false),
		"with inner curve and key":      pkcs8(t, k, true, true),
		"with inner curve, without key": pkcs8(t, k, false, true),
		"SEC 1":                         sec1,
	} {
		for enc, encode := range map[string]func([]byte) string{
			"base64url":     base64.RawURLEncoding.EncodeToString,
			"padded std":    base64.StdEncoding.EncodeToString,
			"padded url":    base64.URLEncoding.EncodeToString,
			"unpadded std":  base64.RawStdEncoding.EncodeToString,
			"with newlines": func(b []byte) string { return "\n" + base64.RawURLEncoding.EncodeToString(b) + "\n" },
		} {
			t.Run(name+"/"+enc, func(t *testing.T) {
				got, err := normalisePasskeyPrivateKey(encode(der))
				if err != nil {
					t.Fatal(err)
				}
				if strings.ContainsAny(got, "+/=") {
					t.Errorf("not base64url without padding: %s", got)
				}
				raw, err := base64.RawURLEncoding.DecodeString(got)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(raw, long) {
					t.Errorf("got %x\nwant %x", raw, long)
				}
			})
		}
	}
}

func TestNormalisePasskeyPrivateKeyRefusesOtherKeys(t *testing.T) {
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p384DER, err := x509.MarshalPKCS8PrivateKey(p384)
	if err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]string{
		"P-384":     base64.RawURLEncoding.EncodeToString(p384DER),
		"not DER":   base64.RawURLEncoding.EncodeToString([]byte("hello")),
		"not b64":   "***",
		"empty":     "",
		"truncated": base64.RawURLEncoding.EncodeToString(pkcs8(t, newP256Key(t), true, false)[:60]),
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := normalisePasskeyPrivateKey(in); err == nil {
				t.Fatalf("accepted: %s", got)
			}
		})
	}
}

func TestBitwardenCredentialID(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []byte
	}{
		{"b64.q2D0bQ2u3E7XoWlS1yZbVw", []byte{0xab, 0x60, 0xf4, 0x6d, 0x0d, 0xae, 0xdc, 0x4e, 0xd7, 0xa1, 0x69, 0x52, 0xd7, 0x26, 0x5b, 0x57}},
		{"b64.q2D0bQ2u3E7XoWlS1yZbVw==", []byte{0xab, 0x60, 0xf4, 0x6d, 0x0d, 0xae, 0xdc, 0x4e, 0xd7, 0xa1, 0x69, 0x52, 0xd7, 0x26, 0x5b, 0x57}},
		{"8c1b4f2e-0a3d-4b5c-9e6f-7a8b9c0d1e2f", []byte{0x8c, 0x1b, 0x4f, 0x2e, 0x0a, 0x3d, 0x4b, 0x5c, 0x9e, 0x6f, 0x7a, 0x8b, 0x9c, 0x0d, 0x1e, 0x2f}},
		{"8C1B4F2E-0A3D-4B5C-9E6F-7A8B9C0D1E2F", []byte{0x8c, 0x1b, 0x4f, 0x2e, 0x0a, 0x3d, 0x4b, 0x5c, 0x9e, 0x6f, 0x7a, 0x8b, 0x9c, 0x0d, 0x1e, 0x2f}},
	} {
		got, err := bitwardenCredentialID(tc.in)
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if !bytes.Equal(got, tc.want) {
			t.Errorf("%s: got %x, want %x", tc.in, got, tc.want)
		}
	}
	tooLong := "b64." + base64.RawURLEncoding.EncodeToString(make([]byte, maxCredentialIDLength+1))
	for _, bad := range []string{"", "b64.", "not-a-guid", "8c1b4f2e0a3d4b5c9e6f7a8b9c0d1e2f", "zzzzzzzz-0a3d-4b5c-9e6f-7a8b9c0d1e2f", "b64.***", tooLong} {
		if got, err := bitwardenCredentialID(bad); err == nil {
			t.Errorf("%q accepted as %x", bad, got)
		}
	}
}

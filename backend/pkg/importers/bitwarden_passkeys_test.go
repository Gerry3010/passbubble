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

package importers_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Gerry3010/passbubble/backend/pkg/exporters"
	"github.com/Gerry3010/passbubble/backend/pkg/importers"
)

// p256PKCS8 returns a fresh P-256 key as Go writes PKCS#8: the long form.
func p256PKCS8(t *testing.T) []byte {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// bitwardenPasskeyItem is a login as Bitwarden exports one with passkeys.
func bitwardenPasskeyItem(t *testing.T, creds ...map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"items": []any{map[string]any{
			"type": 1,
			"name": "Example",
			"login": map[string]any{
				"username":         "alice@example.com",
				"password":         "",
				"uris":             []any{map[string]any{"uri": "https://example.com"}},
				"fido2Credentials": creds,
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func bitwardenCred(keyValue string) map[string]any {
	return map[string]any{
		"credentialId":    "8c1b4f2e-0a3d-4b5c-9e6f-7a8b9c0d1e2f",
		"keyType":         "public-key",
		"keyAlgorithm":    "ECDSA",
		"keyCurve":        "P-256",
		"keyValue":        keyValue,
		"rpId":            "example.com",
		"userHandle":      "dXNlci0xMjM0",
		"userName":        "alice@example.com",
		"counter":         "17",
		"rpName":          "Example",
		"userDisplayName": "Alice",
		"discoverable":    "true",
		"creationDate":    "2026-11-14T03:00:00.123Z",
	}
}

func TestBitwardenImportReadsPasskeys(t *testing.T) {
	key := p256PKCS8(t)
	guid := bitwardenCred(base64.RawURLEncoding.EncodeToString(key))
	b64 := bitwardenCred(base64.StdEncoding.EncodeToString(key)) // padded standard base64 is read too
	b64["credentialId"] = "b64.q2D0bQ2u3E7XoWlS1yZbVw"
	b64["discoverable"] = "false"
	b64["userHandle"] = "+/+/"
	delete(b64, "rpName")
	delete(b64, "userDisplayName")
	b64["creationDate"] = nil

	res, err := importers.ParseBitwarden(bitwardenPasskeyItem(t, guid, b64))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 1 || res.SkippedPasskeys != 0 || len(res.Warnings) != 0 {
		t.Fatalf("result = %+v", res)
	}
	rec := res.Records[0]
	// A passkey without password keeps the login a password entry.
	if rec.Type != "password" || rec.Password != "" || rec.Username != "alice@example.com" {
		t.Errorf("record = %+v", rec)
	}
	longKey := base64.RawURLEncoding.EncodeToString(key)
	want := []importers.Passkey{
		{
			CredentialID:    "jBtPLgo9S1yeb3qLnA0eLw",
			RPID:            "example.com",
			RPName:          "Example",
			UserHandle:      "dXNlci0xMjM0",
			UserName:        "alice@example.com",
			UserDisplayName: "Alice",
			KeyAlgorithm:    -7,
			PrivateKey:      longKey,
			SignCount:       0, // the counter is dropped
			Discoverable:    true,
			CreatedAt:       "2026-11-14T03:00:00Z",
		},
		{
			CredentialID: "q2D0bQ2u3E7XoWlS1yZbVw",
			RPID:         "example.com",
			UserHandle:   "-_-_",
			UserName:     "alice@example.com",
			KeyAlgorithm: -7,
			PrivateKey:   longKey,
		},
	}
	if !reflect.DeepEqual(rec.Passkeys, want) {
		t.Fatalf("passkeys\n got: %+v\nwant: %+v", rec.Passkeys, want)
	}
}

func TestBitwardenImportSkipsAndCountsWhatItCannotUse(t *testing.T) {
	key := base64.RawURLEncoding.EncodeToString(p256PKCS8(t))
	rsa := bitwardenCred(key)
	rsa["keyAlgorithm"], rsa["keyCurve"] = "RSA", ""
	p384 := bitwardenCred(key)
	p384["keyCurve"] = "P-384"
	badKey := bitwardenCred(base64.RawURLEncoding.EncodeToString([]byte("not a key")))
	badID := bitwardenCred(key)
	badID["credentialId"] = "not-a-guid"
	noRP := bitwardenCred(key)
	noRP["rpId"] = ""
	good := bitwardenCred(key)

	res, err := importers.ParseBitwarden(bitwardenPasskeyItem(t, rsa, p384, badKey, badID, noRP, good))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 1 || len(res.Records[0].Passkeys) != 1 {
		t.Fatalf("the entry and its good passkey must survive: %+v", res.Records)
	}
	if res.SkippedPasskeys != 5 || len(res.Warnings) != 5 || res.Skipped != 0 {
		t.Fatalf("skipped %d passkeys, %d records, warnings %q", res.SkippedPasskeys, res.Skipped, res.Warnings)
	}
	for _, w := range res.Warnings {
		if !strings.HasPrefix(w, "Example: skipped a passkey") {
			t.Errorf("warning %q should name the entry", w)
		}
	}
	if !strings.Contains(res.Warnings[0], "RSA") || !strings.Contains(res.Warnings[1], "P-384") {
		t.Errorf("warnings should say why: %q", res.Warnings)
	}
}

func TestBitwardenImportOfAPasskeyWithOnlyATOTPSecretStaysALogin(t *testing.T) {
	data := bitwardenPasskeyItem(t, bitwardenCred(base64.RawURLEncoding.EncodeToString(p256PKCS8(t))))
	data = []byte(strings.Replace(string(data), `"password":""`, `"password":"","totp":"JBSWY3DPEHPK3PXP"`, 1))
	res, err := importers.ParseBitwarden(data)
	if err != nil {
		t.Fatal(err)
	}
	if rec := res.Records[0]; rec.Type != "password" || rec.TOTPSecret == "" || len(rec.Passkeys) != 1 {
		t.Fatalf("record = %+v", rec)
	}
}

func TestBitwardenImportReadsABooleanDiscoverable(t *testing.T) {
	c := bitwardenCred(base64.RawURLEncoding.EncodeToString(p256PKCS8(t)))
	c["discoverable"] = true
	res, err := importers.ParseBitwarden(bitwardenPasskeyItem(t, c))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Records[0].Passkeys[0].Discoverable {
		t.Fatal("discoverable lost")
	}
}

// Export and import again: the passkey comes back as it was, apart from what
// the format has no place for (the PRF secret).
func TestBitwardenPasskeyRoundTrip(t *testing.T) {
	key := base64.RawURLEncoding.EncodeToString(p256PKCS8(t))
	stored := []exporters.Passkey{
		{
			CredentialID: "q2D0bQ2u3E7XoWlS1yZbVw", RPID: "example.com", RPName: "Example",
			UserHandle: "dXNlci0xMjM0", UserName: "alice@example.com", UserDisplayName: "Alice",
			KeyAlgorithm: -7, PrivateKey: key, Discoverable: true,
			PRFSecret: "3q2-7wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", CreatedAt: "2026-11-14T03:00:00Z",
		},
		{
			CredentialID: "AQID", RPID: "login.example.org", UserHandle: "BAUG", UserName: "bob",
			KeyAlgorithm: -7, PrivateKey: key, Discoverable: false,
		},
	}
	data, err := exporters.ExportBitwarden([]exporters.EntryRecord{{
		Type: "password", Name: "Example", URL: "https://example.com", Username: "alice@example.com",
		Passkeys: stored,
	}}, exporters.BitwardenExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := importers.ParseBitwarden(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 1 || res.SkippedPasskeys != 0 {
		t.Fatalf("result = %+v", res)
	}
	rec := res.Records[0]
	if len(rec.CustomFields) != 0 {
		t.Errorf("passkeys must not travel as fields too: %+v", rec.CustomFields)
	}
	want := []importers.Passkey{
		{
			CredentialID: "q2D0bQ2u3E7XoWlS1yZbVw", RPID: "example.com", RPName: "Example",
			UserHandle: "dXNlci0xMjM0", UserName: "alice@example.com", UserDisplayName: "Alice",
			KeyAlgorithm: -7, PrivateKey: key, Discoverable: true, CreatedAt: "2026-11-14T03:00:00Z",
		},
		{
			CredentialID: "AQID", RPID: "login.example.org", UserHandle: "BAUG", UserName: "bob",
			KeyAlgorithm: -7, PrivateKey: key,
		},
	}
	if !reflect.DeepEqual(rec.Passkeys, want) {
		t.Fatalf("passkeys\n got: %+v\nwant: %+v", rec.Passkeys, want)
	}
}

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

package cli

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

	"github.com/Gerry3010/passbubble/backend/pkg/importers"
	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/vault"
)

// testPasskeyKey is a P-256 key in the stored form (base64url PKCS#8).
func testPasskeyKey(t *testing.T) string {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(der)
}

// storedPasskey is a passkey record as the app writes it, with a key the CLI
// does not model.
func storedPasskey(id, rpID, key string) map[string]any {
	return map[string]any{
		"credential_id": id, "rp_id": rpID, "rp_name": "Old name", "user_handle": "dXNlci0xMjM0",
		"user_name": "octo", "key_algorithm": float64(-7), "private_key": key, "sign_count": float64(0),
		"discoverable": true, "prf_secret": "prf-" + id, "created_at": "2026-01-01T00:00:00Z",
		"future_pk": "keep me",
	}
}

func storedWithPasskeys(t *testing.T, passkeys ...map[string]any) string {
	t.Helper()
	pk := make([]any, len(passkeys))
	for i, p := range passkeys {
		pk[i] = p
	}
	b, err := json.Marshal(map[string]any{"username": "octo", "password": "pw", "future": 1, "passkeys": pk})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// bitwardenWithPasskeys is a Bitwarden file with one login holding passkeys.
func bitwardenWithPasskeys(t *testing.T, creds ...map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"items": []any{map[string]any{
		"type": 1, "name": "GitHub",
		"login": map[string]any{"username": "octo", "password": "pw", "fido2Credentials": creds},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func bitwardenCredential(id, rpID, key string) map[string]any {
	return map[string]any{
		"credentialId": "b64." + id, "keyType": "public-key", "keyAlgorithm": "ECDSA", "keyCurve": "P-256",
		"keyValue": key, "rpId": rpID, "userHandle": "dXNlci0xMjM0", "userName": "octo", "counter": "5",
		"rpName": "New name", "userDisplayName": "", "discoverable": "true", "creationDate": "2026-06-01T00:00:00.000Z",
	}
}

func TestMergeImportCombinesPasskeysByCredentialID(t *testing.T) {
	key := testPasskeyKey(t)
	s := newCommandVault(t, storedWithPasskeys(t,
		storedPasskey("AQID", "a.example", key),
		storedPasskey("BAUG", "b.example", key),
	))
	runImport(t, "bitwarden", bitwardenWithPasskeys(t,
		bitwardenCredential("BAUG", "b.example", key), // same id: replaces in place
		bitwardenCredential("BwgJ", "c.example", key), // new id: appended
	))

	got := s.saved(t)
	if got["future"] != float64(1) || got["password"] != "pw" {
		t.Errorf("the rest of the entry changed: %v", got)
	}
	replaced := storedPasskey("BAUG", "b.example", key)
	replaced["rp_name"] = "New name"
	replaced["created_at"] = "2026-06-01T00:00:00Z"
	want := []any{
		storedPasskey("AQID", "a.example", key), // not in the file: stays
		replaced,                                // the file's values, the PRF secret and unknown key kept
		map[string]any{
			"credential_id": "BwgJ", "rp_id": "c.example", "rp_name": "New name", "user_handle": "dXNlci0xMjM0",
			"user_name": "octo", "key_algorithm": float64(-7), "private_key": key, "sign_count": float64(0),
			"discoverable": true, "created_at": "2026-06-01T00:00:00Z",
		},
	}
	if !reflect.DeepEqual(got["passkeys"], want) {
		t.Fatalf("passkeys\n got: %v\nwant: %v", got["passkeys"], want)
	}
}

func TestMergeImportWithoutPasskeysKeepsTheStoredOnes(t *testing.T) {
	key := testPasskeyKey(t)
	stored := storedWithPasskeys(t, storedPasskey("AQID", "a.example", key))
	s := newCommandVault(t, stored)
	runImport(t, "csv-generic", "name,url,username,password,notes\nGitHub,,octo,new-pw,\n")

	got := s.saved(t)
	var want map[string]any
	if err := json.Unmarshal([]byte(stored), &want); err != nil {
		t.Fatal(err)
	}
	want["password"] = "new-pw"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestReplaceAllTakesExactlyThePasskeysOfTheFile(t *testing.T) {
	key := testPasskeyKey(t)
	s := newCommandVault(t, storedWithPasskeys(t, storedPasskey("AQID", "a.example", key)))
	runReplaceAll(t, "bitwarden", bitwardenWithPasskeys(t, bitwardenCredential("BwgJ", "c.example", key)))

	passkeys, _ := s.saved(t)["passkeys"].([]any)
	if len(passkeys) != 1 || passkeys[0].(map[string]any)["credential_id"] != "BwgJ" {
		t.Fatalf("passkeys = %v", passkeys)
	}
}

func TestSamePasskeyIDComparesBytes(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		same bool
	}{
		{"q2D0bQ2u3E7XoWlS1yZbVw", "q2D0bQ2u3E7XoWlS1yZbVw", true},
		{"-_8", "+/8=", true},
		{"-_8", "+/8", true},
		{"AQID", "AQIE", false},
		{"", "AQID", false},
		{"not base64!", "not base64!", true},
		{"not base64!", "AQID", false},
	} {
		if got := samePasskeyID(tc.a, tc.b); got != tc.same {
			t.Errorf("samePasskeyID(%q, %q) = %v", tc.a, tc.b, got)
		}
	}
}

// Export: the passkeys go out as Bitwarden passkeys and nowhere else; the
// server fields as fields.
func TestBitwardenExportOfPasskeysAndServerFields(t *testing.T) {
	key := testPasskeyKey(t)
	s := newCommandVault(t, storedWithPasskeys(t, storedPasskey("AQID", "a.example", key)))
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "web01", Type: "server"},
		`{"username":"root","password":"pw","host":"web01.example.com","port":"2222","private_key":"KEY","key_passphrase":"PP"}`)

	items := runBitwardenExport(t)
	login := items[0]["login"].(map[string]any)
	creds, _ := login["fido2Credentials"].([]any)
	if len(creds) != 1 {
		t.Fatalf("login = %v", login)
	}
	cred := creds[0].(map[string]any)
	if cred["credentialId"] != "b64.AQID" || cred["keyValue"] != key || cred["counter"] != "0" || cred["rpId"] != "a.example" {
		t.Errorf("credential = %v", cred)
	}
	fields := fieldsByName(t, items[0])
	for name := range fields {
		if strings.Contains(name, "passkey") || strings.Contains(name, "future_pk") || strings.Contains(name, "prf") {
			t.Errorf("passkeys must not travel as fields too: %v", fields)
		}
	}
	if f := fields["future"]; f == nil || f["value"] != "1" {
		t.Errorf("the unknown key should still travel: %v", fields)
	}

	server := fieldsByName(t, items[1])
	for name, want := range map[string]struct {
		value     string
		fieldType float64
	}{
		"host": {"web01.example.com", 0}, "port": {"2222", 0},
		"private_key": {"KEY", 1}, "key_passphrase": {"PP", 1}, "type": {"server", 0},
	} {
		if f := server[name]; f == nil || f["value"] != want.value || f["type"] != want.fieldType {
			t.Errorf("server field %s = %v, want %+v", name, f, want)
		}
	}
	if l := items[1]["login"].(map[string]any); l["username"] != "root" || l["password"] != "pw" {
		t.Errorf("server login = %v", l)
	}
}

func TestCSVExportCountsPasskeysAsLeftOut(t *testing.T) {
	key := testPasskeyKey(t)
	newCommandVault(t, `{"username":"octo","password":"pw","passkeys":[`+mustJSON(t, storedPasskey("AQID", "a.example", key))+`]}`)
	file, stderr := runCSVExport(t)
	if !strings.Contains(stderr, "1 field of 1 entry") || !strings.Contains(stderr, "passkeys") {
		t.Errorf("stderr = %q", stderr)
	}
	if strings.Contains(file, key) {
		t.Error("the passkey's key must not be in the CSV")
	}
}

func TestImportMappingCarriesPasskeys(t *testing.T) {
	key := testPasskeyKey(t)
	got := roundTrip(t, []vault.EntryRecord{{Name: "GitHub", Type: "password", Data: vault.EntryData{
		Username: "octo",
		Passkeys: []vault.Passkey{{CredentialID: "AQID", RPID: "a.example", UserHandle: "BAUG", UserName: "octo",
			KeyAlgorithm: -7, PrivateKey: key, Discoverable: true, CreatedAt: "2026-01-01T00:00:00Z", PRFSecret: "lost"}},
	}}}, "bitwarden", "bitwarden")
	want := []vault.Passkey{{CredentialID: "AQID", RPID: "a.example", UserHandle: "BAUG", UserName: "octo",
		KeyAlgorithm: -7, PrivateKey: key, Discoverable: true, CreatedAt: "2026-01-01T00:00:00Z"}}
	if !reflect.DeepEqual(got[0].Passkeys, want) {
		t.Fatalf("passkeys\n got: %+v\nwant: %+v", got[0].Passkeys, want)
	}
	if got := importerRecordToEntryData(importers.EntryRecord{Name: "GitHub"}); got.Passkeys != nil {
		t.Errorf("no passkeys must stay nil: %+v", got.Passkeys)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

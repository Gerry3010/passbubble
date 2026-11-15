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

package exporters_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Gerry3010/passbubble/backend/pkg/exporters"
)

func TestExportBitwardenWritesPasskeysAsFido2Credentials(t *testing.T) {
	data, err := exporters.ExportBitwarden([]exporters.EntryRecord{{
		Type: "password", Name: "Example", Username: "alice@example.com",
		UnknownFields: []exporters.UnknownField{{Key: "future", Value: "x"}},
		Passkeys: []exporters.Passkey{
			{
				CredentialID: "q2D0bQ2u3E7XoWlS1yZbVw", RPID: "example.com", RPName: "Example",
				UserHandle: "dXNlci0xMjM0", UserName: "alice@example.com", UserDisplayName: "Alice",
				KeyAlgorithm: -7, PrivateKey: "MIGHAgEA", SignCount: 0, Discoverable: true,
				PRFSecret: "3q2-7w", CreatedAt: "2026-11-14T03:00:00Z",
			},
			// Written in standard base64 by some client: goes out as base64url.
			{CredentialID: "+/8=", RPID: "b.example", UserHandle: "+/8=", UserName: "bob", KeyAlgorithm: -7, PrivateKey: "ab+/"},
		},
	}}, exporters.BitwardenExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var export struct {
		Items []struct {
			Fields []exportedField `json:"fields"`
			Login  struct {
				Fido2Credentials []map[string]any `json:"fido2Credentials"`
			} `json:"login"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatal(err)
	}
	item := export.Items[0]
	want := []map[string]any{
		{
			"credentialId": "b64.q2D0bQ2u3E7XoWlS1yZbVw", "keyType": "public-key", "keyAlgorithm": "ECDSA",
			"keyCurve": "P-256", "keyValue": "MIGHAgEA", "rpId": "example.com", "userHandle": "dXNlci0xMjM0",
			"userName": "alice@example.com", "counter": "0", "rpName": "Example", "userDisplayName": "Alice",
			"discoverable": "true", "creationDate": "2026-11-14T03:00:00Z",
		},
		{
			"credentialId": "b64.-_8", "keyType": "public-key", "keyAlgorithm": "ECDSA",
			"keyCurve": "P-256", "keyValue": "ab-_", "rpId": "b.example", "userHandle": "-_8",
			"userName": "bob", "counter": "0", "rpName": "", "userDisplayName": "",
			"discoverable": "false", "creationDate": nil,
		},
	}
	if !reflect.DeepEqual(item.Login.Fido2Credentials, want) {
		t.Fatalf("fido2Credentials\n got: %v\nwant: %v", item.Login.Fido2Credentials, want)
	}
	// The passkeys travel once, in their own place: no field holds them or the
	// PRF secret.
	wantFields := []exportedField{{"future", "x", hiddenField}}
	if !reflect.DeepEqual(item.Fields, wantFields) {
		t.Errorf("fields = %+v, want %+v", item.Fields, wantFields)
	}
	if strings.Contains(string(data), "3q2-7w") {
		t.Error("the PRF secret has no place in the format")
	}
}

func TestExportBitwardenWithoutPasskeysHasNoFido2Key(t *testing.T) {
	item := exportOne(t, exporters.EntryRecord{Type: "password", Name: "GitHub", Username: "octo", Password: "pw"})
	if _, has := item.Login["fido2Credentials"]; has {
		t.Errorf("login = %v", item.Login)
	}
}

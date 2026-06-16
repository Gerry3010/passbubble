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
	"testing"

	"github.com/Gerry3010/passbubble/backend/pkg/importers"
)

const bitwardenJSON = `{
  "items": [
    {
      "type": 1,
      "name": "GitHub",
      "login": {
        "username": "user@example.com",
        "password": "s3cr3t",
        "uris": [{"uri": "https://github.com"}],
        "totp": ""
      },
      "fields": [{"name": "Recovery Code", "value": "ABCD-1234"}]
    },
    {
      "type": 1,
      "name": "Authenticator",
      "login": {
        "username": "user@example.com",
        "password": "",
        "totp": "JBSWY3DPEHPK3PXP"
      }
    },
    {
      "type": 2,
      "name": "SSH Key",
      "notes": "-----BEGIN RSA PRIVATE KEY-----"
    },
    {
      "type": 3,
      "name": "Visa Card",
      "card": {
        "cardholderName": "Max Mustermann",
        "number": "4111111111111111",
        "expMonth": "12",
        "expYear": "2027",
        "code": "123"
      }
    },
    {
      "type": 4,
      "name": "My Identity",
      "identity": {
        "firstName": "Max",
        "lastName": "Mustermann",
        "email": "max@example.com",
        "country": "DE"
      }
    },
    {
      "type": 99,
      "name": "Unknown type"
    }
  ]
}`

func TestParseBitwarden(t *testing.T) {
	result, err := importers.ParseBitwarden([]byte(bitwardenJSON))
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Records) != 5 {
		t.Fatalf("expected 5 records, got %d", len(result.Records))
	}
	if result.Skipped != 1 {
		t.Fatalf("expected 1 skipped (unknown type), got %d", result.Skipped)
	}

	tests := []struct {
		idx      int
		name     string
		typ      string
		username string
		url      string
	}{
		{0, "GitHub", "password", "user@example.com", "https://github.com"},
		{1, "Authenticator", "totp", "user@example.com", ""},
		{2, "SSH Key", "note", "", ""},
		{3, "Visa Card", "credit-card", "", ""},
		{4, "My Identity", "identity", "", ""},
	}

	for _, tt := range tests {
		rec := result.Records[tt.idx]
		if rec.Name != tt.name {
			t.Errorf("[%d] name: want %q, got %q", tt.idx, tt.name, rec.Name)
		}
		if rec.Type != tt.typ {
			t.Errorf("[%d] type: want %q, got %q", tt.idx, tt.typ, rec.Type)
		}
		if rec.Username != tt.username {
			t.Errorf("[%d] username: want %q, got %q", tt.idx, tt.username, rec.Username)
		}
		if rec.URL != tt.url {
			t.Errorf("[%d] url: want %q, got %q", tt.idx, tt.url, rec.URL)
		}
	}

	// GitHub entry should have custom field
	gh := result.Records[0]
	if len(gh.CustomFields) != 1 || gh.CustomFields[0].Label != "Recovery Code" {
		t.Errorf("expected custom field 'Recovery Code', got %+v", gh.CustomFields)
	}

	// Card fields
	card := result.Records[3]
	if card.CardNumber != "4111111111111111" {
		t.Errorf("card number: want 4111111111111111, got %s", card.CardNumber)
	}
	if card.HolderName != "Max Mustermann" {
		t.Errorf("holder: want Max Mustermann, got %s", card.HolderName)
	}

	// Identity fields
	ident := result.Records[4]
	if ident.FirstName != "Max" || ident.LastName != "Mustermann" {
		t.Errorf("identity name: got %s %s", ident.FirstName, ident.LastName)
	}
}

func TestParseBitwardenInvalidJSON(t *testing.T) {
	_, err := importers.ParseBitwarden([]byte("{invalid"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseBitwardenEmpty(t *testing.T) {
	result, err := importers.ParseBitwarden([]byte(`{"items":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(result.Records))
	}
}

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
	"slices"
	"testing"

	"github.com/Gerry3010/passbubble/backend/pkg/exporters"
)

func TestExportBitwarden(t *testing.T) {
	records := []exporters.EntryRecord{
		{
			Type: "password", Name: "GitHub",
			URL: "https://github.com", Username: "user@example.com", Password: "s3cr3t",
			Notes:        "work",
			CustomFields: []exporters.CustomField{{Label: "2FA backup", Value: "ABCD"}},
		},
		{
			Type: "totp", Name: "Auth App",
			Username: "user@example.com", TOTPSecret: "JBSWY3DP",
		},
		{
			Type: "note", Name: "SSH Key", Notes: "private key content",
		},
		{
			Type: "credit-card", Name: "Visa",
			CardNumber: "4111111111111111", HolderName: "Max M",
			ExpiryMonth: "12", ExpiryYear: "2027", CVV: "123",
		},
		{
			Type: "identity", Name: "Me",
			FirstName: "Max", LastName: "M", Email: "max@example.com", Country: "DE",
		},
		{
			Type: "api-key", Name: "Stripe Key",
			Password: "sk_live_xxx",
		},
	}

	data, err := exporters.ExportBitwarden(records, exporters.BitwardenExportOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var export map[string]any
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	items, ok := export["items"].([]any)
	if !ok || len(items) != len(records) {
		t.Fatalf("expected %d items, got %d", len(records), len(items))
	}

	typeChecks := []struct {
		idx      int
		wantType float64 // JSON numbers decode as float64
		wantName string
	}{
		{0, 1, "GitHub"},
		{1, 1, "Auth App"},
		{2, 2, "SSH Key"},
		{3, 3, "Visa"},
		{4, 4, "Me"},
		{5, 1, "Stripe Key"},
	}

	for _, tc := range typeChecks {
		item := items[tc.idx].(map[string]any)
		if item["name"] != tc.wantName {
			t.Errorf("[%d] name: want %q, got %q", tc.idx, tc.wantName, item["name"])
		}
		if item["type"].(float64) != tc.wantType {
			t.Errorf("[%d] type: want %.0f, got %.0f", tc.idx, tc.wantType, item["type"].(float64))
		}
	}

	// Verify encrypted=false
	if export["encrypted"] != false {
		t.Error("expected encrypted=false")
	}

	// Verify custom field on GitHub entry
	ghItem := items[0].(map[string]any)
	fields := ghItem["fields"].([]any)
	if len(fields) != 1 {
		t.Fatalf("expected 1 custom field, got %d", len(fields))
	}
	field := fields[0].(map[string]any)
	if field["name"] != "2FA backup" {
		t.Errorf("custom field name: want '2FA backup', got %q", field["name"])
	}
}

func TestExportBitwardenEmpty(t *testing.T) {
	data, err := exporters.ExportBitwarden(nil, exporters.BitwardenExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var export map[string]any
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatal(err)
	}
	items := export["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("expected empty items, got %d", len(items))
	}
}

func TestExportBitwardenCarriesATOTPSecretOnPasswordEntries(t *testing.T) {
	data, err := exporters.ExportBitwarden([]exporters.EntryRecord{
		{Type: "password", Name: "With", Username: "octo", Password: "pw", TOTPSecret: "JBSWY3DPEHPK3PXP"},
		{Type: "password", Name: "Without", Username: "octo", Password: "pw"},
	}, exporters.BitwardenExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var export struct {
		Items []struct {
			Login map[string]any `json:"login"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatal(err)
	}
	if got := export.Items[0].Login["totp"]; got != "JBSWY3DPEHPK3PXP" {
		t.Errorf("totp = %v", got)
	}
	if _, has := export.Items[1].Login["totp"]; has {
		t.Errorf("an entry without a secret needs no totp key: %v", export.Items[1].Login)
	}
}

type exportedField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  int    `json:"type"`
}

type exportedItem struct {
	Type     int             `json:"type"`
	Notes    string          `json:"notes"`
	Fields   []exportedField `json:"fields"`
	Login    map[string]any  `json:"login"`
	Card     map[string]any  `json:"card"`
	Identity map[string]any  `json:"identity"`
}

func exportOne(t *testing.T, rec exporters.EntryRecord) exportedItem {
	t.Helper()
	data, err := exporters.ExportBitwarden([]exporters.EntryRecord{rec}, exporters.BitwardenExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var export struct {
		Items []exportedItem `json:"items"`
	}
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatal(err)
	}
	return export.Items[0]
}

const (
	textField   = 0
	hiddenField = 1
)

// Everything that has no place in the item comes along under its JSON key.
func TestExportBitwardenKeepsWhatTheFormatHasNoPlaceFor(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rec      exporters.EntryRecord
		wantType int
		want     []exportedField
	}{
		{
			name: "bank account",
			rec: exporters.EntryRecord{
				Type: "bank-account", Name: "Giro", Username: "max", Password: "online-pw",
				BankName: "Erste", IBAN: "AT00 1234", BIC: "GIBAATWW", AccountNumber: "1234", AccountType: "checking",
			},
			wantType: 1,
			want: []exportedField{
				{"bank_name", "Erste", textField},
				{"iban", "AT00 1234", hiddenField},
				{"bic", "GIBAATWW", textField},
				{"account_number", "1234", hiddenField},
				{"account_type", "checking", textField},
				{"type", "bank-account", textField},
			},
		},
		{
			name: "license",
			rec: exporters.EntryRecord{
				Type: "license", Name: "IDE", ProductName: "GoLand", LicenseKey: "KEY-1",
				PurchaseEmail: "max@example.com", PurchaseDate: "2026-01-01", ExpiresAt: "2027-01-01",
			},
			wantType: 1,
			want: []exportedField{
				{"product_name", "GoLand", textField},
				{"license_key", "KEY-1", hiddenField},
				{"purchase_email", "max@example.com", textField},
				{"purchase_date", "2026-01-01", textField},
				{"expires_at", "2027-01-01", textField},
				{"type", "license", textField},
			},
		},
		{
			name: "a type this version does not know",
			rec: exporters.EntryRecord{
				Type: "future-type", Name: "X", Password: "pw",
				UnknownFields: []exporters.UnknownField{{Key: "future_key", Value: "v"}},
			},
			wantType: 1,
			want: []exportedField{
				{"type", "future-type", textField},
				{"future_key", "v", hiddenField},
			},
		},
		{
			name: "a login with everything a login has no place for",
			rec: exporters.EntryRecord{
				Type: "password", Name: "GitHub", URL: "https://github.com", Username: "octo", Password: "pw", TOTPSecret: "S",
				SignInWith: "google", Issuer: "GitHub", Period: 60, Digits: 8, Algorithm: "SHA256",
				CardNumber: "4111", CVV: "123", FirstName: "Max",
				UnknownFields: []exporters.UnknownField{{Key: "future_flag", Value: `{"nested":true}`}},
			},
			wantType: 1,
			want: []exportedField{
				{"sign_in_with", "google", textField},
				{"issuer", "GitHub", textField},
				{"period", "60", textField},
				{"digits", "8", textField},
				{"algorithm", "SHA256", textField},
				{"card_number", "4111", hiddenField},
				{"cvv", "123", hiddenField},
				{"first_name", "Max", textField},
				{"future_flag", `{"nested":true}`, hiddenField},
			},
		},
		{
			name: "a card with a URL and a login",
			rec: exporters.EntryRecord{
				Type: "credit-card", Name: "Visa", URL: "https://bank.example", Username: "max", Password: "pw", TOTPSecret: "S",
				CardNumber: "4111", HolderName: "Max", ExpiryMonth: "12", ExpiryYear: "2030", CVV: "123", IBAN: "AT00",
			},
			wantType: 3,
			want: []exportedField{
				{"url", "https://bank.example", textField},
				{"username", "max", textField},
				{"password", "pw", hiddenField},
				{"totp_secret", "S", hiddenField},
				{"iban", "AT00", hiddenField},
			},
		},
		{
			name: "a note with a URL and a password",
			rec: exporters.EntryRecord{
				Type: "note", Name: "N", Notes: "text", URL: "https://x.example", Password: "pw", LicenseKey: "K",
			},
			wantType: 2,
			want: []exportedField{
				{"url", "https://x.example", textField},
				{"password", "pw", hiddenField},
				{"license_key", "K", hiddenField},
			},
		},
		{
			name: "an identity with a URL",
			rec: exporters.EntryRecord{
				Type: "identity", Name: "Me", URL: "https://x.example", Title: "Dr", FirstName: "Max", Street: "Main St",
				BankName: "Erste",
			},
			wantType: 4,
			want: []exportedField{
				{"url", "https://x.example", textField},
				{"bank_name", "Erste", textField},
			},
		},
		{
			name:     "a plain login has nothing left over",
			rec:      exporters.EntryRecord{Type: "password", Name: "GitHub", URL: "https://github.com", Username: "octo", Password: "pw", TOTPSecret: "S", Notes: "n"},
			wantType: 1,
		},
		{
			name:     "a TOTP entry has nothing left over",
			rec:      exporters.EntryRecord{Type: "totp", Name: "AWS", Username: "octo", Password: "pw", TOTPSecret: "S"},
			wantType: 1,
		},
		{
			name:     "no type counts as a login",
			rec:      exporters.EntryRecord{Name: "Old", Username: "octo", Password: "pw"},
			wantType: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := exportOne(t, tc.rec)
			if item.Type != tc.wantType {
				t.Errorf("item type = %d, want %d", item.Type, tc.wantType)
			}
			if !slices.Equal(item.Fields, tc.want) {
				t.Errorf("fields\n got: %+v\nwant: %+v", item.Fields, tc.want)
			}
		})
	}
}

func TestExportBitwardenUsesThePlacesTheFormatHas(t *testing.T) {
	identity := exportOne(t, exporters.EntryRecord{
		Type: "identity", Name: "Me", Title: "Dr", FirstName: "Max", LastName: "M", Company: "C", Email: "e", Phone: "p",
		Street: "s", City: "c", State: "st", PostalCode: "1", Country: "AT",
	}).Identity
	for key, want := range map[string]string{
		"title": "Dr", "firstName": "Max", "lastName": "M", "company": "C", "email": "e", "phone": "p",
		"address1": "s", "city": "c", "state": "st", "postalCode": "1", "country": "AT",
	} {
		if identity[key] != want {
			t.Errorf("identity.%s = %v, want %q", key, identity[key], want)
		}
	}
}

func TestExportBitwardenHidesCustomFieldsItDoesNotKnow(t *testing.T) {
	item := exportOne(t, exporters.EntryRecord{Type: "password", Name: "X", CustomFields: []exporters.CustomField{
		{Label: "no kind", Value: "1"},
		{Label: "text", Value: "2", Type: "text"},
		{Label: "url", Value: "3", Type: "url"},
		{Label: "email", Value: "4", Type: "email"},
		{Label: "phone", Value: "5", Type: "phone"},
		{Label: "note", Value: "6", Type: "note"},
		{Label: "password", Value: "7", Type: "password"},
		{Label: "ssh", Value: "8", Type: "ssh"},
		{Label: "totp", Value: "9", Type: "totp"},
		{Label: "future", Value: "10", Type: "future-kind"},
	}})
	want := []exportedField{
		{"no kind", "1", textField},
		{"text", "2", textField},
		{"url", "3", textField},
		{"email", "4", textField},
		{"phone", "5", textField},
		{"note", "6", textField},
		{"password", "7", hiddenField},
		{"ssh", "8", hiddenField},
		{"totp", "9", hiddenField},
		{"future", "10", hiddenField},
	}
	if !slices.Equal(item.Fields, want) {
		t.Errorf("fields\n got: %+v\nwant: %+v", item.Fields, want)
	}
}

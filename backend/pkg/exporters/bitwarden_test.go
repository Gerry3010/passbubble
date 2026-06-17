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
	"testing"

	"github.com/Gerry3010/passbubble/backend/pkg/exporters"
)

func TestExportBitwarden(t *testing.T) {
	records := []exporters.EntryRecord{
		{
			Type: "password", Name: "GitHub",
			URL: "https://github.com", Username: "user@example.com", Password: "s3cr3t",
			Notes: "work",
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

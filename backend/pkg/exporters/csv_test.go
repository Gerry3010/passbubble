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
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/Gerry3010/passbubble/backend/pkg/exporters"
)

func TestExportCSV(t *testing.T) {
	records := []exporters.EntryRecord{
		{Name: "GitHub", URL: "https://github.com", Username: "user@example.com", Password: "s3cr3t", Notes: "work account"},
		{Name: "Bank", URL: "https://bank.de", Username: "max", Password: "p@ss"},
	}

	data, err := exporters.ExportCSV(records)
	if err != nil {
		t.Fatal(err)
	}

	r := csv.NewReader(bytes.NewReader(data))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 3 { // header + 2 records
		t.Fatalf("expected 3 rows (header + 2), got %d", len(rows))
	}

	header := rows[0]
	if header[0] != "name" || header[1] != "url" || header[2] != "username" || header[3] != "password" {
		t.Errorf("unexpected header: %v", header)
	}

	row1 := rows[1]
	if row1[0] != "GitHub" || row1[2] != "user@example.com" || row1[3] != "s3cr3t" {
		t.Errorf("unexpected row 1: %v", row1)
	}
	if row1[4] != "work account" {
		t.Errorf("notes not exported: %v", row1)
	}
}

func TestExportCSVEmpty(t *testing.T) {
	data, err := exporters.ExportCSV(nil)
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(bytes.NewReader(data))
	rows, _ := r.ReadAll()
	if len(rows) != 1 { // header only
		t.Fatalf("expected 1 row (header only), got %d", len(rows))
	}
}

func TestExportCSVSpecialChars(t *testing.T) {
	records := []exporters.EntryRecord{
		{Name: `Entry "with" quotes`, Password: "pass,word", Notes: "line1\nline2"},
	}
	data, err := exporters.ExportCSV(records)
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(bytes.NewReader(data))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("special chars broke CSV: %v", err)
	}
	if rows[1][0] != `Entry "with" quotes` {
		t.Errorf("quotes not preserved: %q", rows[1][0])
	}
	if rows[1][3] != "pass,word" {
		t.Errorf("comma in password not preserved: %q", rows[1][3])
	}
}

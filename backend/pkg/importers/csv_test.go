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

func TestParseCSVGeneric(t *testing.T) {
	data := []byte("name,url,username,password,notes\n" +
		"GitHub,https://github.com,user@example.com,s3cr3t,work account\n" +
		"Empty,,,, \n")

	result, err := importers.ParseCSV(data, importers.CSVFormatGeneric)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(result.Records))
	}

	r := result.Records[0]
	if r.Name != "GitHub" || r.Username != "user@example.com" || r.Password != "s3cr3t" {
		t.Errorf("unexpected record: %+v", r)
	}
	if r.URL != "https://github.com" || r.Notes != "work account" {
		t.Errorf("unexpected url/notes: %+v", r)
	}
}

func TestParseCSVChrome(t *testing.T) {
	data := []byte("name,url,username,password\n" +
		"google.com,https://google.com,me@gmail.com,pass123\n")

	result, err := importers.ParseCSV(data, importers.CSVFormatChrome)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	if result.Records[0].Username != "me@gmail.com" {
		t.Errorf("unexpected username: %s", result.Records[0].Username)
	}
}

func TestParseCSVLastPass(t *testing.T) {
	// url,username,password,totp,extra,name,grouping,fav
	data := []byte("url,username,password,totp,extra,name,grouping,fav\n" +
		"https://bank.de,user,pass123,,notes,My Bank,Finance,0\n" +
		"https://auth.example.com,user2,pass,JBSWY3DP,,TOTP Account,Work,0\n")

	result, err := importers.ParseCSV(data, importers.CSVFormatLastPass)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(result.Records))
	}
	if result.Records[0].Name != "My Bank" {
		t.Errorf("expected 'My Bank', got %q", result.Records[0].Name)
	}
	if result.Records[1].Type != "totp" {
		t.Errorf("expected totp type, got %q", result.Records[1].Type)
	}
}

func TestParseCSVSkipsNoNameNoURL(t *testing.T) {
	data := []byte("name,url,username,password,notes\n" +
		",,user,pass,\n") // no name and no url

	result, err := importers.ParseCSV(data, importers.CSVFormatGeneric)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 0 || result.Skipped != 1 {
		t.Fatalf("expected 0 records and 1 skipped, got %d/%d", len(result.Records), result.Skipped)
	}
}

func TestParseCSVEmpty(t *testing.T) {
	data := []byte("name,url,username,password\n")
	result, err := importers.ParseCSV(data, importers.CSVFormatGeneric)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(result.Records))
	}
}

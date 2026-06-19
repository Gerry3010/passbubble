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
	"strings"
	"testing"
)

func findRec(t *testing.T, recs []EntryRecord, name string) EntryRecord {
	t.Helper()
	for _, r := range recs {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("record %q not found", name)
	return EntryRecord{}
}

func TestNormalizeTimestamp(t *testing.T) {
	cases := map[string]string{
		"2023-12-21T01:49:17.499442+00:00": "2023-12-21T01:49:17Z", // Psono µs+tz
		"2024-05-01T12:00:00.000Z":         "2024-05-01T12:00:00Z", // Bitwarden
		"2024-05-01T12:00:00Z":             "2024-05-01T12:00:00Z", // plain RFC3339
		"":                                 "",
		"not-a-date":                       "",
	}
	for in, want := range cases {
		if got := normalizeTimestamp(in); got != want {
			t.Errorf("normalizeTimestamp(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPsonoDatesAndFolders(t *testing.T) {
	// Folder name with a literal "/" must NOT be split.
	data := []byte(`{
	  "folders": [
	    {"name": "Business", "folders": [
	      {"name": "App-/Keys", "items": [
	        {"type":"application_password","name":"Deep","application_password_title":"Deep",
	         "application_password_username":"u","application_password_password":"p",
	         "create_date":"2023-12-21T01:49:17.499442+00:00","write_date":"2024-01-02T03:04:05+00:00"}
	      ]}
	    ]}
	  ],
	  "items": [
	    {"type":"website_password","name":"Root","website_password_url":"https://x",
	     "website_password_username":"ru","website_password_password":"rp",
	     "create_date":"2020-06-01T00:00:00+00:00","write_date":"2020-06-02T00:00:00+00:00"}
	  ]
	}`)
	res, err := ParsePsono(data)
	if err != nil {
		t.Fatal(err)
	}

	root := findRec(t, res.Records, "Root")
	if len(root.FolderPath) != 0 {
		t.Errorf("root entry FolderPath = %v, want empty", root.FolderPath)
	}
	if root.CreatedAt != "2020-06-01T00:00:00Z" {
		t.Errorf("root CreatedAt = %q", root.CreatedAt)
	}

	deep := findRec(t, res.Records, "Deep")
	want := []string{"Business", "App-/Keys"}
	if strings.Join(deep.FolderPath, "|") != strings.Join(want, "|") {
		t.Errorf("deep FolderPath = %v, want %v", deep.FolderPath, want)
	}
	if deep.CreatedAt != "2023-12-21T01:49:17Z" || deep.UpdatedAt != "2024-01-02T03:04:05Z" {
		t.Errorf("deep dates = %q / %q", deep.CreatedAt, deep.UpdatedAt)
	}
}

func TestBitwardenDatesAndFolders(t *testing.T) {
	data := []byte(`{
	  "folders": [{"id":"f1","name":"Work/Email"}],
	  "items": [
	    {"type":1,"name":"Gmail","folderId":"f1",
	     "creationDate":"2022-03-04T05:06:07.000Z","revisionDate":"2023-03-04T05:06:07.000Z",
	     "login":{"username":"me","password":"pw","uris":[{"uri":"https://mail"}]}}
	  ]
	}`)
	res, err := ParseBitwarden(data)
	if err != nil {
		t.Fatal(err)
	}
	g := findRec(t, res.Records, "Gmail")
	// Bitwarden nests via "/" → two path segments.
	if strings.Join(g.FolderPath, "|") != "Work|Email" {
		t.Errorf("FolderPath = %v, want [Work Email]", g.FolderPath)
	}
	if g.CreatedAt != "2022-03-04T05:06:07Z" || g.UpdatedAt != "2023-03-04T05:06:07Z" {
		t.Errorf("dates = %q / %q", g.CreatedAt, g.UpdatedAt)
	}
}

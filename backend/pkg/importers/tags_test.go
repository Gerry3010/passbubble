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
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Gerry3010/passbubble/backend/pkg/exporters"
	"github.com/Gerry3010/passbubble/backend/pkg/importers"
	"github.com/tobischo/gokeepasslib/v3"
	"github.com/tobischo/gokeepasslib/v3/wrappers"
)

var (
	roundTripTags = []string{"work", "two words", "Überweisung"}

	roundTripRecords = []exporters.EntryRecord{
		{
			Type: "password", Name: "Tagged", URL: "https://github.com", Username: "octo", Password: "pw",
			CustomFields: []exporters.CustomField{{Label: "Recovery", Value: "ABCD"}},
			Tags:         roundTripTags,
		},
		{Type: "password", Name: "Untagged", Username: "octo", Password: "pw"},
	}
)

// checkRoundTrip checks the records against roundTripRecords.
func checkRoundTrip(t *testing.T, got []importers.EntryRecord) {
	t.Helper()
	if len(got) != 2 {
		t.Fatalf("want 2 records, got %d", len(got))
	}
	if !slices.Equal(got[0].Tags, roundTripTags) {
		t.Errorf("tags = %q, want %q", got[0].Tags, roundTripTags)
	}
	if got[1].Tags != nil {
		t.Errorf("an entry without tags should stay without, got %q", got[1].Tags)
	}
}

func TestCSVTagsRoundTrip(t *testing.T) {
	data, err := exporters.ExportCSV(roundTripRecords)
	if err != nil {
		t.Fatal(err)
	}
	if first, _, _ := strings.Cut(string(data), "\n"); first != "name,url,username,password,notes,tags" {
		t.Errorf("header = %q", first)
	}
	if !strings.Contains(string(data), `"work, two words, Überweisung"`) {
		t.Errorf("tags should be joined with \", \":\n%s", data)
	}

	res, err := importers.ParseCSV(data, importers.CSVFormatGeneric)
	if err != nil {
		t.Fatal(err)
	}
	checkRoundTrip(t, res.Records)
}

func TestOnePasswordCSVTagsRoundTrip(t *testing.T) {
	data, err := exporters.Export1PasswordCSV(roundTripRecords)
	if err != nil {
		t.Fatal(err)
	}
	if first, _, _ := strings.Cut(string(data), "\n"); first != "Title,Username,Password,URL,Notes,Type,Tags" {
		t.Errorf("header = %q", first)
	}
	res, err := importers.ParseCSV(data, importers.CSVFormat1Password)
	if err != nil {
		t.Fatal(err)
	}
	checkRoundTrip(t, res.Records)
}

func TestCSVTagsColumnIsFoundByName(t *testing.T) {
	data := []byte("name,url,username,password,notes,folder,TAGS\n" +
		"GitHub,https://github.com,octo,pw,,Business,\"work,, ops ,\"\n")
	res, err := importers.ParseCSV(data, importers.CSVFormatGeneric)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Records[0].Tags; !slices.Equal(got, []string{"work", "ops"}) {
		t.Errorf("tags = %q", got)
	}
}

// A file from before tags, or with other extra columns, reads as it always did.
func TestCSVWithoutTagsColumnHasNoTags(t *testing.T) {
	for name, tc := range map[string]struct {
		format importers.CSVFormat
		data   string
	}{
		"generic":          {importers.CSVFormatGeneric, "name,url,username,password,notes\nA,u,o,p,n\n"},
		"extra column":     {importers.CSVFormatGeneric, "name,url,username,password,notes,folder\nA,u,o,p,n,work\n"},
		"1password":        {importers.CSVFormat1Password, "Title,Username,Password,URL,Notes,Type\nA,o,p,u,n,Login\n"},
		"chrome with tags": {importers.CSVFormatChrome, "name,url,username,password,tags\nA,u,o,p,work\n"},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := importers.ParseCSV([]byte(tc.data), tc.format)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Records) != 1 || res.Records[0].Tags != nil {
				t.Fatalf("records = %+v", res.Records)
			}
		})
	}
}

func TestBitwardenTagsRoundTrip(t *testing.T) {
	data, err := exporters.ExportBitwarden(roundTripRecords, exporters.BitwardenExportOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// On disk: a plain text field after the real custom fields; none without tags.
	var export struct {
		Items []struct {
			Fields []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
				Type  int    `json:"type"`
			} `json:"fields"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatal(err)
	}
	fields := export.Items[0].Fields
	if len(fields) != 2 || fields[1].Name != "passbubble_tags" ||
		fields[1].Value != "work, two words, Überweisung" || fields[1].Type != 0 {
		t.Errorf("fields = %+v", fields)
	}
	if len(export.Items[1].Fields) != 0 {
		t.Errorf("an entry without tags needs no field: %+v", export.Items[1].Fields)
	}

	res, err := importers.ParseBitwarden(data)
	if err != nil {
		t.Fatal(err)
	}
	checkRoundTrip(t, res.Records)
	cfs := res.Records[0].CustomFields
	if len(cfs) != 1 || cfs[0].Label != "Recovery" {
		t.Errorf("the tags field must not stay a custom field: %+v", cfs)
	}
}

func Test1PUXTagsRoundTrip(t *testing.T) {
	data, err := exporters.Export1PUX(roundTripRecords)
	if err != nil {
		t.Fatal(err)
	}

	// An item without tags keeps writing an empty list, not null.
	raw := read1PUXExportData(t, data)
	if !strings.Contains(raw, `"tags": []`) || strings.Contains(raw, `"tags": null`) {
		t.Errorf("export.data should hold an empty tag list for the untagged item:\n%s", raw)
	}

	res, err := importers.Parse1PUX(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Records) != 2 {
		t.Fatalf("want 2 records, got %d", len(res.Records))
	}
	if !slices.Equal(res.Records[0].Tags, roundTripTags) {
		t.Errorf("tags = %q", res.Records[0].Tags)
	}
	if len(res.Records[1].Tags) != 0 {
		t.Errorf("tags = %q, want none", res.Records[1].Tags)
	}
}

func read1PUXExportData(t *testing.T, archive []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name != "export.data" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rc.Close() }()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	t.Fatal("export.data not found")
	return ""
}

// keepassFile builds a real .kdbx with one entry per tags value.
func keepassFile(t *testing.T, password string, tags ...string) []byte {
	t.Helper()
	group := gokeepasslib.NewGroup()
	group.Name = "Root"
	for i, value := range tags {
		entry := gokeepasslib.NewEntry()
		entry.Tags = value
		entry.Values = append(entry.Values,
			gokeepasslib.ValueData{Key: "Title", Value: gokeepasslib.V{Content: "Entry " + string(rune('A'+i))}},
			gokeepasslib.ValueData{Key: "Password", Value: gokeepasslib.V{Content: "pw", Protected: wrappers.NewBoolWrapper(true)}},
		)
		group.Entries = append(group.Entries, entry)
	}
	db := gokeepasslib.NewDatabase()
	db.Credentials = gokeepasslib.NewPasswordCredentials(password)
	db.Content.Root = &gokeepasslib.RootData{Groups: []gokeepasslib.Group{group}}
	if err := db.LockProtectedEntries(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := gokeepasslib.NewEncoder(&buf).Encode(db); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestKeePassTags(t *testing.T) {
	data := keepassFile(t, "master", "work;two words", "a, b;c", "", " ; , ")

	res, err := importers.ParseKeePass(bytes.NewReader(data), "master")
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"work", "two words"}, {"a", "b", "c"}, nil, nil}
	if len(res.Records) != len(want) {
		t.Fatalf("want %d records, got %d", len(want), len(res.Records))
	}
	for i, tags := range want {
		if got := res.Records[i].Tags; !slices.Equal(got, tags) {
			t.Errorf("record %d: tags = %q, want %q", i, got, tags)
		}
		if tags == nil && res.Records[i].Tags != nil {
			t.Errorf("record %d: no tags should stay nil, got %q", i, res.Records[i].Tags)
		}
	}
	for _, rec := range res.Records {
		for _, cf := range rec.CustomFields {
			if strings.EqualFold(cf.Label, "tags") {
				t.Errorf("tags leaked into the custom fields: %+v", rec.CustomFields)
			}
		}
	}
}

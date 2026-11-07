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
	"reflect"
	"testing"

	"github.com/Gerry3010/passbubble/backend/pkg/exporters"
	"github.com/Gerry3010/passbubble/backend/pkg/importers"
)

func TestBitwardenFieldTypes(t *testing.T) {
	const file = `{"items":[{"type":2,"name":"Note","fields":[
		{"name":"Plain","value":"text value","type":0},
		{"name":"No type","value":"old file"},
		{"name":"Hidden","value":"s3cret","type":1},
		{"name":"Flag","value":"true","type":2},
		{"name":"scan.png","value":"data:image/png;base64,aGVsbG8=","type":1},
		{"name":"no mime","value":"data:;base64,aGVsbG8=","type":1},
		{"name":"Text data URI","value":"data:image/png;base64,aGVsbG8=","type":0},
		{"name":"Not base64","value":"data:text/plain;base64,not base64!","type":1},
		{"name":"Not a file","value":"data: my note","type":1}
	]}]}`
	res, err := importers.ParseBitwarden([]byte(file))
	if err != nil {
		t.Fatal(err)
	}
	want := []importers.CustomField{
		{Label: "Plain", Value: "text value"},
		{Label: "No type", Value: "old file"},
		{Label: "Hidden", Value: "s3cret", Type: "password"},
		{Label: "Flag", Value: "true"},
		{Label: "scan.png", Value: "aGVsbG8=", Type: "file", Filename: "scan.png", MimeType: "image/png"},
		{Label: "no mime", Value: "aGVsbG8=", Type: "file", Filename: "no mime"},
		{Label: "Text data URI", Value: "data:image/png;base64,aGVsbG8="},
		{Label: "Not base64", Value: "data:text/plain;base64,not base64!", Type: "password"},
		{Label: "Not a file", Value: "data: my note", Type: "password"},
	}
	if got := res.Records[0].CustomFields; !reflect.DeepEqual(got, want) {
		t.Fatalf("custom fields\n got: %+v\nwant: %+v", got, want)
	}
}

func TestBitwardenFieldKindsRoundTrip(t *testing.T) {
	records := []exporters.EntryRecord{{
		Type: "password", Name: "GitHub", Username: "octo", Password: "pw",
		CustomFields: []exporters.CustomField{
			{Label: "PIN", Value: "1234", Type: "password"},
			{Label: "Plain", Value: "text"},
			{Label: "Scan", Value: "aGVsbG8=", Type: "file", Filename: "scan.png", MimeType: "image/png"},
			{Label: "Unnamed.bin", Value: "aGVsbG8=", Type: "file"},
		},
	}}
	data, err := exporters.ExportBitwarden(records, exporters.BitwardenExportOptions{IncludeFiles: true, FilesAsBase64: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := importers.ParseBitwarden(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []importers.CustomField{
		{Label: "PIN", Value: "1234", Type: "password"},
		{Label: "Plain", Value: "text"},
		// The format has one name per field, so the file name stands in for the label.
		{Label: "scan.png", Value: "aGVsbG8=", Type: "file", Filename: "scan.png", MimeType: "image/png"},
		// No file name stored: the label names it, and the MIME type is the generic one.
		{Label: "Unnamed.bin", Value: "aGVsbG8=", Type: "file", Filename: "Unnamed.bin", MimeType: "application/octet-stream"},
	}
	if got := res.Records[0].CustomFields; !reflect.DeepEqual(got, want) {
		t.Fatalf("custom fields\n got: %+v\nwant: %+v", got, want)
	}
}

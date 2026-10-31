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
	"slices"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/vault"
)

func TestSplitTagArgs(t *testing.T) {
	got := splitTagArgs([]string{"work", "dev, ops", "", "a,,b"})
	want := []string{"work", "dev", " ops", "", "a", "", "b"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	norm, err := vault.NormalizeTags(got)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(norm, []string{"work", "dev", "ops", "a", "b"}) {
		t.Fatalf("normalised = %q", norm)
	}
}

func TestTagFlagsAreRepeatable(t *testing.T) {
	if err := addCmd.Flags().Parse([]string{"--tag", "a", "--tag", "b,c"}); err != nil {
		t.Fatal(err)
	}
	vals, _ := addCmd.Flags().GetStringArray("tag")
	if got := splitTagArgs(vals); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("got %q", got)
	}
	for _, name := range []string{"add-tag", "remove-tag", "clear-tags"} {
		if updateCmd.Flags().Lookup(name) == nil {
			t.Errorf("update is missing --%s", name)
		}
	}
	if listCmd.Flags().Lookup("tag") == nil {
		t.Error("list is missing --tag")
	}
	if !slices.Contains(updateCmd.Aliases, "edit") {
		t.Error(`"edit" should be an alias of update`)
	}
}

func TestFormatTags(t *testing.T) {
	if got := formatTags(nil); got != "(none)" {
		t.Errorf("formatTags(nil) = %q", got)
	}
	if got := formatTags([]string{"work", "two words"}); got != "work, two words" {
		t.Errorf("formatTags = %q", got)
	}
	if got := describeFilter([]string{"work", "finance"}); got != "'work' and 'finance'" {
		t.Errorf("describeFilter = %q", got)
	}
}

func TestDescribeFileField(t *testing.T) {
	cases := []struct {
		cf   vault.CustomField
		want string
	}{
		{vault.CustomField{Label: "Scan", Value: "aGVsbG8=", Type: "file", Filename: "scan.png", MimeType: "image/png"},
			"[file] scan.png (image/png, 5 bytes)"},
		{vault.CustomField{Label: "Scan", Value: "aGVsbG8=", Type: "file"}, "[file] Scan (5 bytes)"},
		{vault.CustomField{Label: "Scan", Value: "%%%", Type: "file"}, "[file] Scan"},
	}
	for _, c := range cases {
		if got := describeFileField(c.cf); got != c.want {
			t.Errorf("describeFileField(%+v) = %q, want %q", c.cf, got, c.want)
		}
	}
}

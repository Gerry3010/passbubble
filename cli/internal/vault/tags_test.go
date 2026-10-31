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

package vault

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestNormalizeTag(t *testing.T) {
	cases := map[string]string{
		"work":              "work",
		"  work  ":          "work",
		"#work":             "work",
		"###work":           "work",
		"  #work":           "work",
		"# work":            "work",
		"two   words":       "two words",
		"tab\tand\nnewline": "tab and newline",
		"inner#hash":        "inner#hash",
		"work#":             "work#",
		"Ünïcödé  Tag":      "Ünïcödé Tag",
		"#":                 "",
		"  ##  ":            "",
		"":                  "",
	}
	for in, want := range cases {
		if got := NormalizeTag(in); got != want {
			t.Errorf("NormalizeTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeTags(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"keeps order", []string{"b", "a", "c"}, []string{"b", "a", "c"}},
		{"drops empty", []string{"", " ", "#", "a"}, []string{"a"}},
		{"case-insensitive dedupe keeps first spelling", []string{"Work", "work", "WORK", "#work"}, []string{"Work"}},
		{"dedupe after whitespace collapse", []string{"two words", "Two   Words"}, []string{"two words"}},
		{"umlaut case", []string{"Übung", "übung"}, []string{"Übung"}},
		{"exactly 40 characters", []string{strings.Repeat("a", 40)}, []string{strings.Repeat("a", 40)}},
		{"40 multibyte characters", []string{strings.Repeat("ä", 40)}, []string{strings.Repeat("ä", 40)}},
		{"length counted after normalising", []string{"##  " + strings.Repeat("a", 40) + "  "}, []string{strings.Repeat("a", 40)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalizeTags(c.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestNormalizeTagsTooLong(t *testing.T) {
	long := strings.Repeat("x", MaxTagLength+1)
	_, err := NormalizeTags([]string{"ok", long})
	if err == nil {
		t.Fatal("expected an error for a 41-character tag")
	}
	if !strings.Contains(err.Error(), "too long") || !strings.Contains(err.Error(), long) {
		t.Fatalf("error should name the tag and the problem: %v", err)
	}
}

func TestHasTags(t *testing.T) {
	tags := []string{"Work", "Finance", "two words"}
	cases := []struct {
		name string
		want []string
		ok   bool
	}{
		{"no filter matches", nil, true},
		{"single", []string{"work"}, true},
		{"case-insensitive", []string{"FINANCE"}, true},
		{"and: all present", []string{"work", "finance"}, true},
		{"and: one missing", []string{"work", "private"}, false},
		{"filter normalised", []string{"#Two  Words"}, true},
		{"no substring match", []string{"wor"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasTags(tags, c.want); got != c.ok {
				t.Fatalf("HasTags(%q, %q) = %v, want %v", tags, c.want, got, c.ok)
			}
		})
	}
	if HasTags(nil, []string{"work"}) {
		t.Error("an untagged entry must not match a tag filter")
	}
}

func TestRemoveTags(t *testing.T) {
	in := []string{"Work", "Finance", "dev"}
	got := RemoveTags(in, []string{"work", "#DEV", "missing"})
	if !slices.Equal(got, []string{"Finance"}) {
		t.Fatalf("got %q", got)
	}
	if !slices.Equal(in, []string{"Work", "Finance", "dev"}) {
		t.Fatalf("input modified: %q", in)
	}
}

func TestEditTags(t *testing.T) {
	current := []string{"Work", "Finance"}
	cases := []struct {
		name     string
		clearAll bool
		remove   []string
		add      []string
		want     []string
	}{
		{"no change", false, nil, nil, []string{"Work", "Finance"}},
		{"add appends", false, nil, []string{"dev"}, []string{"Work", "Finance", "dev"}},
		{"existing spelling wins", false, nil, []string{"work", "FINANCE"}, []string{"Work", "Finance"}},
		{"remove ignores case", false, []string{"finance"}, nil, []string{"Work"}},
		{"remove then re-add changes spelling", false, []string{"work"}, []string{"WORK"}, []string{"Finance", "WORK"}},
		{"clear", true, nil, nil, nil},
		{"clear then add", true, nil, []string{"only"}, []string{"only"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := EditTags(current, c.clearAll, c.remove, c.add)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
	if !slices.Equal(current, []string{"Work", "Finance"}) {
		t.Fatalf("current modified: %q", current)
	}
	if _, err := EditTags(nil, false, nil, []string{strings.Repeat("y", 41)}); err == nil {
		t.Fatal("expected an error for a too-long added tag")
	}
}

func TestCountTags(t *testing.T) {
	got := CountTags([][]string{
		{"work", "Finance"},
		nil,
		{"Work", "banking", "work"}, // duplicate on one entry counts once
		{"finance", "#Zeta", "  "},
		{"alpha"},
	})
	want := []TagCount{
		{"alpha", 1},
		{"banking", 1},
		{"Finance", 2}, // first-seen spelling
		{"work", 2},
		{"Zeta", 1}, // case-insensitive sort: Zeta after work
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if CountTags(nil) != nil {
		t.Fatal("no entries → no tags")
	}
}

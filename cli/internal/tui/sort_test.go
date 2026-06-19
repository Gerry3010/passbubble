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

package tui

import (
	"testing"

	vaultpkg "github.com/Gerry3010/passbubble/cli/internal/vault"
)

func ptr(s string) *string { return &s }

// names returns the display names of items in order, for easy assertions.
func names(items []listItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = itemName(it)
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("order mismatch:\n got=%v\nwant=%v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("order mismatch at %d:\n got=%v\nwant=%v", i, got, want)
		}
	}
}

// baseModel builds a model with two root folders and a mix of entries.
func baseModel() Model {
	return Model{
		sortAsc:     true,
		folderFirst: true,
		folders: []*vaultpkg.Folder{
			{ID: "a", Name: "Work", CreatedAt: "2024-01-01T00:00:00Z"},
			{ID: "b", Name: "Banking", CreatedAt: "2024-02-01T00:00:00Z"},
		},
		allEntries: []Entry{
			{ID: "e1", Service: "Zeta", URL: "https://z.example", CreatedAt: "2024-03-01T00:00:00Z", UpdatedAt: "2024-06-01T00:00:00Z"},
			{ID: "e2", Service: "alpha", URL: "https://a.example", CreatedAt: "2024-04-01T00:00:00Z", UpdatedAt: "2024-05-01T00:00:00Z"},
			{ID: "e3", Service: "Inside", FolderID: ptr("a"), CreatedAt: "2024-01-15T00:00:00Z", UpdatedAt: "2024-01-15T00:00:00Z"},
		},
	}
}

func TestCurrentItemsRootLevel(t *testing.T) {
	m := baseModel()
	items := m.currentItems()
	// Two folders + two root entries (e3 is inside folder "a").
	if len(items) != 4 {
		t.Fatalf("want 4 items at root, got %d (%v)", len(items), names(items))
	}
	// Folders are emitted first (before sorting).
	if items[0].kind != folderKind || items[1].kind != folderKind {
		t.Fatalf("folders should come first in currentItems, got %v", names(items))
	}
}

func TestCurrentItemsInsideFolder(t *testing.T) {
	m := baseModel()
	m.folderStack = []*vaultpkg.Folder{m.folders[0]} // drill into "Work" (id a)
	items := m.currentItems()
	got := names(items)
	eq(t, got, []string{"Inside"})
}

func TestSortByNameFolderFirst(t *testing.T) {
	m := baseModel()
	m.sortField = sortByName
	m.sortAsc = true
	m.folderFirst = true
	// Folders sorted (Banking, Work), then entries (alpha, Zeta).
	eq(t, names(m.sortedItems()), []string{"Banking", "Work", "alpha", "Zeta"})
}

func TestSortByNameMixed(t *testing.T) {
	m := baseModel()
	m.sortField = sortByName
	m.sortAsc = true
	m.folderFirst = false
	// Case-insensitive name order across folders + entries:
	// alpha, Banking, Work, Zeta
	eq(t, names(m.sortedItems()), []string{"alpha", "Banking", "Work", "Zeta"})
}

func TestSortByNameDescending(t *testing.T) {
	m := baseModel()
	m.sortField = sortByName
	m.sortAsc = false
	m.folderFirst = true
	// Folders desc (Work, Banking), then entries desc (Zeta, alpha).
	eq(t, names(m.sortedItems()), []string{"Work", "Banking", "Zeta", "alpha"})
}

func TestSortByCreatedMixed(t *testing.T) {
	m := baseModel()
	m.sortField = sortByCreated
	m.sortAsc = true
	m.folderFirst = false
	// CreatedAt: Work 01-01, Banking 02-01, Zeta 03-01, alpha 04-01
	eq(t, names(m.sortedItems()), []string{"Work", "Banking", "Zeta", "alpha"})
}

func TestSortByUpdatedFolderFirst(t *testing.T) {
	m := baseModel()
	m.sortField = sortByUpdated
	m.sortAsc = true
	m.folderFirst = true
	// Folders first (by created fallback: Work 01-01, Banking 02-01),
	// then entries by updated: alpha 05-01, Zeta 06-01.
	eq(t, names(m.sortedItems()), []string{"Work", "Banking", "alpha", "Zeta"})
}

func TestSortByURLMixed(t *testing.T) {
	m := baseModel()
	m.sortField = sortByURL
	m.sortAsc = true
	m.folderFirst = false
	// Folders have empty URL (sort first by tiebreak name: Banking, Work),
	// then entries by url: a.example (alpha), z.example (Zeta).
	eq(t, names(m.sortedItems()), []string{"Banking", "Work", "alpha", "Zeta"})
}

func TestSameFolder(t *testing.T) {
	cases := []struct {
		entry, level *string
		want         bool
	}{
		{nil, nil, true},
		{ptr(""), nil, true},
		{ptr("a"), nil, false},
		{nil, ptr("a"), false},
		{ptr("a"), ptr("a"), true},
		{ptr("a"), ptr("b"), false},
	}
	for i, c := range cases {
		if got := sameFolder(c.entry, c.level); got != c.want {
			t.Errorf("case %d: sameFolder=%v want %v", i, got, c.want)
		}
	}
}

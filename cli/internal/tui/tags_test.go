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
	"strings"
	"testing"
)

func TestDetailScreenShowsTags(t *testing.T) {
	m := NewModel(nil, nil, "")
	m.width = 100
	m.screen = DetailScreen
	m.detailEntry = Entry{ID: "e1", Service: "GitHub", Type: "note"}

	if strings.Contains(m.renderDetailScreen(), "Tags:") {
		t.Fatal("no tags loaded yet → no Tags line")
	}

	next, _ := m.Update(EntryTagsMsg{EntryID: "e1", Tags: []string{"work", "two words"}})
	m = next.(Model)
	if out := m.renderDetailScreen(); !strings.Contains(out, "Tags: work, two words") {
		t.Fatalf("detail screen should list the tags:\n%s", out)
	}

	// A late answer for another entry must not overwrite the current tags.
	next, _ = m.Update(EntryTagsMsg{EntryID: "other", Tags: []string{"stale"}})
	m = next.(Model)
	if strings.Contains(m.renderDetailScreen(), "stale") {
		t.Fatal("tags of a different entry leaked into the detail screen")
	}
}

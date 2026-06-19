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

import "testing"

func TestBuildKeymapDefaults(t *testing.T) {
	km := buildKeymap(nil)
	if km[actAddPassword] != "p" {
		t.Fatalf("default add_password = %q, want p", km[actAddPassword])
	}
	if km[actQuit] != "q" {
		t.Fatalf("default quit = %q, want q", km[actQuit])
	}
}

func TestBuildKeymapOverrideAndUnbind(t *testing.T) {
	km := buildKeymap(map[string]string{
		actAddPassword: "a", // rebind
		actQuit:        "",  // unbind
		"bogus_action": "z", // ignored (unknown action)
	})
	if km[actAddPassword] != "a" {
		t.Errorf("override add_password = %q, want a", km[actAddPassword])
	}
	if km[actQuit] != "" {
		t.Errorf("unbind quit = %q, want empty", km[actQuit])
	}
	if _, ok := km["bogus_action"]; ok {
		t.Errorf("unknown action should not be added to keymap")
	}
}

func TestActionForKey(t *testing.T) {
	m := Model{keymap: defaultKeymap()}
	if a, ok := m.actionForKey("p"); !ok || a != actAddPassword {
		t.Errorf("actionForKey(p) = %q,%v want add_password,true", a, ok)
	}
	// Unbound key resolves to nothing.
	m.keymap[actAddPassword] = ""
	if _, ok := m.actionForKey(""); ok {
		t.Errorf("empty key must never resolve to an action")
	}
}

func TestCurrentItemsFilter(t *testing.T) {
	m := baseModel()
	m.filter = "alp" // matches "alpha" by name
	items := m.currentItems()
	got := names(items)
	eq(t, got, []string{"alpha"})

	// Filter matches across folders, ignoring the current level, and shows no folders.
	m.filter = "example" // matches both entries by URL
	for _, it := range m.currentItems() {
		if it.kind == folderKind {
			t.Fatalf("filtered view must not contain folders")
		}
	}
}

func TestReservedKey(t *testing.T) {
	for _, k := range []string{"up", "down", "enter", "esc", "j", "k"} {
		if !isReservedKey(k) {
			t.Errorf("%q should be reserved", k)
		}
	}
	if isReservedKey("p") {
		t.Errorf("p should not be reserved")
	}
}

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

package config

import (
	"path/filepath"
	"testing"
)

func TestPreferencesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	asc, ff := false, true
	in := &Config{
		ServerURL:   "https://example.test",
		UserID:      "u1",
		SortField:   "updated",
		SortAsc:     &asc,
		FolderFirst: &ff,
		Keybindings: map[string]string{"add_password": "a", "quit": ""},
	}
	if err := in.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out.SortField != "updated" {
		t.Errorf("SortField = %q, want updated", out.SortField)
	}
	if out.SortAsc == nil || *out.SortAsc != false {
		t.Errorf("SortAsc = %v, want false", out.SortAsc)
	}
	if out.FolderFirst == nil || *out.FolderFirst != true {
		t.Errorf("FolderFirst = %v, want true", out.FolderFirst)
	}
	if out.Keybindings["add_password"] != "a" {
		t.Errorf("Keybindings[add_password] = %q, want a", out.Keybindings["add_password"])
	}
	if v, ok := out.Keybindings["quit"]; !ok || v != "" {
		t.Errorf("Keybindings[quit] = %q,%v want empty,true", v, ok)
	}
}

func TestUnsetPreferencesStayNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	in := &Config{ServerURL: "https://example.test", UserID: "u1"}
	if err := in.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out.SortAsc != nil {
		t.Errorf("SortAsc should stay nil when unset, got %v", *out.SortAsc)
	}
	if out.FolderFirst != nil {
		t.Errorf("FolderFirst should stay nil when unset, got %v", *out.FolderFirst)
	}
}

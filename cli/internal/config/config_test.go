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
	"os"
	"path/filepath"
	"strings"
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
	if out.Generator != nil {
		t.Errorf("Generator should stay nil when unset, got %+v", *out.Generator)
	}
	if got := out.GeneratorDefaults(); got != DefaultGeneratorSettings() {
		t.Errorf("GeneratorDefaults() without a block = %+v, want factory defaults", got)
	}
}

func TestGeneratorRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	want := GeneratorSettings{
		Length:         32,
		Lowercase:      true,
		Uppercase:      false,
		Digits:         true,
		Symbols:        true,
		SymbolSet:      `-_!"'#: €🔑`,
		AvoidAmbiguous: true,
	}
	in := &Config{ServerURL: "https://example.test", UserID: "u1", Generator: &want}
	if err := in.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out.Generator == nil {
		t.Fatal("Generator block was not read back")
	}
	if *out.Generator != want {
		t.Errorf("Generator = %+v, want %+v", *out.Generator, want)
	}
	if got := out.GeneratorDefaults(); got != want {
		t.Errorf("GeneratorDefaults() = %+v, want %+v", got, want)
	}

	// Resetting drops the block again.
	out.Generator = nil
	if err := out.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "generator") {
		t.Errorf("reset config still has a generator block:\n%s", raw)
	}
}

func TestGeneratorPartialBlockKeepsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "server_url: https://example.test\ngenerator:\n  symbol_set: \"-_\"\n  uppercase: false\n"
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := DefaultGeneratorSettings()
	want.SymbolSet = "-_"
	want.Uppercase = false
	if got := out.GeneratorDefaults(); got != want {
		t.Errorf("GeneratorDefaults() = %+v, want %+v", got, want)
	}
}

func TestGeneratorSettingsHelpers(t *testing.T) {
	g := DefaultGeneratorSettings()
	if g.Length != 20 || !g.Lowercase || !g.Uppercase || !g.Digits || !g.Symbols || g.AvoidAmbiguous {
		t.Errorf("factory defaults = %+v", g)
	}
	if err := g.Validate(); err != nil {
		t.Errorf("factory defaults should validate: %v", err)
	}
	if got := g.Summary(); got != "20 chars · a-z A-Z 0-9 symbols · avoid ambiguous: off" {
		t.Errorf("Summary() = %q", got)
	}

	g.ApplySymbolSetInput("none")
	if g.Symbols || g.SymbolSetInput() != "none" {
		t.Errorf("\"none\" should turn symbols off: %+v", g)
	}
	if g.SymbolSet != DefaultGeneratorSettings().SymbolSet {
		t.Errorf("turning symbols off should keep the set, got %q", g.SymbolSet)
	}
	g.ApplySymbolSetInput("-_")
	if !g.Symbols || g.SymbolSet != "-_" || g.SymbolSetInput() != "-_" {
		t.Errorf("a set should turn symbols on: %+v", g)
	}
	o := g.Options()
	if o.Length != g.Length || o.SymbolSet != "-_" || !o.Symbols || o.NoAmbiguous {
		t.Errorf("Options() = %+v", o)
	}
}

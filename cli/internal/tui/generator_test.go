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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/pkg/generator"
	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// send feeds keys to the model, replaying the form messages they produce
// the way the bubbletea runtime would.
func send(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, cmd := m.Update(key(k))
		m = next.(Model)
		if cmd == nil {
			continue
		}
		switch msg := cmd().(type) {
		case FormSubmittedMsg, FormCancelledMsg:
			next, _ = m.Update(msg)
			m = next.(Model)
		}
	}
	return m
}

// typeText types s one character at a time.
func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m = send(t, m, string(r))
	}
	return m
}

// clearField deletes the current form field's value.
func clearField(t *testing.T, m Model) Model {
	t.Helper()
	for range []rune(m.form.Fields[m.form.CurrentField].Value) {
		m = send(t, m, "backspace")
	}
	return m
}

func onlyFrom(pw, allowed string) bool {
	for _, r := range pw {
		if !strings.ContainsRune(allowed, r) {
			return false
		}
	}
	return true
}

// recordAdds replaces the add-entry command and records what it receives.
func recordAdds(t *testing.T) *[]map[string]string {
	t.Helper()
	var got []map[string]string
	orig := addPasswordCmd
	addPasswordCmd = func(fields map[string]string) tea.Cmd {
		got = append(got, fields)
		return nil
	}
	t.Cleanup(func() { addPasswordCmd = orig })
	return &got
}

func newTestModel(t *testing.T, cfg *config.Config) (Model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.yaml")
	m := NewModel(nil, cfg, path)
	m.screen = MainScreen
	m.width = 100
	return m, path
}

func TestGenerateScreenUsesDefaultsAndOneOffOptions(t *testing.T) {
	saved := config.DefaultGeneratorSettings()
	saved.Length, saved.SymbolSet, saved.Uppercase = 30, "-_", false
	cfg := &config.Config{Generator: &saved}
	m, path := newTestModel(t, cfg)

	next, _ := m.runAction(actGenerate)
	m = next.(Model)
	if m.screen != GenerateScreen {
		t.Fatalf("screen = %v, want GenerateScreen", m.screen)
	}
	pw := m.generatedPassword
	if utf8.RuneCountInString(pw) != 30 || !onlyFrom(pw, generator.LowercaseChars+generator.DigitChars+"-_") {
		t.Fatalf("password %q ignores the saved defaults", pw)
	}
	out := m.renderGenerateScreen()
	for _, want := range []string{"30 chars · a-z 0-9 symbols", "Symbol set: -_", "Strength:"} {
		if !strings.Contains(out, want) {
			t.Errorf("generate screen should show %q:\n%s", want, out)
		}
	}

	m = send(t, m, "r")
	if m.generatedPassword == pw {
		t.Errorf("r should regenerate, still %q", pw)
	}

	// o: length 40 and symbol set € for this screen only.
	m = send(t, m, "o")
	if !m.showingForm || m.form.Type != GeneratorOptionsForm {
		t.Fatal("o should open the options form")
	}
	if m.form.Fields[0].Value != "30" || m.form.Fields[1].Value != "-_" {
		t.Errorf("options form should start with the defaults, got %q / %q", m.form.Fields[0].Value, m.form.Fields[1].Value)
	}
	m = clearField(t, m)
	m = typeText(t, m, "40")
	m = send(t, m, "tab")
	m = clearField(t, m)
	m = typeText(t, m, "€")
	m = send(t, m, "enter")
	if m.showingForm {
		t.Fatalf("form should be closed, error: %q", m.form.Error)
	}
	pw = m.generatedPassword
	if utf8.RuneCountInString(pw) != 40 || !onlyFrom(pw, generator.LowercaseChars+generator.DigitChars+"€") || !strings.Contains(pw, "€") {
		t.Errorf("password %q ignores the one-off options", pw)
	}

	// The saved defaults stay as they were.
	if *cfg.Generator != saved {
		t.Errorf("one-off options changed the defaults: %+v", *cfg.Generator)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("one-off options must not write the config (stat: %v)", err)
	}
}

func TestOptionsFormRejectsEmptySymbolSet(t *testing.T) {
	m, _ := newTestModel(t, &config.Config{})
	next, _ := m.runAction(actGenerate)
	m = next.(Model)

	m = send(t, m, "o", "tab")
	m = clearField(t, m)
	m = send(t, m, "enter")
	if !m.showingForm || !strings.Contains(m.form.Error, `"none"`) {
		t.Fatalf("an empty symbol set should be reported in the form, error = %q", m.form.Error)
	}
	m = typeText(t, m, "none")
	m = send(t, m, "enter")
	if m.showingForm {
		t.Fatalf("form should be closed, error: %q", m.form.Error)
	}
	if pw := m.generatedPassword; !onlyFrom(pw, generator.LowercaseChars+generator.UppercaseChars+generator.DigitChars) {
		t.Errorf(`"none" should turn symbols off: %q`, pw)
	}
}

func TestAddFormConfirmsGeneratedPassword(t *testing.T) {
	adds := recordAdds(t)
	m, _ := newTestModel(t, &config.Config{})

	next, _ := m.runAction(actAddPassword)
	m = next.(Model)
	m = typeText(t, m, "GitHub")
	m = send(t, m, "tab", "tab", "enter") // empty password field
	if !m.showGenConfirm || m.showingForm {
		t.Fatalf("an empty password should open the confirm step (form error %q)", m.form.Error)
	}
	if len(*adds) != 0 {
		t.Fatal("nothing may be stored before Enter")
	}
	if n := utf8.RuneCountInString(m.genConfirmPW); n != generator.DefaultLength {
		t.Errorf("generated length %d, want %d", n, generator.DefaultLength)
	}
	out := m.View()
	for _, want := range []string{"GitHub", "[Enter] use · r regenerate · o options · q cancel", m.genConfirmPW} {
		if !strings.Contains(out, want) {
			t.Errorf("confirm step should show %q:\n%s", want, out)
		}
	}

	first := m.genConfirmPW
	m = send(t, m, "r")
	if m.genConfirmPW == first {
		t.Error("r should regenerate")
	}

	// o changes the length for this password; Enter in the form goes back.
	m = send(t, m, "o")
	if !m.showingForm || m.form.Type != GeneratorOptionsForm {
		t.Fatal("o should open the options form")
	}
	m = clearField(t, m)
	m = typeText(t, m, "24")
	m = send(t, m, "enter")
	if m.showingForm || !m.showGenConfirm {
		t.Fatalf("options should return to the confirm step (form error %q)", m.form.Error)
	}
	if n := utf8.RuneCountInString(m.genConfirmPW); n != 24 {
		t.Errorf("length after options = %d, want 24", n)
	}

	// q goes back to the form with the password field untouched.
	m = send(t, m, "q")
	if m.showGenConfirm || !m.showingForm || m.form.Type != AddPasswordForm {
		t.Fatal("q should return to the add form")
	}
	if m.form.Fields[0].Value != "GitHub" || m.form.Fields[2].Value != "" {
		t.Errorf("the add form should be restored as it was: %+v", m.form.Fields)
	}
	if len(*adds) != 0 {
		t.Fatal("cancel must not store anything")
	}

	// Submitting again starts over from the defaults; only Enter stores.
	m = send(t, m, "enter")
	if !m.showGenConfirm {
		t.Fatal("submitting again should show the confirm step")
	}
	if n := utf8.RuneCountInString(m.genConfirmPW); n != generator.DefaultLength {
		t.Errorf("one-off length leaked into the next run: %d", n)
	}
	pw := m.genConfirmPW
	m = send(t, m, "x") // unknown keys do nothing
	m = send(t, m, "enter")
	if m.showGenConfirm {
		t.Error("Enter should close the confirm step")
	}
	if len(*adds) != 1 || (*adds)[0]["password"] != pw || (*adds)[0]["service"] != "GitHub" {
		t.Fatalf("stored %v, want GitHub with %q", *adds, pw)
	}
}

func TestAddFormTypedPasswordSkipsConfirm(t *testing.T) {
	adds := recordAdds(t)
	m, _ := newTestModel(t, &config.Config{})

	next, _ := m.runAction(actAddPassword)
	m = next.(Model)
	m = typeText(t, m, "GitHub")
	m = send(t, m, "tab", "tab")
	m = typeText(t, m, "hunter2!")
	m = send(t, m, "enter")
	if m.showGenConfirm {
		t.Fatal("a typed password needs no confirm step")
	}
	if len(*adds) != 1 || (*adds)[0]["password"] != "hunter2!" {
		t.Fatalf("stored %v", *adds)
	}
}

func TestSettingsEditAndResetGeneratorDefaults(t *testing.T) {
	cfg := &config.Config{ServerURL: "https://example.test"}
	m, path := newTestModel(t, cfg)
	m.screen = SettingsScreen
	if out := m.renderSettingsScreen(); !strings.Contains(out, "20 chars · a-z A-Z 0-9 symbols") {
		t.Errorf("settings should show the factory defaults:\n%s", out)
	}

	m = send(t, m, "g")
	if !m.showingForm || m.form.Type != GeneratorDefaultsForm || len(m.form.Fields) != 7 {
		t.Fatal("g should open the generator defaults form")
	}

	// All classes off is rejected inside the form; Esc saves nothing.
	for i := 1; i <= 4; i++ {
		m = send(t, m, "tab")
		m = clearField(t, m)
		m = typeText(t, m, "n")
	}
	m = send(t, m, "enter")
	if !m.showingForm || !strings.Contains(m.form.Error, generator.ErrNoClasses.Error()) {
		t.Fatalf("all classes off should be reported, error = %q", m.form.Error)
	}
	m = send(t, m, "esc")
	if _, err := os.Stat(path); !os.IsNotExist(err) || cfg.Generator != nil {
		t.Fatalf("a cancelled form must not save (stat: %v)", err)
	}

	m = send(t, m, "g")
	m = clearField(t, m)
	m = typeText(t, m, "24")
	m = send(t, m, "tab", "tab") // uppercase
	m = clearField(t, m)
	m = typeText(t, m, "n")
	m = send(t, m, "tab", "tab", "tab") // symbol set
	m = clearField(t, m)
	m = typeText(t, m, "-_.€")
	m = send(t, m, "enter")
	if m.showingForm {
		t.Fatalf("form should be closed, error: %q", m.form.Error)
	}

	want := config.DefaultGeneratorSettings()
	want.Length, want.Uppercase, want.SymbolSet = 24, false, "-_.€"
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Generator == nil || *loaded.Generator != want {
		t.Fatalf("saved %+v, want %+v", loaded.Generator, want)
	}
	if loaded.ServerURL != "https://example.test" {
		t.Errorf("saving lost the server URL: %q", loaded.ServerURL)
	}

	// The generate screen now starts from the new defaults.
	m.screen = MainScreen
	next, _ := m.runAction(actGenerate)
	if pw := next.(Model).generatedPassword; utf8.RuneCountInString(pw) != 24 || strings.ContainsAny(pw, generator.UppercaseChars) {
		t.Errorf("generate screen ignores the new defaults: %q", pw)
	}

	m.screen = SettingsScreen
	m = send(t, m, "G")
	if cfg.Generator != nil {
		t.Errorf("G should reset the defaults, got %+v", *cfg.Generator)
	}
	if loaded, _ := config.Load(path); loaded.Generator != nil {
		t.Errorf("reset was not saved: %+v", *loaded.Generator)
	}
}

func TestFormInputHandlesMultiByteCharacters(t *testing.T) {
	m, _ := newTestModel(t, &config.Config{})
	m.showingForm = true
	m.form = CreateNewFolderForm(nil)
	m = typeText(t, m, "€ä")
	m = send(t, m, " ")
	m = send(t, m, "backspace", "backspace")
	if got := m.form.Fields[0].Value; got != "€" {
		t.Errorf("field = %q, want €", got)
	}
}

func TestLockClearsPendingGeneratedPassword(t *testing.T) {
	m, _ := newTestModel(t, &config.Config{})
	next, _ := m.runAction(actAddPassword)
	m = next.(Model)
	m = typeText(t, m, "GitHub")
	m = send(t, m, "enter")
	if !m.showGenConfirm {
		t.Fatal("expected the confirm step")
	}
	m = m.lockVault()
	if m.showGenConfirm || m.genConfirmPW != "" || m.genFields != nil {
		t.Error("locking must drop the pending generated password")
	}
	if m.screen != UnlockScreen {
		t.Errorf("screen = %v, want UnlockScreen", m.screen)
	}
}

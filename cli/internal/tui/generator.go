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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/pkg/generator"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// legacyStyleLength keeps the memorable and passphrase styles at the length
// they always had; the generator options only shape random passwords.
const legacyStyleLength = 16

// addPasswordCmd stores a new password entry. A variable so tests can see
// what the add flow submits without touching a vault.
var addPasswordCmd = handleAddPassword

// generatorDefaults returns the saved generator defaults, or the factory
// defaults without a config.
func (m Model) generatorDefaults() config.GeneratorSettings {
	return m.cfg.GeneratorDefaults()
}

// generateStyle produces one password: "memorable" and "passphrase" use
// their fixed styles, anything else is a random password from s.
func generateStyle(style string, s config.GeneratorSettings) (string, error) {
	var opts *generator.Options
	switch style {
	case "memorable":
		opts = generator.DefaultOptions()
		opts.Type = generator.Memorable
		opts.Length = legacyStyleLength
	case "passphrase":
		opts = generator.DefaultOptions()
		opts.Type = generator.Passphrase
		opts.Length = legacyStyleLength
	default:
		opts = s.Options()
	}
	passwords, err := generator.New(opts).Generate()
	if err != nil {
		return "", err
	}
	return passwords[0], nil
}

// generateOnScreen fills the generate screen with a new password.
func (m Model) generateOnScreen(style string) Model {
	pw, err := generateStyle(style, m.genSettings)
	if err != nil {
		m.status = fmt.Sprintf("Password generation failed: %v", err)
		m.statusType = "error"
		return m
	}
	m.generatedPassword = pw
	m.generateType = style
	m.status = fmt.Sprintf("%s password generated", style)
	m.statusType = "success"
	return m
}

// startGenConfirm shows a generated password for an add form that was
// submitted without one. Nothing is stored until the user presses Enter.
func (m Model) startGenConfirm(msg FormSubmittedMsg) Model {
	form := m.form
	form.Submitted = false
	// Own copy of the fields: forms edit them in place.
	form.Fields = append([]FormField(nil), form.Fields...)
	m.genSettings = m.generatorDefaults()
	pw, err := generateStyle("random", m.genSettings)
	if err != nil {
		// Back to the form, so the user can type a password instead.
		form.Error = fmt.Sprintf("cannot generate a password: %v (check the generator defaults in settings)", err)
		m.form = form
		m.showingForm = true
		return m
	}
	m.genReturnForm = form
	m.genFields = msg.Fields
	m.genConfirmPW = pw
	m.showGenConfirm = true
	m.status = "Password generated — Enter to use it"
	m.statusType = "info"
	return m
}

// clearGenConfirm drops the confirm step and its pending password.
func (m *Model) clearGenConfirm() {
	m.showGenConfirm = false
	m.genConfirmPW = ""
	m.genFields = nil
	m.genReturnForm = FormModel{}
}

// handleGenConfirm drives the confirm step: only Enter takes the password.
func (m Model) handleGenConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "enter":
		fields := make(map[string]string, len(m.genFields)+1)
		for k, v := range m.genFields {
			fields[k] = v
		}
		fields["password"] = m.genConfirmPW
		m.clearGenConfirm()
		return m, addPasswordCmd(fields)

	case "r":
		pw, err := generateStyle("random", m.genSettings)
		if err != nil {
			m.status = fmt.Sprintf("Password generation failed: %v", err)
			m.statusType = "error"
			return m, nil
		}
		m.genConfirmPW = pw
		m.status = "Password regenerated"
		m.statusType = "info"
		return m, nil

	case "o":
		m.showingForm = true
		m.form = CreateGeneratorOptionsForm(m.genSettings)
		m.form.width = m.width
		m.status = "Generator options for this password"
		m.statusType = "info"
		return m, nil

	case "q", "esc":
		// Cancel leaves the password field as it was: back to the add form.
		form := m.genReturnForm
		m.clearGenConfirm()
		m.form = form
		m.showingForm = true
		m.status = "Generated password discarded"
		m.statusType = "info"
		return m, nil
	}
	return m, nil
}

// applyGeneratorOptionsForm takes the one-off options and regenerates the
// password of whatever asked for them (confirm step or generate screen).
func (m Model) applyGeneratorOptionsForm(fields map[string]string) Model {
	s, err := applyOneOffOptions(m.genSettings, fields)
	if err != nil {
		m.status = err.Error()
		m.statusType = "error"
		return m
	}
	m.genSettings = s
	if m.showGenConfirm {
		pw, err := generateStyle("random", s)
		if err != nil {
			m.status = fmt.Sprintf("Password generation failed: %v", err)
			m.statusType = "error"
			return m
		}
		m.genConfirmPW = pw
		m.status = "Options applied to this password"
		m.statusType = "info"
		return m
	}
	return m.generateOnScreen("random")
}

// saveGeneratorDefaultsForm stores the settings form as the new defaults.
func (m Model) saveGeneratorDefaultsForm(fields map[string]string) Model {
	s, err := parseGeneratorDefaults(fields)
	if err == nil && m.cfg == nil {
		err = errors.New("no config loaded")
	}
	if err == nil {
		m.cfg.Generator = &s
		err = m.cfg.Save(m.cfgPath)
	}
	if err != nil {
		m.status = fmt.Sprintf("Generator defaults not saved: %v", err)
		m.statusType = "error"
		return m
	}
	m.status = "Generator defaults saved: " + s.Summary()
	m.statusType = "success"
	return m
}

// resetGeneratorDefaults drops the saved defaults (factory settings apply).
func (m Model) resetGeneratorDefaults() Model {
	if m.cfg == nil {
		return m
	}
	m.cfg.Generator = nil
	if err := m.cfg.Save(m.cfgPath); err != nil {
		m.status = fmt.Sprintf("Generator defaults not reset: %v", err)
		m.statusType = "error"
		return m
	}
	m.status = "Generator defaults reset to the factory settings"
	m.statusType = "success"
	return m
}

// --- Forms ---

func generatorLengthField(length int) FormField {
	return FormField{
		Label:      fmt.Sprintf("Length (%d-%d)", generator.MinLength, generator.MaxLength),
		Key:        "length",
		Value:      strconv.Itoa(length),
		IsRequired: true,
		Validator: func(s string) error {
			_, err := parseGeneratorLength(s)
			return err
		},
	}
}

func parseGeneratorLength(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < generator.MinLength || n > generator.MaxLength {
		return 0, fmt.Errorf("length must be a number between %d and %d", generator.MinLength, generator.MaxLength)
	}
	return n, nil
}

// CreateGeneratorOptionsForm asks for length and symbol set for one
// password; the saved defaults stay untouched.
func CreateGeneratorOptionsForm(s config.GeneratorSettings) FormModel {
	return FormModel{
		Type:  GeneratorOptionsForm,
		Title: "Generator options (this time only)",
		Fields: []FormField{
			generatorLengthField(s.Length),
			{
				Label:       fmt.Sprintf("Symbol set (%q turns symbols off)", config.SymbolsOffKeyword),
				Key:         "symbol_set",
				Value:       s.SymbolSetInput(),
				Placeholder: "symbols to draw from",
			},
		},
		Check: func(fields map[string]string) error {
			_, err := applyOneOffOptions(s, fields)
			return err
		},
	}
}

// applyOneOffOptions applies the options form to s and validates the result.
func applyOneOffOptions(s config.GeneratorSettings, fields map[string]string) (config.GeneratorSettings, error) {
	n, err := parseGeneratorLength(fields["length"])
	if err != nil {
		return s, err
	}
	s.Length = n
	s.ApplySymbolSetInput(fields["symbol_set"])
	if err := s.Validate(); err != nil {
		if errors.Is(err, generator.ErrEmptyClass) && s.Symbols {
			return s, fmt.Errorf("%v — enter symbols or %q", err, config.SymbolsOffKeyword)
		}
		return s, err
	}
	return s, nil
}

func yesNoField(label, key string, on bool) FormField {
	return FormField{
		Label:      label + " (y/n)",
		Key:        key,
		Value:      formatYesNo(on),
		IsRequired: true,
		Validator: func(s string) error {
			_, err := parseYesNo(s)
			return err
		},
	}
}

func formatYesNo(on bool) string {
	if on {
		return "y"
	}
	return "n"
}

func parseYesNo(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes", "on", "true":
		return true, nil
	case "n", "no", "off", "false":
		return false, nil
	}
	return false, fmt.Errorf("answer y or n")
}

// CreateGeneratorDefaultsForm edits the generator defaults saved in the
// config.
func CreateGeneratorDefaultsForm(s config.GeneratorSettings) FormModel {
	return FormModel{
		Type:  GeneratorDefaultsForm,
		Title: "Password generator defaults",
		Fields: []FormField{
			generatorLengthField(s.Length),
			yesNoField("Lowercase a-z", "lowercase", s.Lowercase),
			yesNoField("Uppercase A-Z", "uppercase", s.Uppercase),
			yesNoField("Digits 0-9", "digits", s.Digits),
			yesNoField("Symbols", "symbols", s.Symbols),
			{
				Label:       "Symbol set",
				Key:         "symbol_set",
				Value:       s.SymbolSet,
				Placeholder: "symbols to draw from",
			},
			yesNoField("Avoid ambiguous characters "+generator.AmbiguousChars, "avoid_ambiguous", s.AvoidAmbiguous),
		},
		Check: func(fields map[string]string) error {
			_, err := parseGeneratorDefaults(fields)
			return err
		},
	}
}

// parseGeneratorDefaults reads the defaults form and validates the result.
func parseGeneratorDefaults(fields map[string]string) (config.GeneratorSettings, error) {
	var s config.GeneratorSettings
	var err error
	if s.Length, err = parseGeneratorLength(fields["length"]); err != nil {
		return s, err
	}
	for _, f := range []struct {
		key string
		dst *bool
	}{
		{"lowercase", &s.Lowercase},
		{"uppercase", &s.Uppercase},
		{"digits", &s.Digits},
		{"symbols", &s.Symbols},
		{"avoid_ambiguous", &s.AvoidAmbiguous},
	} {
		if *f.dst, err = parseYesNo(fields[f.key]); err != nil {
			return s, fmt.Errorf("%s: %w", f.key, err)
		}
	}
	s.SymbolSet = fields["symbol_set"]
	return s, s.Validate()
}

// --- Rendering ---

// renderStrength renders the local strength score of pw as text and a bar.
func (m Model) renderStrength(pw string) string {
	res := generator.CheckStrength(pw)
	score := res.Score
	if score < 0 {
		score = 0
	}
	return fmt.Sprintf("Strength: %s (%d/100) %s", res.Level, res.Score, m.renderProgressBar(score, 100))
}

// renderGenOptions describes the options a random password is built from.
func (m Model) renderGenOptions(s config.GeneratorSettings) []string {
	lines := []string{"Options: " + s.Summary()}
	if s.Symbols {
		lines = append(lines, "Symbol set: "+generator.CleanSymbolSet(s.SymbolSet))
	}
	return lines
}

// renderGenConfirm renders the confirm step of the add form.
func (m Model) renderGenConfirm() string {
	title := m.titleStyle.Render("passbubble:~$ generate")

	service := strings.TrimSpace(m.genFields["service"])
	var content []string
	content = append(content, fmt.Sprintf("Generated password for %s:", service), "")
	content = append(content, fmt.Sprintf("Password: %s", m.secretStyle.Render(m.genConfirmPW)))
	content = append(content, m.renderStrength(m.genConfirmPW), "")
	content = append(content, m.renderGenOptions(m.genSettings)...)

	detailWidth := m.width - 8
	if detailWidth < 40 {
		detailWidth = 40
	}
	box := m.detailStyle.Width(detailWidth).Render(strings.Join(content, "\n"))
	help := m.helpStyle.Render("[Enter] use · r regenerate · o options · q cancel")

	return lipgloss.JoinVertical(lipgloss.Left, title, "", box, "", help, m.renderStatus())
}

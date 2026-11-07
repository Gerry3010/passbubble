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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Gerry3010/passbubble/cli/pkg/totp"
)

// TOTPUpdateMsg represents a TOTP code update
type TOTPUpdateMsg struct {
	EntryID   string
	Code      string
	Remaining int
	Error     error
}

// SecretLoadMsg represents loaded secret data
type SecretLoadMsg struct {
	EntryID  string
	Password string
	HasTOTP  bool // the entry holds a TOTP secret
	Error    error
}

// Enhanced methods with real integrations. Both go by the entry's ID: names are
// not unique, and a list row does not know the username.

// updateTOTPReal updates the TOTP code from the entry's secret and settings.
func (m Model) updateTOTPReal() tea.Cmd {
	if !m.detailShowsCode() {
		return nil
	}
	v, id := m.vault, m.detailEntry.ID

	return func() tea.Msg {
		full, err := fetchEntry(v, id)
		if err != nil {
			return TOTPUpdateMsg{EntryID: id, Error: fmt.Errorf("failed to load TOTP secret: %w", err)}
		}
		// An empty secret would produce a valid-looking code.
		if full.Data.TOTPSecret == "" {
			return TOTPUpdateMsg{EntryID: id, Error: errors.New("entry has no TOTP secret")}
		}

		opts := totp.StoredOptions(full.Data.Period, full.Data.Digits, full.Data.Algorithm)
		code, err := totp.GenerateCode(full.Data.TOTPSecret, opts)
		if err != nil {
			return TOTPUpdateMsg{EntryID: id, Error: fmt.Errorf("failed to generate TOTP code: %w", err)}
		}

		return TOTPUpdateMsg{
			EntryID:   id,
			Code:      totp.FormatCode(code),
			Remaining: totp.GetTimeRemaining(opts.Period),
		}
	}
}

// detailShowsCode reports whether the detail entry has a TOTP code to show: a TOTP
// entry by its type, a login once its decrypted data turned out to hold a secret.
func (m Model) detailShowsCode() bool {
	return m.detailEntry.Type == "totp" || m.detailHasTOTP
}

// loadPasswordReal loads the password and what the detail screen needs to know
// about the TOTP secret. Both kinds of login can hold both.
func (m Model) loadPasswordReal() tea.Cmd {
	if m.detailEntry.Type != "password" && m.detailEntry.Type != "totp" {
		return nil
	}
	v, id := m.vault, m.detailEntry.ID

	return func() tea.Msg {
		full, err := fetchEntry(v, id)
		if err != nil {
			return SecretLoadMsg{EntryID: id, Error: fmt.Errorf("failed to load password: %w", err)}
		}

		return SecretLoadMsg{EntryID: id, Password: full.Data.Password, HasTOTP: full.Data.TOTPSecret != ""}
	}
}

// Enhanced update method that handles real data
func (m Model) UpdateEnhanced(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TOTPUpdateMsg:
		if msg.Error != nil {
			m.status = fmt.Sprintf("TOTP Error: %v", msg.Error)
			return m, nil
		}
		
		m.totpCode = msg.Code
		m.totpRemaining = msg.Remaining
		return m, nil
		
	case SecretLoadMsg:
		if msg.Error != nil {
			m.status = fmt.Sprintf("Secret Error: %v", msg.Error)
			return m, nil
		}
		
		// Store password securely (in real implementation, this would be handled more carefully)
		m.status = "Password loaded successfully"
		return m, nil
		
	default:
		return m.Update(msg)
	}
}



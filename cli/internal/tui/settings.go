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
	"fmt"
	"strings"
	"time"

	vaultpkg "github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/Gerry3010/passbubble/cli/pkg/keyring"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// handleSettingsScreen drives the settings screen, including the keybinding editor.
func (m Model) handleSettingsScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Capturing mode: the next key (re)binds the selected action; Esc unbinds it.
	if m.kbCapturing {
		switch key {
		case "esc":
			m.setBinding(actionOrder[m.kbCursor], "") // unbind
			m.kbCapturing = false
			m.status = "Unbound " + actionLabels[actionOrder[m.kbCursor]]
			m.statusType = "info"
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		default:
			if isReservedKey(key) {
				m.status = fmt.Sprintf("%q is reserved for navigation — pick another key", key)
				m.statusType = "error"
				m.kbCapturing = false
				return m, nil
			}
			m.setBinding(actionOrder[m.kbCursor], key)
			m.kbCapturing = false
			m.status = fmt.Sprintf("Bound %s → %s", actionLabels[actionOrder[m.kbCursor]], key)
			m.statusType = "success"
			return m, nil
		}
	}

	switch key {
	case "esc", "q":
		m.screen = MainScreen
		return m, nil

	case "up", "k":
		if m.kbCursor > 0 {
			m.kbCursor--
		}
		return m, nil

	case "down", "j":
		if m.kbCursor < len(actionOrder)-1 {
			m.kbCursor++
		}
		return m, nil

	case "enter":
		// Begin capturing a new key for the selected action.
		m.kbCapturing = true
		m.status = "Press a key to bind (Esc to unbind)…"
		m.statusType = "info"
		return m, nil

	case "t":
		// Cycle the auto-lock idle timeout through the presets and persist it.
		m.cycleLogoutInterval()
		return m, nil

	case "l":
		return m.lockVault(), nil

	case "o":
		return m.logout()
	}
	return m, nil
}

// logoutPresets are the selectable auto-lock idle timeouts, in minutes.
// 0 means "off" (auto-lock disabled).
var logoutPresets = []int{0, 1, 5, 10, 15, 30, 60}

// cycleLogoutInterval advances the auto-lock timeout to the next preset,
// applies it to the live model and persists it to the config.
func (m *Model) cycleLogoutInterval() {
	cur := int(m.idleTimeout / time.Minute)
	next := logoutPresets[0]
	for i, p := range logoutPresets {
		if p == cur {
			next = logoutPresets[(i+1)%len(logoutPresets)]
			break
		}
	}
	m.idleTimeout = time.Duration(next) * time.Minute
	if m.cfg != nil {
		m.cfg.LogoutInterval = &next
		_ = m.cfg.Save(m.cfgPath)
	}
	if next == 0 {
		m.status = "Auto-lock disabled"
	} else {
		m.status = fmt.Sprintf("Auto-lock set to %s", formatTimeout(m.idleTimeout))
	}
	m.statusType = "success"
}

// formatTimeout renders an idle timeout for display ("off" when disabled).
func formatTimeout(d time.Duration) string {
	if d <= 0 {
		return "off"
	}
	mins := int(d / time.Minute)
	if mins == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", mins)
}

// setBinding updates the in-memory keymap + config and persists it.
func (m *Model) setBinding(action, key string) {
	if m.keymap == nil {
		m.keymap = defaultKeymap()
	}
	// Clear any other action currently holding this key to avoid duplicates.
	if key != "" {
		for a, k := range m.keymap {
			if k == key && a != action {
				m.keymap[a] = ""
			}
		}
	}
	m.keymap[action] = key
	if m.cfg != nil {
		if m.cfg.Keybindings == nil {
			m.cfg.Keybindings = map[string]string{}
		}
		// Persist the full resolved map so unbinds survive restarts.
		for a, k := range m.keymap {
			m.cfg.Keybindings[a] = k
		}
		_ = m.cfg.Save(m.cfgPath)
	}
}

// isReservedKey reports whether a key is fixed for navigation and not bindable.
func isReservedKey(key string) bool {
	switch key {
	case "up", "down", "left", "right", "k", "j", "h", "l",
		"enter", "esc", "backspace", "tab", "shift+tab", "ctrl+c":
		return true
	}
	return false
}

// renderHelp renders the keybinding/help overlay.
func (m Model) renderHelp() string {
	var b strings.Builder
	b.WriteString(m.titleStyle.Render("⌨  Keybindings"))
	b.WriteString("\n\n")
	for _, action := range actionOrder {
		key := m.keymap[action]
		if key == "" {
			key = "—"
		}
		fmt.Fprintf(&b, "  %-22s %s\n", actionLabels[action], key)
	}
	b.WriteString("\n")
	b.WriteString("  Navigation (fixed): ↑/↓ j/k  Enter/→ open  Esc/← up\n")
	b.WriteString("\n")
	b.WriteString(m.helpStyle.Render("Press any key to close"))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Render(b.String())
}

// lockVault clears the in-memory keys and returns to the unlock screen.
func (m Model) lockVault() Model {
	if m.vault != nil {
		m.vault.Lock()
	}
	m.screen = UnlockScreen
	m.authFields = newUnlockFields()
	m.authCursor = 0
	m.authErr = ""
	m.authBusy = false
	// Drop loaded data so a locked vault never shows stale entries.
	m.allEntries = nil
	m.folders = nil
	m.folderStack = nil
	m.cursor = 0
	m.listOffset = 0
	return m
}

// logout clears credentials, returns to the login screen, and fires a
// best-effort server-side logout in the background.
func (m Model) logout() (tea.Model, tea.Cmd) {
	refresh := ""
	if m.cfg != nil {
		refresh = m.cfg.RefreshToken
	}
	v := m.vault
	if m.cfg != nil {
		m.cfg.Clear()
		_ = m.cfg.Save(m.cfgPath)
	}
	if v != nil {
		v.Lock()
	}
	keyring.SetGlobal(nil)
	m.allEntries = nil
	m.folders = nil
	m.folderStack = nil
	m = m.gotoLogin()
	return m, logoutAPICmd(v, refresh)
}

// logoutAPICmd best-effort revokes the refresh token server-side.
func logoutAPICmd(v *vaultpkg.Vault, refresh string) tea.Cmd {
	return func() tea.Msg {
		if v != nil && refresh != "" {
			_ = v.Client().Logout(refresh)
		}
		return nil
	}
}

// renderSettingsScreen renders the settings screen.
func (m Model) renderSettingsScreen() string {
	var b strings.Builder
	b.WriteString(m.titleStyle.Render("⚙️  Settings"))
	b.WriteString("\n\n")

	server, email, userID := "—", "—", "—"
	if m.cfg != nil {
		if m.cfg.ServerURL != "" {
			server = m.cfg.ServerURL
		}
		if m.cfg.Email != "" {
			email = m.cfg.Email
		}
		if m.cfg.UserID != "" {
			userID = m.cfg.UserID
		}
	}
	locked := "unlocked"
	if m.vault == nil || !m.vault.IsUnlocked() {
		locked = "locked"
	}

	fmt.Fprintf(&b, "Account\n  Email:   %s\n  User ID: %s\n\n", email, userID)
	fmt.Fprintf(&b, "Server\n  URL:     %s\n  Vault:   %s\n\n", server, locked)
	fmt.Fprintf(&b, "Security\n  Auto-lock: %s  (press 't' to change)\n\n", formatTimeout(m.idleTimeout))

	b.WriteString(m.titleStyle.Render("Keybindings"))
	b.WriteString("\n")
	if m.kbCapturing {
		b.WriteString(m.helpStyle.Render("  Press a key to bind, or Esc to unbind…"))
		b.WriteString("\n")
	}
	for i, action := range actionOrder {
		cursor := "  "
		if i == m.kbCursor {
			cursor = "> "
		}
		key := m.keymap[action]
		if key == "" {
			key = "—"
		}
		line := fmt.Sprintf("%s%-22s %s", cursor, actionLabels[action], key)
		if i == m.kbCursor {
			line = m.selectedStyle.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.helpStyle.Render("↑/↓: select  Enter: rebind  (then Esc: unbind)  t: auto-lock  l: lock  o: log out  q: back"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Render(b.String())
	return box
}

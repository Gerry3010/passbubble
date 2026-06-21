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
	"strconv"
	"strings"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
	vaultpkg "github.com/Gerry3010/passbubble/cli/internal/vault"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// authField is one input row on the login/register/unlock screens.
type authField struct {
	key    string
	label  string
	value  string
	hidden bool // mask the value (passwords)
}

// AuthResultMsg is produced by the async login/register/unlock commands.
type AuthResultMsg struct {
	vault *vaultpkg.Vault
	cfg   *config.Config
	err   error
}

func newLoginFields(cfg *config.Config) []authField {
	server := ""
	if cfg != nil {
		server = cfg.ServerURL
	}
	return []authField{
		{key: "server", label: "Server URL", value: server},
		{key: "email", label: "Email"},
		{key: "password", label: "Master password", hidden: true},
	}
}

func newRegisterFields(cfg *config.Config) []authField {
	server := ""
	if cfg != nil {
		server = cfg.ServerURL
	}
	return []authField{
		{key: "server", label: "Server URL", value: server},
		{key: "token", label: "Invitation token"},
		{key: "name", label: "Display name"},
		{key: "email", label: "Email"},
		{key: "password", label: "Master password", hidden: true},
		{key: "password2", label: "Confirm master password", hidden: true},
	}
}

func newUnlockFields() []authField {
	return []authField{
		{key: "password", label: "Master password", hidden: true},
	}
}

func newPINUnlockFields() []authField {
	return []authField{
		{key: "pin", label: "PIN", hidden: true},
	}
}

func newPINSetupFields() []authField {
	return []authField{
		{key: "password", label: "Master password", hidden: true},
		{key: "pin", label: "New PIN (digits)", hidden: true},
		{key: "pin2", label: "Confirm PIN", hidden: true},
		{key: "interval", label: "Require master password after N days (1–60)", value: "14"},
	}
}

// PINConfigMsg is produced by the async PIN enable/disable commands.
type PINConfigMsg struct {
	status string
	err    error
}

// authValue returns the current value of an auth field by key.
func (m Model) authValue(key string) string {
	for _, f := range m.authFields {
		if f.key == key {
			return strings.TrimSpace(f.value)
		}
	}
	return ""
}

// handleAuthScreen drives the login/register/unlock screens.
func (m Model) handleAuthScreen(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.authBusy {
		return m, nil // ignore input while a request is in flight
	}

	switch msg.String() {
	case "esc":
		if m.screen == RegisterScreen {
			return m.gotoLogin(), nil
		}
		if m.screen == PINSetupScreen {
			// Cancel PIN setup, return to settings.
			m.screen = SettingsScreen
			m.authFields = nil
			m.authErr = ""
			return m, nil
		}
		return m, tea.Quit

	case "ctrl+r":
		if m.screen == LoginScreen {
			m.screen = RegisterScreen
			m.authFields = newRegisterFields(m.cfg)
			m.authCursor = 0
			m.authErr = ""
		}
		return m, nil

	case "ctrl+o":
		// Sign out from the unlock screen: clears the session + any PIN and
		// returns to the login screen (mirrors the settings 'o' shortcut).
		if m.screen == UnlockScreen {
			return m.logout()
		}
		return m, nil

	case "ctrl+p":
		// On the unlock screen, toggle between PIN and master-password entry
		// (only meaningful when a PIN is configured).
		if m.screen == UnlockScreen && m.vault != nil && m.vault.PINEnabled() {
			m.pinMode = !m.pinMode
			if m.pinMode {
				m.authFields = newPINUnlockFields()
			} else {
				m.authFields = newUnlockFields()
			}
			m.authCursor = 0
			m.authErr = ""
		}
		return m, nil

	case "tab", "down":
		if m.authCursor < len(m.authFields)-1 {
			m.authCursor++
		}
		return m, nil

	case "shift+tab", "up":
		if m.authCursor > 0 {
			m.authCursor--
		}
		return m, nil

	case "enter":
		if m.authCursor < len(m.authFields)-1 {
			m.authCursor++
			return m, nil
		}
		return m.submitAuth()

	case "backspace":
		f := &m.authFields[m.authCursor]
		if len(f.value) > 0 {
			f.value = f.value[:len(f.value)-1]
		}
		return m, nil

	case "ctrl+v":
		if clip, err := readClipboard(); err == nil && clip != "" {
			m.authFields[m.authCursor].value += strings.TrimRight(clip, "\n\r")
		}
		return m, nil

	default:
		if len(msg.String()) == 1 {
			m.authFields[m.authCursor].value += msg.String()
		}
		return m, nil
	}
}

// gotoLogin switches back to the login screen.
func (m Model) gotoLogin() Model {
	m.screen = LoginScreen
	m.authFields = newLoginFields(m.cfg)
	m.authCursor = 0
	m.authErr = ""
	m.authBusy = false
	return m
}

// submitAuth validates input and launches the relevant async command.
func (m Model) submitAuth() (tea.Model, tea.Cmd) {
	switch m.screen {
	case LoginScreen:
		m.authBusy = true
		m.authErr = ""
		return m, m.loginCmd(m.authValue("server"), m.authValue("email"), m.authValue("password"))
	case RegisterScreen:
		if m.authValue("password") != m.authValue("password2") {
			m.authErr = "passwords do not match"
			return m, nil
		}
		m.authBusy = true
		m.authErr = ""
		return m, m.registerCmd(m.authValue("server"), m.authValue("token"), m.authValue("name"),
			m.authValue("email"), m.authValue("password"))
	case UnlockScreen:
		m.authBusy = true
		m.authErr = ""
		if m.pinMode {
			return m, m.pinUnlockCmd(m.authValue("pin"))
		}
		return m, m.unlockCmd(m.authValue("password"))
	case PINSetupScreen:
		if m.authValue("pin") != m.authValue("pin2") {
			m.authErr = "PINs do not match"
			return m, nil
		}
		if len(m.authValue("pin")) < 4 {
			m.authErr = "PIN must be at least 4 digits"
			return m, nil
		}
		m.authBusy = true
		m.authErr = ""
		return m, m.enablePINCmd(m.authValue("password"), m.authValue("pin"), m.authValue("interval"))
	}
	return m, nil
}

// loginCmd authenticates, persists config, and unlocks the vault.
func (m Model) loginCmd(server, email, password string) tea.Cmd {
	cfgPath := m.cfgPath
	return func() tea.Msg {
		server = strings.TrimRight(strings.TrimSpace(server), "/")
		if server == "" || email == "" || password == "" {
			return AuthResultMsg{err: fmt.Errorf("server, email and password are required")}
		}
		client := apiclient.New(server)
		resp, err := client.Login(apiclient.LoginRequest{Email: email, Password: password})
		if err != nil {
			return AuthResultMsg{err: fmt.Errorf("login failed: %w", err)}
		}
		newCfg := &config.Config{
			ServerURL:       server,
			UserID:          resp.UserID,
			Email:           resp.Email,
			RefreshToken:    resp.RefreshToken,
			PubX25519:       resp.PubX25519,
			PubMLKEM768:     resp.PubMLKEM768,
			EncPrivX25519:   resp.EncPrivX25519,
			EncPrivMLKEM768: resp.EncPrivMLKEM768,
			KDFSalt:         resp.KDFSalt,
			KDFTime:         resp.KDFTime,
			KDFMemory:       resp.KDFMemory,
		}
		if err := newCfg.Save(cfgPath); err != nil {
			return AuthResultMsg{err: fmt.Errorf("save config: %w", err)}
		}
		v := vaultpkg.New(newCfg, cfgPath)
		if err := v.Authenticate(); err != nil {
			return AuthResultMsg{err: err}
		}
		if err := v.Unlock(password); err != nil {
			return AuthResultMsg{err: err}
		}
		return AuthResultMsg{vault: v, cfg: newCfg}
	}
}

// registerCmd generates keys, registers the account, and unlocks the vault.
func (m Model) registerCmd(server, token, name, email, password string) tea.Cmd {
	cfgPath := m.cfgPath
	return func() tea.Msg {
		server = strings.TrimRight(strings.TrimSpace(server), "/")
		if server == "" || name == "" || email == "" || password == "" {
			return AuthResultMsg{err: fmt.Errorf("server, name, email and password are required")}
		}
		privX25519, pubX25519, err := crypto.GenerateX25519()
		if err != nil {
			return AuthResultMsg{err: fmt.Errorf("generate x25519: %w", err)}
		}
		privMLKEM, pubMLKEM, err := crypto.GenerateMLKEM768()
		if err != nil {
			return AuthResultMsg{err: fmt.Errorf("generate mlkem768: %w", err)}
		}
		kdfParams, err := crypto.NewKDFParams()
		if err != nil {
			return AuthResultMsg{err: err}
		}
		masterKey := crypto.DeriveKey(password, kdfParams)
		encPrivX25519, err := crypto.Encrypt(masterKey, privX25519)
		if err != nil {
			return AuthResultMsg{err: err}
		}
		encPrivMLKEM, err := crypto.Encrypt(masterKey, privMLKEM)
		if err != nil {
			return AuthResultMsg{err: err}
		}
		client := apiclient.New(server)
		resp, err := client.Register(apiclient.RegisterRequest{
			Email:           email,
			Name:            name,
			Password:        password,
			InvitationToken: token,
			PubX25519:       crypto.B64Enc(pubX25519),
			PubMLKEM768:     crypto.B64Enc(pubMLKEM),
			EncPrivX25519:   crypto.B64Enc(encPrivX25519),
			EncPrivMLKEM768: crypto.B64Enc(encPrivMLKEM),
			KDFSalt:         crypto.B64Enc(kdfParams.Salt),
		})
		if err != nil {
			return AuthResultMsg{err: fmt.Errorf("registration failed: %w", err)}
		}
		newCfg := &config.Config{
			ServerURL:       server,
			UserID:          resp.UserID,
			Email:           resp.Email,
			RefreshToken:    resp.RefreshToken,
			PubX25519:       crypto.B64Enc(pubX25519),
			PubMLKEM768:     crypto.B64Enc(pubMLKEM),
			EncPrivX25519:   crypto.B64Enc(encPrivX25519),
			EncPrivMLKEM768: crypto.B64Enc(encPrivMLKEM),
			KDFSalt:         crypto.B64Enc(kdfParams.Salt),
			KDFTime:         int(kdfParams.Time),
			KDFMemory:       int(kdfParams.Memory),
		}
		if err := newCfg.Save(cfgPath); err != nil {
			return AuthResultMsg{err: fmt.Errorf("save config: %w", err)}
		}
		v := vaultpkg.New(newCfg, cfgPath)
		if err := v.Authenticate(); err != nil {
			return AuthResultMsg{err: err}
		}
		if err := v.Unlock(password); err != nil {
			return AuthResultMsg{err: err}
		}
		return AuthResultMsg{vault: v, cfg: newCfg}
	}
}

// unlockCmd refreshes the token and decrypts the private keys in place.
func (m Model) unlockCmd(password string) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if v == nil {
			return AuthResultMsg{err: fmt.Errorf("no session — please log in")}
		}
		if err := v.Authenticate(); err != nil {
			return AuthResultMsg{err: err}
		}
		if err := v.Unlock(password); err != nil {
			return AuthResultMsg{err: err}
		}
		return AuthResultMsg{vault: v}
	}
}

// pinUnlockCmd refreshes the token and decrypts the private keys using the PIN.
func (m Model) pinUnlockCmd(pin string) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if v == nil {
			return AuthResultMsg{err: fmt.Errorf("no session — please log in")}
		}
		if pin == "" {
			return AuthResultMsg{err: fmt.Errorf("PIN is required")}
		}
		if err := v.UnlockWithPIN(pin); err != nil {
			return AuthResultMsg{err: err}
		}
		// Keys are in memory; now obtain a fresh access token for API calls.
		if err := v.Authenticate(); err != nil {
			return AuthResultMsg{err: err}
		}
		return AuthResultMsg{vault: v}
	}
}

// enablePINCmd wraps the master key under the new PIN and persists the config.
func (m Model) enablePINCmd(masterPassword, pin, intervalStr string) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if v == nil {
			return PINConfigMsg{err: fmt.Errorf("no session — please log in")}
		}
		interval := vaultpkg.DefaultPINIntervalDays
		if intervalStr != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(intervalStr)); err == nil {
				interval = vaultpkg.ClampPINIntervalDays(n)
			}
		}
		if err := v.EnablePIN(masterPassword, pin, interval, vaultpkg.DefaultPINMaxTries); err != nil {
			return PINConfigMsg{err: err}
		}
		return PINConfigMsg{status: fmt.Sprintf("PIN enabled (master password required every %d days)", interval)}
	}
}

// disablePINCmd removes the PIN quick-unlock configuration.
func (m Model) disablePINCmd() tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if v == nil {
			return PINConfigMsg{err: fmt.Errorf("no session")}
		}
		if err := v.DisablePIN(); err != nil {
			return PINConfigMsg{err: err}
		}
		return PINConfigMsg{status: "PIN disabled"}
	}
}

// renderAuthScreen renders the login / register / unlock screen.
func (m Model) renderAuthScreen() string {
	var titleText, help, banner string
	switch m.screen {
	case RegisterScreen:
		titleText = "passbubble:~$ register"
		help = "Tab/↑↓: fields  Enter: next/submit  Ctrl+V: paste  Esc: back to login"
	case UnlockScreen:
		titleText = "passbubble:~$ unlock vault"
		if m.pinMode {
			help = "Enter: unlock with PIN  Ctrl+P: use master password  Ctrl+O: log out  Esc/Ctrl+C: quit"
		} else {
			help = "Enter: unlock  Ctrl+V: paste  Ctrl+O: log out  Esc/Ctrl+C: quit"
			if m.vault != nil && m.vault.PINEnabled() {
				help = "Enter: unlock  Ctrl+P: use PIN  Ctrl+O: log out  Ctrl+V: paste  Esc/Ctrl+C: quit"
			}
		}
	case PINSetupScreen:
		titleText = "passbubble:~$ setup pin"
		help = "Tab/↑↓: fields  Enter: next/submit  Esc: cancel"
		banner = "⚠ A PIN is less secure than your master password: it is stored on this device\n" +
			"  in a plaintext config file and a short PIN can be brute-forced by anyone with\n" +
			"  read access to it. Use a strong PIN and keep this machine trusted."
	default:
		titleText = "passbubble:~$ login"
		help = "Tab/↑↓: fields  Enter: next/submit  Ctrl+R: register  Esc: quit"
	}

	var b strings.Builder
	b.WriteString(m.titleStyle.Render(titleText))
	b.WriteString("\n\n")
	if banner != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(colAmber).Render(banner))
		b.WriteString("\n\n")
	}

	for i, f := range m.authFields {
		labelStyle := lipgloss.NewStyle().Foreground(colGreen).Bold(true)
		if i == m.authCursor {
			labelStyle = labelStyle.Background(colSurface)
		}
		b.WriteString(labelStyle.Render(f.label))
		b.WriteString("\n")

		value := f.value
		if f.hidden {
			value = strings.Repeat("•", len(f.value))
		}
		if i == m.authCursor {
			value += "█"
		}
		fieldWidth := m.width - 12
		if fieldWidth < 30 {
			fieldWidth = 30
		}
		inputStyle := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colBorder).
			Padding(0, 1).
			Width(fieldWidth)
		if i == m.authCursor {
			inputStyle = inputStyle.BorderForeground(colGreen)
		}
		b.WriteString(inputStyle.Render(value))
		b.WriteString("\n\n")
	}

	if m.authBusy {
		b.WriteString(lipgloss.NewStyle().Foreground(colGreen).Render("Working…"))
		b.WriteString("\n\n")
	}
	if m.authErr != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(colRed).Bold(true).Render("Error: " + m.authErr))
		b.WriteString("\n\n")
	}
	b.WriteString(m.helpStyle.Render(help))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colBorder).
		Padding(1, 2).
		Render(b.String())
}

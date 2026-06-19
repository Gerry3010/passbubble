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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mdp/qrterminal/v3"
)

// ShareLinkCreatedMsg carries the result of an async share-link creation.
type ShareLinkCreatedMsg struct {
	url string
	err error
}

// shareExpiryOptions are the expiry choices offered in the picker. A zero
// duration means "never expires".
var shareExpiryOptions = []struct {
	key   string
	label string
	d     time.Duration
}{
	{"1", "1 day", 24 * time.Hour},
	{"2", "7 days", 7 * 24 * time.Hour},
	{"3", "30 days", 30 * 24 * time.Hour},
	{"4", "90 days", 90 * 24 * time.Hour},
	{"5", "1 year", 365 * 24 * time.Hour},
	{"6", "Never", 0},
}

// handleShareMenu handles key input while the expiry picker is open.
func (m Model) handleShareMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.showShareMenu = false
		m.status = "Share cancelled"
		m.statusType = "info"
		return m, nil
	}
	for _, opt := range shareExpiryOptions {
		if msg.String() == opt.key {
			m.showShareMenu = false
			m.status = "Creating share link…"
			m.statusType = "info"
			return m, m.createShareLinkCmd(m.shareEntryID, m.shareIsFolder, m.shareEntryName, opt.d)
		}
	}
	return m, nil
}

// createShareLinkCmd builds an entry or folder share link off the UI thread.
func (m Model) createShareLinkCmd(id string, isFolder bool, name string, validity time.Duration) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if v == nil {
			return ShareLinkCreatedMsg{err: fmt.Errorf("vault is locked")}
		}
		var (
			url string
			err error
		)
		if isFolder {
			url, err = v.CreateFolderShareLink(id, name, validity)
		} else {
			url, err = v.CreateEntryShareLink(id, validity)
		}
		return ShareLinkCreatedMsg{url: url, err: err}
	}
}

// renderQR renders a scannable QR of s as half-block unicode (compact, black on
// white so it scans regardless of terminal theme).
func renderQR(s string) string {
	var b strings.Builder
	qrterminal.GenerateHalfBlock(s, qrterminal.L, &b)
	return b.String()
}

// renderShareMenu renders the expiry picker overlay.
func (m Model) renderShareMenu() string {
	kind := "Entry"
	if m.shareIsFolder {
		kind = "Folder"
	}
	var b strings.Builder
	b.WriteString(m.titleStyle.Render("🔗 Share link"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "%s: %s\n\n", kind, m.shareEntryName)
	b.WriteString("Valid for:\n")
	for _, opt := range shareExpiryOptions {
		fmt.Fprintf(&b, "  [%s] %s\n", opt.key, opt.label)
	}
	b.WriteString("\n")
	b.WriteString(m.helpStyle.Render("1-6: pick expiry   Esc: cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Render(b.String())
	return lipgloss.JoinVertical(lipgloss.Left, box)
}

// renderShareQR renders the created link as a QR code plus the raw URL.
func (m Model) renderShareQR() string {
	var b strings.Builder
	b.WriteString(m.titleStyle.Render("🔗 Share link — scan or copy"))
	b.WriteString("\n\n")
	b.WriteString(m.shareQR)
	b.WriteString("\n")
	b.WriteString(m.helpStyle.Render("The decryption key is in the link (after #) and never reaches the server."))
	b.WriteString("\n\n")
	b.WriteString(m.secretStyle.Render(m.shareURL))
	b.WriteString("\n\n")
	b.WriteString(m.helpStyle.Render("Copied to clipboard · press any key to close"))
	return b.String()
}

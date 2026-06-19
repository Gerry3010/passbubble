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

	tea "github.com/charmbracelet/bubbletea"
)

// handleSearchInput handles key input while the search field is active. The
// list filters live as the query changes (see Model.currentItems).
func (m Model) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Cancel search: close the input and clear the filter.
		m.showSearch = false
		m.filter = ""
		m.cursor = 0
		m.listOffset = 0
		return m, nil

	case "enter":
		// Apply: close the input but keep the filter so results can be browsed.
		m.showSearch = false
		m.cursor = 0
		m.listOffset = 0
		return m, nil

	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.cursor = 0
			m.listOffset = 0
		}
		return m, nil

	case "ctrl+u":
		m.filter = ""
		m.cursor = 0
		m.listOffset = 0
		return m, nil

	case "ctrl+v":
		if clip, err := readClipboard(); err == nil && clip != "" {
			m.filter += strings.TrimRight(clip, "\n\r")
		}
		return m, nil

	default:
		if len(msg.String()) == 1 {
			m.filter += msg.String()
			m.cursor = 0
			m.listOffset = 0
		}
		return m, nil
	}
}

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

import "github.com/charmbracelet/lipgloss"

// Brand color tokens for the "phosphor terminal" theme — bright green on
// near-black, mirroring the browser extension. Lip Gloss accepts truecolor
// hex and auto-degrades to ANSI on limited terminals.
// See docs/design-guidelines.md for the shared design language.
var (
	colBg      = lipgloss.Color("#0a0a0a")
	colSurface = lipgloss.Color("#0e140f")
	colGreen   = lipgloss.Color("#00ff41")
	colMuted   = lipgloss.Color("#5f8c6a")
	colBorder  = lipgloss.Color("#1d3a24")
	colRed     = lipgloss.Color("#ff5f56")
	colAmber   = lipgloss.Color("#ffb000")
)

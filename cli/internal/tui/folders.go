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

	vaultpkg "github.com/Gerry3010/passbubble/cli/internal/vault"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// folderContentCount returns how many direct subfolders + entries a folder holds.
// Used to refuse deletion of non-empty folders (the backend has no ON DELETE
// CASCADE on entries.folder_id / folders.parent_id, so a delete would 500).
func (m Model) folderContentCount(f *vaultpkg.Folder) int {
	n := len(f.Children)
	for i := range m.allEntries {
		if e := m.allEntries[i].FolderID; e != nil && *e == f.ID {
			n++
		}
	}
	return n
}

// createFolderCmd creates a folder and refreshes the list.
func (m Model) createFolderCmd(name string, parentID *string) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if _, err := v.CreateFolder(strings.TrimSpace(name), parentID); err != nil {
			return ActionResultMsg{Success: false, Message: fmt.Sprintf("Create folder failed: %v", err), Action: "folder"}
		}
		return ActionResultMsg{Success: true, Message: fmt.Sprintf("Folder %q created", name), Action: "folder"}
	}
}

// renameFolderCmd renames a folder and refreshes the list.
func (m Model) renameFolderCmd(id, name string, parentID *string) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if err := v.RenameFolder(id, strings.TrimSpace(name), parentID); err != nil {
			return ActionResultMsg{Success: false, Message: fmt.Sprintf("Rename failed: %v", err), Action: "folder"}
		}
		return ActionResultMsg{Success: true, Message: fmt.Sprintf("Folder renamed to %q", name), Action: "folder"}
	}
}

// deleteFolderCmd deletes a folder and refreshes the list.
func (m Model) deleteFolderCmd(id string) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if err := v.DeleteFolder(id); err != nil {
			return ActionResultMsg{Success: false, Message: fmt.Sprintf("Delete folder failed: %v", err), Action: "folder"}
		}
		return ActionResultMsg{Success: true, Message: "Folder deleted", Action: "folder"}
	}
}

// --- Move-entry overlay ---

// moveTarget is one selectable destination folder in the move overlay.
type moveTarget struct {
	id    *string // nil = root
	label string
}

// openMoveMenu builds the flattened destination list and opens the overlay.
func (m *Model) openMoveMenu(entry *Entry) {
	m.moveEntryID = entry.ID
	m.moveCursor = 0
	m.moveTargets = []moveTarget{{id: nil, label: "(root)"}}
	for _, root := range m.folders {
		flattenFolderTargets(root, "", &m.moveTargets)
	}
	m.showMoveMenu = true
}

// flattenFolderTargets appends a folder and its descendants as indented targets.
func flattenFolderTargets(f *vaultpkg.Folder, prefix string, out *[]moveTarget) {
	id := f.ID
	*out = append(*out, moveTarget{id: &id, label: prefix + f.Name})
	for _, c := range f.Children {
		flattenFolderTargets(c, prefix+"  ", out)
	}
}

// handleMoveMenu handles key input while the move overlay is open.
func (m Model) handleMoveMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.showMoveMenu = false
		return m, nil
	case "up", "k":
		if m.moveCursor > 0 {
			m.moveCursor--
		}
		return m, nil
	case "down", "j":
		if m.moveCursor < len(m.moveTargets)-1 {
			m.moveCursor++
		}
		return m, nil
	case "enter":
		m.showMoveMenu = false
		if m.moveCursor < 0 || m.moveCursor >= len(m.moveTargets) {
			return m, nil
		}
		target := m.moveTargets[m.moveCursor]
		return m, m.moveEntryCmd(m.moveEntryID, target.id)
	}
	return m, nil
}

// moveEntryCmd moves an entry to the given folder (nil = root) and refreshes.
func (m Model) moveEntryCmd(entryID string, folderID *string) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if err := v.MoveEntry(entryID, folderID); err != nil {
			return ActionResultMsg{Success: false, Message: fmt.Sprintf("Move failed: %v", err), Action: "folder"}
		}
		return ActionResultMsg{Success: true, Message: "Entry moved", Action: "folder"}
	}
}

// renderMoveMenu renders the move-entry overlay.
func (m Model) renderMoveMenu() string {
	var b strings.Builder
	b.WriteString(m.titleStyle.Render("📂 Move to folder"))
	b.WriteString("\n\n")
	for i, t := range m.moveTargets {
		cursor := "  "
		line := "📁 " + t.label
		if t.id == nil {
			line = "🏠 " + t.label
		}
		if i == m.moveCursor {
			cursor = "> "
			line = m.selectedStyle.Render(line)
		}
		b.WriteString(cursor + line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(m.helpStyle.Render("↑/↓: navigate  Enter: move here  Esc: cancel"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Render(b.String())
}

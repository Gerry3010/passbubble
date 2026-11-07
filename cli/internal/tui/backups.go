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
	"strings"

	"github.com/Gerry3010/passbubble/cli/pkg/backup"
	tea "github.com/charmbracelet/bubbletea"
)

// The backup screen does what `pwmgr backup`, `restore` and `list-backups` do, in
// the same directory and through the same code, so the two see each other's files.

var errBackupLocked = errors.New("vault is locked")

// createBackupCmd writes a backup of the whole vault. An incomplete one comes back
// with both a file name and an error.
func (m Model) createBackupCmd(fields map[string]string) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if v == nil || !v.IsUnlocked() {
			return BackupCreatedMsg{Error: errBackupLocked}
		}
		dir, err := backup.DefaultDir()
		if err != nil {
			return BackupCreatedMsg{Error: err}
		}
		path, err := backup.NewPath(dir, strings.TrimSpace(fields["backup_name"]))
		if err != nil {
			return BackupCreatedMsg{Error: err}
		}
		cfg := v.Config()
		result, err := backup.Create(v, path, backup.Options{Server: cfg.ServerURL, Email: cfg.Email})
		return BackupCreatedMsg{Filename: result.Path, Error: err}
	}
}

// restoreBackupCmd adds the entries of a backup to the vault.
func (m Model) restoreBackupCmd(info backup.Info) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if v == nil || !v.IsUnlocked() {
			return BackupRestoredMsg{Error: errBackupLocked}
		}
		file, err := backup.Read(info.Path)
		if err != nil {
			return BackupRestoredMsg{Error: err}
		}
		result := backup.Restore(v, file, nil)
		if len(result.Failed) > 0 {
			return BackupRestoredMsg{
				Restored: result.Restored,
				Error: fmt.Errorf("%d of %d entries restored, these were not: %s",
					result.Restored, len(file.Entries), strings.Join(result.Failed, "; ")),
			}
		}
		return BackupRestoredMsg{
			Restored: result.Restored,
			Message:  fmt.Sprintf("Restored %d entries from %s", result.Restored, info.Name),
		}
	}
}

// loadBackups lists the backups in the default directory.
func (m Model) loadBackups() tea.Cmd {
	return func() tea.Msg {
		dir, err := backup.DefaultDir()
		if err != nil {
			return LoadBackupsMsg{Err: err}
		}
		backups, err := backup.List(dir)
		if err != nil {
			return LoadBackupsMsg{Err: err}
		}
		return LoadBackupsMsg{Backups: backups}
	}
}

// reloadEntriesKeepingStatus refreshes the list and leaves the status line alone.
func (m Model) reloadEntriesKeepingStatus() tea.Cmd {
	load := m.loadEntries()
	return func() tea.Msg {
		msg := load()
		if loaded, ok := msg.(LoadEntriesMsg); ok {
			loaded.KeepStatus = true
			return loaded
		}
		return msg
	}
}

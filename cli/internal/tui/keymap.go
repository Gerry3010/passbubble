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

// Rebindable main-screen actions. Navigation keys (arrows, j/k, enter, esc)
// are intentionally fixed and not part of the keymap.
const (
	actAddPassword = "add_password"
	actAddTOTP     = "add_totp"
	actNewFolder   = "new_folder"
	actEdit        = "edit"
	actDelete      = "delete"
	actMove        = "move"
	actCopyPass    = "copy_password"
	actCopyUser    = "copy_username"
	actShareLink   = "share_link"
	actSortCycle   = "sort_cycle"
	actSortDir     = "sort_dir"
	actFolderFirst = "folder_first"
	actSortMenu    = "sort_menu"
	actSearch      = "search"
	actGenerate    = "generate"
	actBackup      = "backup"
	actBackups     = "backups"
	actSettings    = "settings"
	actRefresh     = "refresh"
	actHelp        = "help"
	actQuit        = "quit"
)

// actionOrder is the stable display order used by the settings/help screens.
var actionOrder = []string{
	actAddPassword, actAddTOTP, actNewFolder, actEdit, actDelete, actMove,
	actCopyPass, actCopyUser, actShareLink, actSearch, actSortCycle, actSortDir, actFolderFirst,
	actSortMenu, actGenerate, actBackup, actBackups, actSettings, actRefresh, actHelp, actQuit,
}

// actionLabels are human-readable descriptions for the settings/help screens.
var actionLabels = map[string]string{
	actAddPassword: "Add password",
	actAddTOTP:     "Add TOTP",
	actNewFolder:   "New folder",
	actEdit:        "Edit / rename",
	actDelete:      "Delete",
	actMove:        "Move entry",
	actCopyPass:    "Copy password",
	actCopyUser:    "Copy username",
	actShareLink:   "Create share link (QR)",
	actSearch:      "Search / filter",
	actSortCycle:   "Cycle sort field",
	actSortDir:     "Toggle sort direction",
	actFolderFirst: "Toggle folders-first",
	actSortMenu:    "Open sort menu",
	actGenerate:    "Password generator",
	actBackup:      "Create backup",
	actBackups:     "Manage backups",
	actSettings:    "Settings",
	actRefresh:     "Refresh",
	actHelp:        "Help overlay",
	actQuit:        "Quit",
}

// defaultKeymap maps each action to its default key.
func defaultKeymap() map[string]string {
	return map[string]string{
		actAddPassword: "p",
		actAddTOTP:     "t",
		actNewFolder:   "n",
		actEdit:        "e",
		actDelete:      "d",
		actMove:        "m",
		actCopyPass:    "y",
		actCopyUser:    "u",
		actShareLink:   "L",
		actSearch:      "/",
		actSortCycle:   "s",
		actSortDir:     "S",
		actFolderFirst: "f",
		actSortMenu:    "o",
		actGenerate:    "g",
		actBackup:      "c",
		actBackups:     "b",
		actSettings:    ".",
		actRefresh:     "r",
		actHelp:        "?",
		actQuit:        "q",
	}
}

// buildKeymap overlays user overrides from config onto the defaults.
// A binding whose value is "" is treated as unbound (the key does nothing).
func buildKeymap(overrides map[string]string) map[string]string {
	km := defaultKeymap()
	for action, key := range overrides {
		if _, ok := km[action]; ok {
			km[action] = key // "" = unbound
		}
	}
	return km
}

// actionForKey returns the action bound to a key (reverse lookup), if any.
func (m Model) actionForKey(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	for action, k := range m.keymap {
		if k == key {
			return action, true
		}
	}
	return "", false
}

// keyFor returns the key bound to an action ("" when unbound).
func (m Model) keyFor(action string) string {
	return m.keymap[action]
}

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

	vaultpkg "github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/Gerry3010/passbubble/cli/pkg/totp"
	tea "github.com/charmbracelet/bubbletea"
)

// An edit works on the entry ID, not through the keyring shim: GetEntry decrypts
// everything the entry holds (also keys the CLI does not model), only the fields
// of the form change, and UpdateEntry writes it back under the same data key.
// URL, type, folder and match patterns stay as they are.

// EditFormLoadedMsg carries the decrypted entry an edit form starts from. The
// list only knows metadata; the username and TOTP settings are encrypted.
type EditFormLoadedMsg struct {
	Entry Entry
	Data  *vaultpkg.EntryData
	Err   error
}

// EntryEditedMsg reports a finished edit.
type EntryEditedMsg struct {
	Entry    Entry
	Password string // the entry's password after the edit, for the detail screen
	Err      error
}

func fetchEntry(v *vaultpkg.Vault, id string) (*vaultpkg.Entry, error) {
	if v == nil {
		return nil, errors.New("vault is locked")
	}
	full, err := v.GetEntry(id)
	if err != nil {
		return nil, err
	}
	if full.Data == nil {
		return nil, errors.New("entry has no data")
	}
	return full, nil
}

// loadEditFormCmd decrypts an entry so its edit form can show the stored values.
func (m Model) loadEditFormCmd(entry Entry) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		full, err := fetchEntry(v, entry.ID)
		if err != nil {
			return EditFormLoadedMsg{Err: err}
		}
		entry.Service = full.Name
		entry.URL = full.URL
		entry.Type = full.Type
		return EditFormLoadedMsg{Entry: entry, Data: full.Data}
	}
}

// openEditForm shows the form once its entry is decrypted, unless another form
// was opened in the meantime.
func (m Model) openEditForm(msg EditFormLoadedMsg) Model {
	if msg.Err != nil {
		m.status = fmt.Sprintf("Cannot edit entry: %v", msg.Err)
		m.statusType = "error"
		return m
	}
	if m.showingForm {
		return m
	}
	m.showingForm = true
	m.form = CreateEditEntryForm(msg.Entry, msg.Data)
	m.form.width = m.width
	m.status = "Editing entry"
	m.statusType = "info"
	return m
}

// editEntryCmd applies a submitted edit form to the current state of the entry.
func (m Model) editEntryCmd(entry *Entry, fields map[string]string) tea.Cmd {
	v := m.vault
	return func() tea.Msg {
		if entry == nil {
			return EntryEditedMsg{Err: errors.New("no entry to edit")}
		}
		name := strings.TrimSpace(fields["service"])
		if name == "" {
			return EntryEditedMsg{Err: errors.New("service name is required")}
		}
		full, err := fetchEntry(v, entry.ID)
		if err != nil {
			return EntryEditedMsg{Err: err}
		}
		if err := applyEntryEdit(full.Data, full.Type, fields); err != nil {
			return EntryEditedMsg{Err: err}
		}
		if err := v.UpdateEntry(full.ID, name, full.URL, full.Data, nil); err != nil {
			return EntryEditedMsg{Err: err}
		}
		edited := *entry
		edited.Service = name
		edited.Username = full.Data.Username
		edited.URL = full.URL
		edited.Type = full.Type
		edited.FolderID = full.FolderID
		return EntryEditedMsg{Entry: edited, Password: full.Data.Password}
	}
}

// applyEntryEdit changes the fields the edit form covers and leaves the rest of
// data alone. Username and issuer start out filled in, so empty removes them;
// the secrets and TOTP settings keep their value when left empty.
func applyEntryEdit(data *vaultpkg.EntryData, entryType string, fields map[string]string) error {
	setTrimmed(&data.Username, fields["username"])
	if entryType != "totp" {
		if pw := strings.TrimSpace(fields["password"]); pw != "" {
			data.Password = pw
		}
		return nil
	}

	setTrimmed(&data.Issuer, fields["issuer"])
	if secret := strings.TrimSpace(fields["secret"]); secret != "" {
		if !totp.IsValidSecret(secret) {
			return errors.New("invalid TOTP secret format")
		}
		data.TOTPSecret = secret
	}
	if s := strings.TrimSpace(fields["algorithm"]); s != "" {
		algorithm := strings.ToUpper(s)
		if algorithm != "SHA1" && algorithm != "SHA256" && algorithm != "SHA512" {
			return errors.New("algorithm must be SHA1, SHA256, or SHA512")
		}
		data.Algorithm = algorithm
	}
	if s := strings.TrimSpace(fields["length"]); s != "" {
		digits, err := strconv.Atoi(s)
		if err != nil || (digits != 6 && digits != 8) {
			return errors.New("length must be 6 or 8")
		}
		data.Digits = digits
	}
	if s := strings.TrimSpace(fields["period"]); s != "" {
		period, err := strconv.Atoi(s)
		if err != nil || period < 10 || period > 300 {
			return errors.New("period must be between 10 and 300 seconds")
		}
		data.Period = period
	}
	return nil
}

// setTrimmed stores the trimmed input unless that is what dst already says, so
// an untouched field stays byte for byte as it was.
func setTrimmed(dst *string, input string) {
	if v := strings.TrimSpace(input); v != strings.TrimSpace(*dst) {
		*dst = v
	}
}

// finishEdit shows the outcome of an edit and refreshes the list. An open detail
// screen follows the entry, because it looks the secret up by name.
func (m Model) finishEdit(msg EntryEditedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.status = fmt.Sprintf("Failed to update entry: %v", msg.Err)
		m.statusType = "error"
		return m, nil
	}
	if m.screen == DetailScreen && m.detailEntry.ID == msg.Entry.ID {
		m.detailEntry = msg.Entry
		m.password = msg.Password
		m.totpCode = ""
		m.totpRemaining = 0
	}
	m.status = fmt.Sprintf("Entry %s", msg.Entry.Service)
	if msg.Entry.Username != "" {
		m.status += fmt.Sprintf(" (%s)", msg.Entry.Username)
	}
	m.status += " updated successfully"
	m.statusType = "success"
	return m, m.loadEntries()
}

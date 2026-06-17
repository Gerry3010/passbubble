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

package vault

import "strings"

// EntryRecord is a decrypted entry ready for export or duplicate detection.
type EntryRecord struct {
	ID       string
	Name     string
	URL      string
	Type     string
	FolderID *string
	Data     EntryData
}

// ListAllDecrypted fetches and decrypts all entries.
func (v *Vault) ListAllDecrypted() ([]EntryRecord, error) {
	entries, err := v.ListEntries()
	if err != nil {
		return nil, err
	}
	out := make([]EntryRecord, 0, len(entries))
	for _, e := range entries {
		if e.Data == nil {
			continue
		}
		out = append(out, EntryRecord{
			ID:       e.ID,
			Name:     e.Name,
			URL:      e.URL,
			Type:     e.Type,
			FolderID: e.FolderID,
			Data:     *e.Data,
		})
	}
	return out, nil
}

// IsDuplicate returns the ID of an existing entry if one matches name+username.
func IsDuplicate(name, username string, existing []EntryRecord) (string, bool) {
	for _, e := range existing {
		if strings.EqualFold(e.Name, name) && strings.EqualFold(e.Data.Username, username) {
			return e.ID, true
		}
	}
	return "", false
}

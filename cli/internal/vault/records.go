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

import (
	"fmt"
	"strings"
)

// EntryRecord is a decrypted entry ready for export or duplicate detection.
type EntryRecord struct {
	ID       string
	Name     string
	URL      string
	Type     string
	FolderID *string
	Data     EntryData
}

// ListAllDecrypted fetches and decrypts all entries. ListEntries returns
// metadata only (Data==nil), so each entry is fetched + decrypted individually.
func (v *Vault) ListAllDecrypted() ([]EntryRecord, error) {
	entries, err := v.ListEntries()
	if err != nil {
		return nil, err
	}
	out := make([]EntryRecord, 0, len(entries))
	for _, e := range entries {
		full, err := v.GetEntry(e.ID) // fetches entry_key + encrypted_data, then decrypts
		if err != nil {
			return nil, fmt.Errorf("decrypt entry %q: %w", e.Name, err)
		}
		if full.Data == nil {
			continue
		}
		out = append(out, EntryRecord{
			ID:       full.ID,
			Name:     full.Name,
			URL:      full.URL,
			Type:     full.Type,
			FolderID: full.FolderID,
			Data:     *full.Data,
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

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

// FindDuplicates returns the IDs of all existing entries a new record of the given
// type would duplicate: same name and username, in the same type family. An update
// cannot change an entry's type, so anything else would store the record's fields
// on an entry typed for something else.
func FindDuplicates(name, username, entryType string, existing []EntryRecord) []string {
	var ids []string
	for _, e := range existing {
		if strings.EqualFold(e.Name, name) && strings.EqualFold(e.Data.Username, username) && sameTypeFamily(e.Type, entryType) {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

// sameTypeFamily reports whether two entry types can stand in for each other:
// password, totp and no type at all are one family, the logins, and every other
// type is its own. Keep in step with the app (flutter_app/lib/core/jobs/job_runner.dart).
func sameTypeFamily(a, b string) bool {
	isLogin := func(t string) bool { return t == "" || t == "password" || t == "totp" }
	return a == b || (isLogin(a) && isLogin(b))
}

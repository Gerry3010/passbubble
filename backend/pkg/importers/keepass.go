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

package importers

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tobischo/gokeepasslib/v3"
	"github.com/tobischo/gokeepasslib/v3/wrappers"
)

// keepassTime formats a KeePass time wrapper as RFC3339 UTC ("" if unset).
func keepassTime(tw *wrappers.TimeWrapper) string {
	if tw == nil || tw.Time.IsZero() {
		return ""
	}
	return tw.Time.UTC().Format(time.RFC3339)
}

// ParseKeePass parses a KeePass .kdbx file (v3/v4).
// password is the database master password; pass "" for keyfile-only (not yet supported).
func ParseKeePass(r io.Reader, password string) (*ImportResult, error) {
	db := gokeepasslib.NewDatabase()
	db.Credentials = gokeepasslib.NewPasswordCredentials(password)

	if err := gokeepasslib.NewDecoder(r).Decode(db); err != nil {
		return nil, fmt.Errorf("keepass: decode: %w", err)
	}
	if err := db.UnlockProtectedEntries(); err != nil {
		return nil, fmt.Errorf("keepass: unlock: %w", err)
	}

	result := &ImportResult{}
	// The outermost group is the KeePass "Root" container; its own name is not
	// treated as a folder. Sub-groups become the folder hierarchy.
	for _, group := range db.Content.Root.Groups {
		walkKeePassGroup(group, nil, result)
	}
	return result, nil
}

func walkKeePassGroup(group gokeepasslib.Group, parentPath []string, result *ImportResult) {
	for _, entry := range group.Entries {
		rec := convertKeePassEntry(entry)
		if rec == nil {
			result.Skipped++
			continue
		}
		rec.FolderPath = parentPath
		result.Records = append(result.Records, *rec)
	}
	for _, sub := range group.Groups {
		childPath := parentPath
		if sub.Name != "" {
			childPath = append(append([]string{}, parentPath...), sub.Name)
		}
		walkKeePassGroup(sub, childPath, result)
	}
}

func convertKeePassEntry(e gokeepasslib.Entry) *EntryRecord {
	get := func(key string) string {
		return e.GetContent(key)
	}

	title := get("Title")
	if title == "" {
		return nil
	}

	rec := &EntryRecord{
		Type:      "password",
		Name:      title,
		URL:       get("URL"),
		Username:  get("UserName"),
		Password:  get("Password"),
		Notes:     get("Notes"),
		CreatedAt: keepassTime(e.Times.CreationTime),
		UpdatedAt: keepassTime(e.Times.LastModificationTime),
	}

	// Detect TOTP via common KeePass TOTP field names
	for _, key := range []string{"otp", "TOTP Seed", "TimeOtp-Secret-Base32", "HmacOtp-Secret-Base32"} {
		if v := get(key); v != "" {
			rec.TOTPSecret = v
			rec.Type = "totp"
			break
		}
	}

	// Custom fields (skip the standard ones)
	standard := map[string]bool{"Title": true, "URL": true, "UserName": true, "Password": true, "Notes": true}
	for _, val := range e.Values {
		if !standard[val.Key] && !strings.HasPrefix(val.Key, "_") {
			rec.CustomFields = append(rec.CustomFields, CustomField{
				Label: val.Key,
				Value: val.Value.Content,
			})
		}
	}

	return rec
}

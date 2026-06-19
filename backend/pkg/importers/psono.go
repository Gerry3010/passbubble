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
	"encoding/json"
	"fmt"
)

// Psono JSON export structures.
type psonoExport struct {
	Folders []psonoFolder          `json:"folders"`
	Items   []map[string]any `json:"items"`
}

type psonoFolder struct {
	Name    string           `json:"name"`
	Items   []map[string]any `json:"items"`
	Folders []psonoFolder    `json:"folders"`
}

// ParsePsono parses a Psono JSON export (unencrypted).
func ParsePsono(data []byte) (*ImportResult, error) {
	var export psonoExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("psono: invalid JSON: %w", err)
	}

	result := &ImportResult{}

	// Root-level items
	for _, item := range export.Items {
		convertPsonoItem(item, nil, result)
	}

	// Recursively walk folders, accumulating the folder-name path.
	for _, folder := range export.Folders {
		walkPsonoFolder(folder, nil, result)
	}

	return result, nil
}

func walkPsonoFolder(folder psonoFolder, parentPath []string, result *ImportResult) {
	path := parentPath
	if folder.Name != "" {
		// Copy to avoid aliasing the parent's slice across siblings.
		path = append(append([]string{}, parentPath...), folder.Name)
	}
	for _, item := range folder.Items {
		convertPsonoItem(item, path, result)
	}
	for _, sub := range folder.Folders {
		walkPsonoFolder(sub, path, result)
	}
}

func convertPsonoItem(item map[string]any, folderPath []string, result *ImportResult) {
	str := func(key string) string {
		v, _ := item[key].(string)
		return v
	}

	entryType := str("type")
	name := str("name")

	var rec EntryRecord

	switch entryType {
	case "website_password":
		title := str("website_password_title")
		if title == "" {
			title = name
		}
		totp := str("website_password_totp_code")
		t := "password"
		if totp != "" {
			t = "totp"
		}
		rec = EntryRecord{
			Name:       title,
			Type:       t,
			URL:        str("website_password_url"),
			Username:   str("website_password_username"),
			Password:   str("website_password_password"),
			TOTPSecret: totp,
			Notes:      str("website_password_notes"),
		}

	case "application_password":
		title := str("application_password_title")
		if title == "" {
			title = name
		}
		rec = EntryRecord{
			Name:     title,
			Type:     "password",
			Username: str("application_password_username"),
			Password: str("application_password_password"),
		}

	case "note":
		title := str("note_title")
		if title == "" {
			title = name
		}
		rec = EntryRecord{
			Name:  title,
			Type:  "note",
			Notes: str("note_notes"),
		}

	case "bookmark":
		title := str("bookmark_title")
		if title == "" {
			title = name
		}
		rec = EntryRecord{
			Name: title,
			Type: "password",
			URL:  str("bookmark_url"),
		}

	case "totp":
		title := str("totp_title")
		if title == "" {
			title = name
		}
		rec = EntryRecord{
			Name:       title,
			Type:       "totp",
			TOTPSecret: str("totp_code"),
			Notes:      str("totp_notes"),
		}

	default:
		result.Skipped++
		if entryType != "" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("skipping unknown Psono type %q (%s)", entryType, name))
		}
		return
	}

	if rec.Name == "" {
		result.Skipped++
		result.Warnings = append(result.Warnings, "skipping unnamed Psono entry")
		return
	}

	rec.CreatedAt = normalizeTimestamp(str("create_date"))
	rec.UpdatedAt = normalizeTimestamp(str("write_date"))
	rec.FolderPath = folderPath

	// Custom fields
	if cfs, ok := item["custom_fields"].([]any); ok {
		for _, cf := range cfs {
			cfm, ok := cf.(map[string]any)
			if !ok {
				continue
			}
			label, _ := cfm["name"].(string)
			value, _ := cfm["value"].(string)
			if label != "" {
				rec.CustomFields = append(rec.CustomFields, CustomField{Label: label, Value: value})
			}
		}
	}

	result.Records = append(result.Records, rec)
}

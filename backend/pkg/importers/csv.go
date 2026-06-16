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
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
)

// CSVFormat specifies the column layout of a CSV import file.
type CSVFormat string

const (
	// CSVFormatGeneric expects columns: name,url,username,password,notes
	CSVFormatGeneric CSVFormat = "generic"
	// CSVFormatChrome exports from Chrome/Edge password manager: name,url,username,password
	CSVFormatChrome CSVFormat = "chrome"
	// CSVFormatLastPass exports from LastPass: url,username,password,totp,extra,name,grouping,fav
	CSVFormatLastPass CSVFormat = "lastpass"
	// CSVFormat1Password exports from 1Password: Title,Username,Password,URL,Notes,Type
	CSVFormat1Password CSVFormat = "1password"
)

// ParseCSV parses a CSV password export into EntryRecords.
func ParseCSV(data []byte, format CSVFormat) (*ImportResult, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv: parse: %w", err)
	}
	if len(records) < 2 {
		return &ImportResult{}, nil // header only or empty
	}

	result := &ImportResult{}
	rows := records[1:] // skip header

	for i, row := range rows {
		rec, warn := convertCSVRow(row, format, i+2)
		if rec == nil {
			result.Skipped++
			if warn != "" {
				result.Warnings = append(result.Warnings, warn)
			}
			continue
		}
		result.Records = append(result.Records, *rec)
	}
	return result, nil
}

func convertCSVRow(row []string, format CSVFormat, lineNum int) (*EntryRecord, string) {
	col := func(i int) string {
		if i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}

	var rec EntryRecord
	rec.Type = "password"

	switch format {
	case CSVFormatChrome:
		// name, url, username, password
		rec.Name = col(0)
		rec.URL = col(1)
		rec.Username = col(2)
		rec.Password = col(3)

	case CSVFormatLastPass:
		// url, username, password, totp, extra, name, grouping, fav
		rec.URL = col(0)
		rec.Username = col(1)
		rec.Password = col(2)
		rec.TOTPSecret = col(3)
		rec.Notes = col(4)
		rec.Name = col(5)
		if rec.TOTPSecret != "" {
			rec.Type = "totp"
		}

	case CSVFormat1Password:
		// Title, Username, Password, URL, Notes, Type
		rec.Name = col(0)
		rec.Username = col(1)
		rec.Password = col(2)
		rec.URL = col(3)
		rec.Notes = col(4)
		if t := col(5); t != "" {
			rec.Type = mapOnePasswordType(t)
		}

	default: // CSVFormatGeneric: name,url,username,password,notes
		rec.Name = col(0)
		rec.URL = col(1)
		rec.Username = col(2)
		rec.Password = col(3)
		rec.Notes = col(4)
	}

	if rec.Name == "" {
		rec.Name = rec.URL
	}
	if rec.Name == "" {
		return nil, fmt.Sprintf("line %d: skipping row with no name or URL", lineNum)
	}
	return &rec, ""
}

func mapOnePasswordType(t string) string {
	switch strings.ToLower(t) {
	case "login":
		return "password"
	case "secure note", "note":
		return "note"
	case "credit card":
		return "credit-card"
	case "identity":
		return "identity"
	default:
		return "password"
	}
}

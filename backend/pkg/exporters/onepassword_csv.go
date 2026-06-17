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

package exporters

import (
	"bytes"
	"encoding/csv"
)

// Export1PasswordCSV serialises records in 1Password CSV format:
// Title, Username, Password, URL, Notes, Type.
// File custom fields are skipped (CSV cannot carry binary data).
func Export1PasswordCSV(records []EntryRecord) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	_ = w.Write([]string{"Title", "Username", "Password", "URL", "Notes", "Type"})
	for _, rec := range records {
		_ = w.Write([]string{
			rec.Name,
			rec.Username,
			rec.Password,
			rec.URL,
			rec.Notes,
			map1PType(rec.Type),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func map1PType(t string) string {
	switch t {
	case "note":
		return "Secure Note"
	case "credit-card":
		return "Credit Card"
	case "identity":
		return "Identity"
	default:
		return "Login"
	}
}

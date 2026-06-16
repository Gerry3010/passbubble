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

// ExportCSV produces a generic CSV export compatible with Chrome/Edge import.
// Columns: name, url, username, password, notes
func ExportCSV(records []EntryRecord) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	_ = w.Write([]string{"name", "url", "username", "password", "notes"})
	for _, rec := range records {
		_ = w.Write([]string{
			rec.Name,
			rec.URL,
			rec.Username,
			rec.Password,
			rec.Notes,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

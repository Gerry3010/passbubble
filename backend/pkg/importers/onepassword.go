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
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Parse1PUX parses a 1Password 1PUX export ZIP and returns EntryRecords.
// The zipBytes argument is the raw content of a *.1pux file.
func Parse1PUX(zipBytes []byte) (*ImportResult, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("1pux: open zip: %w", err)
	}

	// Collect export.data and files/ blobs
	var exportData []byte
	fileBlobs := map[string][]byte{}

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			continue
		}

		switch {
		case f.Name == "export.data":
			exportData = data
		case strings.HasPrefix(f.Name, "files/"):
			docID := strings.TrimPrefix(f.Name, "files/")
			if docID != "" {
				fileBlobs[docID] = data
			}
		}
	}

	if exportData == nil {
		return nil, fmt.Errorf("1pux: export.data not found in archive")
	}

	var raw struct {
		Accounts []struct {
			Vaults []struct {
				Items []json.RawMessage `json:"items"`
			} `json:"vaults"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(exportData, &raw); err != nil {
		return nil, fmt.Errorf("1pux: parse export.data: %w", err)
	}

	result := &ImportResult{}
	for _, acc := range raw.Accounts {
		for _, vault := range acc.Vaults {
			for _, rawItem := range vault.Items {
				rec, warn := convert1PuxItem(rawItem, fileBlobs)
				if rec == nil {
					result.Skipped++
					if warn != "" {
						result.Warnings = append(result.Warnings, warn)
					}
					continue
				}
				result.Records = append(result.Records, *rec)
			}
		}
	}
	return result, nil
}

func convert1PuxItem(raw json.RawMessage, fileBlobs map[string][]byte) (*EntryRecord, string) {
	var item struct {
		UUID         string `json:"uuid"`
		Trashed      string `json:"trashed"`
		CategoryUUID string `json:"categoryUuid"`
		Details      struct {
			LoginFields []struct {
				Value       string `json:"value"`
				FieldType   string `json:"fieldType"`
				Designation string `json:"designation"`
			} `json:"loginFields"`
			NotesPlain string `json:"notesPlain"`
			Sections   []struct {
				Title  string `json:"title"`
				Fields []struct {
					Title     string          `json:"title"`
					ID        string          `json:"id"`
					Value     json.RawMessage `json:"value"`
					Multiline bool            `json:"multiline"`
				} `json:"fields"`
			} `json:"sections"`
		} `json:"details"`
		Overview struct {
			Title string `json:"title"`
			URLs  []struct {
				URL string `json:"url"`
			} `json:"urls"`
		} `json:"overview"`
		Files []struct {
			DocumentAttributes struct {
				FileName   string `json:"fileName"`
				DocumentID string `json:"documentId"`
			} `json:"documentAttributes"`
		} `json:"files"`
	}

	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, "failed to parse item: " + err.Error()
	}
	if item.Trashed == "Y" {
		return nil, ""
	}

	rec := &EntryRecord{
		Name:  item.Overview.Title,
		Notes: item.Details.NotesPlain,
	}
	if len(item.Overview.URLs) > 0 {
		rec.URL = item.Overview.URLs[0].URL
	}

	// Category → type
	switch item.CategoryUUID {
	case "003": // Secure Note
		rec.Type = "note"
	case "002": // Credit Card
		rec.Type = "credit-card"
	case "004": // Identity
		rec.Type = "identity"
	default: // 001 Login and everything else
		rec.Type = "password"
	}

	// Login fields (username / password)
	for _, lf := range item.Details.LoginFields {
		switch lf.Designation {
		case "username":
			rec.Username = lf.Value
		case "password":
			rec.Password = lf.Value
		}
	}

	// Sections → structured fields + custom fields
	for _, sec := range item.Details.Sections {
		for _, sf := range sec.Fields {
			valMap := map[string]any{}
			_ = json.Unmarshal(sf.Value, &valMap)

			handled := mapStructuredField(rec, sf.ID, sf.Title, valMap, sf.Multiline)
			if !handled {
				cfType, cfVal := onePuxValueToCustomField(valMap, sf.Multiline)
				if cfVal != "" || cfType == "file" {
					rec.CustomFields = append(rec.CustomFields, CustomField{
						Label: sf.Title,
						Value: cfVal,
						Type:  cfType,
					})
				}
			}
		}
	}

	// Detect TOTP from custom fields
	for _, cf := range rec.CustomFields {
		if cf.Type == "totp" && cf.Value != "" {
			rec.Type = "totp"
			rec.TOTPSecret = cf.Value
			break
		}
	}

	// File attachments
	for _, fileRef := range item.Files {
		docID := fileRef.DocumentAttributes.DocumentID
		fname := fileRef.DocumentAttributes.FileName
		content, ok := fileBlobs[docID]
		if !ok {
			continue
		}
		rec.CustomFields = append(rec.CustomFields, CustomField{
			Label:    fname,
			Value:    base64.StdEncoding.EncodeToString(content),
			Type:     "file",
			Filename: fname,
		})
	}

	if rec.Name == "" {
		rec.Name = rec.Username
	}
	if rec.Name == "" {
		return nil, "item has no title"
	}
	return rec, ""
}

// mapStructuredField maps known section field IDs to EntryRecord fields.
// Returns true if the field was handled as a structured field.
func mapStructuredField(rec *EntryRecord, id, title string, valMap map[string]any, multiline bool) bool {
	str := func(key string) string {
		if v, ok := valMap[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	switch id {
	case "username":
		if v := str("string"); v != "" {
			rec.Username = v
		}
	case "password":
		if v := str("concealed"); v != "" {
			rec.Password = v
		}
	case "totp":
		if v := str("totp"); v != "" {
			rec.TOTPSecret = v
			rec.Type = "totp"
		}
	// Credit card
	case "card_number", "ccnum":
		rec.CardNumber = str("creditCardNumber")
		if rec.CardNumber == "" {
			rec.CardNumber = str("string")
		}
	case "holder_name", "cardholder":
		rec.HolderName = str("string")
	case "expiry_month":
		rec.ExpiryMonth = str("string")
	case "expiry_year":
		rec.ExpiryYear = str("string")
	case "cvv", "cvvnum":
		rec.CVV = str("concealed")
	// Identity
	case "firstname":
		rec.FirstName = str("string")
	case "lastname":
		rec.LastName = str("string")
	case "company":
		rec.Company = str("string")
	case "email":
		rec.Email = str("email")
		if rec.Email == "" {
			rec.Email = str("string")
		}
	case "phone":
		rec.Phone = str("phone")
		if rec.Phone == "" {
			rec.Phone = str("string")
		}
	case "street":
		rec.Street = str("string")
	case "city":
		rec.City = str("string")
	case "state":
		rec.State = str("string")
	case "postal_code":
		rec.PostalCode = str("string")
	case "country":
		rec.Country = str("string")
	default:
		return false
	}
	return true
}

// onePuxValueToCustomField converts a 1PUX field value map to a (type, value) pair.
func onePuxValueToCustomField(valMap map[string]any, multiline bool) (cfType, cfVal string) {
	str := func(key string) string {
		if v, ok := valMap[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	if v := str("concealed"); v != "" {
		return "password", v
	}
	if v := str("totp"); v != "" {
		return "totp", v
	}
	if v := str("url"); v != "" {
		return "url", v
	}
	if v := str("email"); v != "" {
		return "email", v
	}
	if v := str("phone"); v != "" {
		return "phone", v
	}
	if v := str("sshKey"); v != "" {
		return "ssh", v
	}
	if v := str("string"); v != "" {
		if multiline {
			return "note", v
		}
		return "text", v
	}
	return "text", ""
}

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
	"encoding/json"
	"fmt"
	"time"
)

type bwExport struct {
	Encrypted bool       `json:"encrypted"`
	Folders   []any      `json:"folders"`
	Items     []bwItem   `json:"items"`
}

type bwItem struct {
	ID             string      `json:"id"`
	OrganizationID any         `json:"organizationId"`
	FolderID       any         `json:"folderId"`
	Type           int         `json:"type"`
	Reprompt       int         `json:"reprompt"`
	Name           string      `json:"name"`
	Notes          string      `json:"notes,omitempty"`
	FavoriteEntry  bool        `json:"favorite"`
	Fields         []bwField   `json:"fields,omitempty"`
	Login          *bwLogin    `json:"login,omitempty"`
	Card           *bwCard     `json:"card,omitempty"`
	Identity       *bwIdentity `json:"identity,omitempty"`
	RevisionDate   string      `json:"revisionDate"`
}

type bwLogin struct {
	URIS     []bwURI `json:"uris,omitempty"`
	Username string  `json:"username"`
	Password string  `json:"password"`
	TOTP     string  `json:"totp,omitempty"`
}

type bwURI struct {
	Match any    `json:"match"`
	URI   string `json:"uri"`
}

type bwCard struct {
	CardholderName string `json:"cardholderName"`
	Brand          string `json:"brand"`
	Number         string `json:"number"`
	ExpMonth       string `json:"expMonth"`
	ExpYear        string `json:"expYear"`
	Code           string `json:"code"`
}

type bwIdentity struct {
	Title      string `json:"title"`
	FirstName  string `json:"firstName"`
	MiddleName string `json:"middleName"`
	LastName   string `json:"lastName"`
	Company    string `json:"company"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	Address1   string `json:"address1"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postalCode"`
	Country    string `json:"country"`
}

type bwField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  int    `json:"type"` // 0=text, 1=hidden
}

// BitwardenExportOptions controls how the Bitwarden export is generated.
type BitwardenExportOptions struct {
	IncludeFiles  bool // include file custom fields in export
	FilesAsBase64 bool // encode file content as data: URI in a hidden field
}

// ExportBitwarden serialises records into a Bitwarden-compatible JSON export.
// Pass a zero-value BitwardenExportOptions for default behaviour (no files).
func ExportBitwarden(records []EntryRecord, opts BitwardenExportOptions) ([]byte, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	export := bwExport{
		Encrypted: false,
		Folders:   []any{},
		Items:     make([]bwItem, 0, len(records)),
	}

	for i, rec := range records {
		item := bwItem{
			ID:           fmt.Sprintf("passbubble-%d", i+1),
			Name:         rec.Name,
			Notes:        rec.Notes,
			RevisionDate: now,
		}

		for _, cf := range rec.CustomFields {
			cfType := cf.Type
			if cfType == "" {
				cfType = "text"
			}
			if cfType == "file" {
				if !opts.IncludeFiles {
					continue
				}
				if opts.FilesAsBase64 {
					mime := cf.MimeType
					if mime == "" {
						mime = "application/octet-stream"
					}
					item.Fields = append(item.Fields, bwField{
						Name:  cf.Filename,
						Value: "data:" + mime + ";base64," + cf.Value,
						Type:  1,
					})
				}
				continue
			}
			bwType := 0
			if cfType == "password" || cfType == "ssh" || cfType == "totp" {
				bwType = 1
			}
			item.Fields = append(item.Fields, bwField{Name: cf.Label, Value: cf.Value, Type: bwType})
		}

		switch rec.Type {
		case "totp":
			item.Type = 1
			item.Login = &bwLogin{
				Username: rec.Username,
				Password: rec.Password,
				TOTP:     rec.TOTPSecret,
			}
			if rec.URL != "" {
				item.Login.URIS = []bwURI{{URI: rec.URL}}
			}
		case "note":
			item.Type = 2
		case "credit-card":
			item.Type = 3
			item.Card = &bwCard{
				CardholderName: rec.HolderName,
				Number:         rec.CardNumber,
				ExpMonth:       rec.ExpiryMonth,
				ExpYear:        rec.ExpiryYear,
				Code:           rec.CVV,
			}
		case "identity":
			item.Type = 4
			item.Identity = &bwIdentity{
				FirstName:  rec.FirstName,
				LastName:   rec.LastName,
				Company:    rec.Company,
				Email:      rec.Email,
				Phone:      rec.Phone,
				Address1:   rec.Street,
				City:       rec.City,
				State:      rec.State,
				PostalCode: rec.PostalCode,
				Country:    rec.Country,
			}
		default: // password, api-key, ssh-key, etc.
			item.Type = 1
			item.Login = &bwLogin{
				Username: rec.Username,
				Password: rec.Password,
			}
			if rec.URL != "" {
				item.Login.URIS = []bwURI{{URI: rec.URL}}
			}
		}

		export.Items = append(export.Items, item)
	}

	return json.MarshalIndent(export, "", "  ")
}

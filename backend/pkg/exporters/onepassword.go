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

// Package exporters: 1PUX (1Password interchange format) exporter.
// Produces a ZIP file named *.1pux containing export.data (JSON) and files/.
package exporters

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// 1PUX category UUIDs
const (
	onePuxCatLogin      = "001"
	onePuxCatCreditCard = "002"
	onePuxCatNote       = "003"
	onePuxCatIdentity   = "004"
)

// ── JSON structures ───────────────────────────────────────────────────────────

type onePuxExport struct {
	Accounts []onePuxAccount `json:"accounts"`
}

type onePuxAccount struct {
	Attrs  onePuxAccountAttrs `json:"attrs"`
	Vaults []onePuxVault      `json:"vaults"`
}

type onePuxAccountAttrs struct {
	AccountName string `json:"accountName"`
	Name        string `json:"name"`
	CreatedAt   int64  `json:"createdAt"`
	LastAuthAt  int64  `json:"lastAuthAt"`
}

type onePuxVault struct {
	Attrs onePuxVaultAttrs `json:"attrs"`
	Items []onePuxItem     `json:"items"`
}

type onePuxVaultAttrs struct {
	UUID      string `json:"uuid"`
	Desc      string `json:"desc"`
	Avatar    string `json:"avatar"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	CreatedAt int64  `json:"createdAt"`
}

type onePuxItem struct {
	UUID         string           `json:"uuid"`
	FavIndex     int              `json:"favIndex"`
	CreatedAt    int64            `json:"createdAt"`
	UpdatedAt    int64            `json:"updatedAt"`
	Trashed      string           `json:"trashed"`
	CategoryUUID string           `json:"categoryUuid"`
	Details      onePuxDetails    `json:"details"`
	Overview     onePuxOverview   `json:"overview"`
	Files        []onePuxFileRef  `json:"files,omitempty"`
}

type onePuxDetails struct {
	LoginFields     []onePuxLoginField `json:"loginFields,omitempty"`
	NotesPlain      string             `json:"notesPlain,omitempty"`
	Sections        []onePuxSection    `json:"sections,omitempty"`
	PasswordHistory []any              `json:"passwordHistory"`
}

type onePuxLoginField struct {
	Value       string `json:"value"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	FieldType   string `json:"fieldType"`
	Designation string `json:"designation,omitempty"`
}

type onePuxSection struct {
	Title  string          `json:"title"`
	Fields []onePuxSField `json:"fields,omitempty"`
}

type onePuxSField struct {
	Title       string         `json:"title"`
	ID          string         `json:"id"`
	Value       map[string]any `json:"value"`
	IndexAt     int            `json:"indexAtSource"`
	Guarded     bool           `json:"guarded"`
	Multiline   bool           `json:"multiline"`
	DontGen     bool           `json:"dontGenerate"`
	InputTraits map[string]any `json:"inputTraits"`
}

type onePuxOverview struct {
	Subtitle string        `json:"subtitle,omitempty"`
	URLs     []onePuxURL   `json:"urls,omitempty"`
	Title    string        `json:"title"`
	AInfo    string        `json:"ainfo,omitempty"`
	Tags     []string      `json:"tags"`
}

type onePuxURL struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type onePuxFileRef struct {
	DocumentAttributes onePuxDocAttrs  `json:"documentAttributes"`
	Overview           map[string]any  `json:"overview"`
}

type onePuxDocAttrs struct {
	FileName      string `json:"fileName"`
	DocumentID    string `json:"documentId"`
	DecryptedSize int    `json:"decryptedSize"`
}

// ── Export1PUX ────────────────────────────────────────────────────────────────

// Export1PUX serialises records into a 1PUX ZIP archive.
// Returns the raw ZIP bytes suitable for writing as a *.1pux file.
func Export1PUX(records []EntryRecord) ([]byte, error) {
	now := time.Now().Unix()

	// file content map: documentId → raw bytes
	fileBlobs := map[string][]byte{}

	items := make([]onePuxItem, 0, len(records))
	for _, rec := range records {
		item := buildOnePuxItem(rec, now, fileBlobs)
		items = append(items, item)
	}

	export := onePuxExport{
		Accounts: []onePuxAccount{{
			Attrs: onePuxAccountAttrs{
				AccountName: "Passbubble",
				Name:        "Passbubble Export",
				CreatedAt:   now,
				LastAuthAt:  now,
			},
			Vaults: []onePuxVault{{
				Attrs: onePuxVaultAttrs{
					UUID:      uuid.NewString(),
					Name:      "Personal",
					Type:      "P",
					CreatedAt: now,
				},
				Items: items,
			}},
		}},
	}

	exportData, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return nil, err
	}

	// Build ZIP
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	f, err := zw.Create("export.data")
	if err != nil {
		return nil, err
	}
	if _, err = f.Write(exportData); err != nil {
		return nil, err
	}

	for docID, content := range fileBlobs {
		f, err := zw.Create("files/" + docID)
		if err != nil {
			return nil, err
		}
		if _, err = f.Write(content); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── item builder ─────────────────────────────────────────────────────────────

func buildOnePuxItem(rec EntryRecord, now int64, fileBlobs map[string][]byte) onePuxItem {
	id := uuid.NewString()
	item := onePuxItem{
		UUID:      id,
		CreatedAt: now,
		UpdatedAt: now,
		Trashed:   "N",
		Overview: onePuxOverview{
			Title: rec.Name,
			Tags:  []string{},
		},
		Details: onePuxDetails{
			NotesPlain:      rec.Notes,
			PasswordHistory: []any{},
		},
	}
	if rec.URL != "" {
		item.Overview.URLs = []onePuxURL{{Label: "website", URL: rec.URL}}
	}

	switch rec.Type {
	case "note":
		item.CategoryUUID = onePuxCatNote

	case "credit-card":
		item.CategoryUUID = onePuxCatCreditCard
		item.Details.Sections = []onePuxSection{{
			Title: "Card Details",
			Fields: []onePuxSField{
				sfield("card_number", "Card Number", "creditCardNumber", rec.CardNumber, false),
				sfield("holder_name", "Cardholder Name", "string", rec.HolderName, false),
				sfield("expiry_month", "Expiry Month", "string", rec.ExpiryMonth, false),
				sfield("expiry_year", "Expiry Year", "string", rec.ExpiryYear, false),
				sfield("cvv", "CVV", "concealed", rec.CVV, false),
			},
		}}
		item.Overview.Subtitle = rec.HolderName

	case "identity":
		item.CategoryUUID = onePuxCatIdentity
		item.Details.Sections = []onePuxSection{
			{Title: "Name", Fields: []onePuxSField{
				sfield("firstname", "First Name", "string", rec.FirstName, false),
				sfield("lastname", "Last Name", "string", rec.LastName, false),
				sfield("company", "Company", "string", rec.Company, false),
			}},
			{Title: "Address", Fields: []onePuxSField{
				sfield("street", "Street", "string", rec.Street, false),
				sfield("city", "City", "string", rec.City, false),
				sfield("state", "State", "string", rec.State, false),
				sfield("postal_code", "Postal Code", "string", rec.PostalCode, false),
				sfield("country", "Country", "string", rec.Country, false),
			}},
			{Title: "Contact", Fields: []onePuxSField{
				sfield("email", "Email", "email", rec.Email, false),
				sfield("phone", "Phone", "phone", rec.Phone, false),
			}},
		}
		item.Overview.Subtitle = rec.FirstName + " " + rec.LastName

	default: // Login: password, totp, api-key, ssh-key, bank-account, license, etc.
		item.CategoryUUID = onePuxCatLogin
		item.Details.LoginFields = []onePuxLoginField{
			{Value: rec.Username, ID: "username", Name: "username", FieldType: "T", Designation: "username"},
			{Value: rec.Password, ID: "password", Name: "password", FieldType: "P", Designation: "password"},
		}
		item.Overview.Subtitle = rec.Username
		item.Overview.AInfo = rec.Username

		// TOTP as section field
		if rec.TOTPSecret != "" {
			item.Details.Sections = append(item.Details.Sections, onePuxSection{
				Title: "One-Time Password",
				Fields: []onePuxSField{
					sfield("totp", "One-Time Password", "totp", rec.TOTPSecret, false),
				},
			})
		}

		// Type-specific extra sections
		switch rec.Type {
		case "license":
			item.Details.Sections = append(item.Details.Sections, onePuxSection{
				Title: "License",
				Fields: []onePuxSField{
					sfield("product_name", "Product Name", "string", rec.ProductName, false),
					sfield("license_key", "License Key", "concealed", rec.LicenseKey, false),
					sfield("purchase_email", "Purchase Email", "email", rec.ProductName, false),
				},
			})
		}
	}

	// Custom fields → additional section
	customSection := onePuxSection{Title: "Custom Fields"}
	for _, cf := range rec.CustomFields {
		cfType := cf.Type
		if cfType == "" {
			cfType = "text"
		}

		if cfType == "file" {
			// Decode base64, store in files/ and add a file reference
			raw, err := base64.StdEncoding.DecodeString(cf.Value)
			if err != nil {
				continue
			}
			docID := uuid.NewString()
			fileBlobs[docID] = raw
			fname := cf.Filename
			if fname == "" {
				fname = cf.Label
			}
			item.Files = append(item.Files, onePuxFileRef{
				DocumentAttributes: onePuxDocAttrs{
					FileName:      fname,
					DocumentID:    docID,
					DecryptedSize: len(raw),
				},
				Overview: map[string]any{"fileName": fname},
			})
			continue
		}

		valueKey, valueVal := cfTypeToOnePux(cfType, cf.Value)
		multiline := cfType == "note" || cfType == "ssh"
		f := onePuxSField{
			Title:     cf.Label,
			ID:        uuid.NewString(),
			Value:     map[string]any{valueKey: valueVal},
			Multiline: multiline,
			InputTraits: map[string]any{
				"keyboard":       "default",
				"correction":     "default",
				"capitalization": "default",
			},
		}
		customSection.Fields = append(customSection.Fields, f)
	}
	if len(customSection.Fields) > 0 {
		item.Details.Sections = append(item.Details.Sections, customSection)
	}

	return item
}

func sfield(id, title, valueKey, value string, guarded bool) onePuxSField {
	return onePuxSField{
		Title:   title,
		ID:      id,
		Value:   map[string]any{valueKey: value},
		Guarded: guarded,
		InputTraits: map[string]any{
			"keyboard":       "default",
			"correction":     "default",
			"capitalization": "default",
		},
	}
}

func cfTypeToOnePux(cfType, value string) (key string, val any) {
	switch cfType {
	case "password", "ssh":
		return "concealed", value
	case "totp":
		return "totp", value
	case "url":
		return "url", value
	case "email":
		return "email", value
	case "phone":
		return "phone", value
	default: // text, note
		return "string", value
	}
}

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
	"strings"
)

// Bitwarden JSON export structures (unencrypted individual export format).
type bitwardenExport struct {
	Items []bitwardenItem `json:"items"`
}

type bitwardenItem struct {
	Type   int    `json:"type"` // 1=login, 2=note, 3=card, 4=identity
	Name   string `json:"name"`
	Notes  string `json:"notes"`
	Login  *bitwardenLogin    `json:"login,omitempty"`
	Card   *bitwardenCard     `json:"card,omitempty"`
	Identity *bitwardenIdentity `json:"identity,omitempty"`
	Fields []bitwardenField   `json:"fields,omitempty"`
}

type bitwardenLogin struct {
	Username string              `json:"username"`
	Password string              `json:"password"`
	URIs     []bitwardenURI      `json:"uris,omitempty"`
	TOTP     string              `json:"totp,omitempty"`
}

type bitwardenURI struct {
	URI string `json:"uri"`
}

type bitwardenCard struct {
	CardholderName string `json:"cardholderName"`
	Number         string `json:"number"`
	Brand          string `json:"brand"`
	ExpMonth       string `json:"expMonth"`
	ExpYear        string `json:"expYear"`
	Code           string `json:"code"`
}

type bitwardenIdentity struct {
	Title      string `json:"title"`
	FirstName  string `json:"firstName"`
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

type bitwardenField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ParseBitwarden parses a Bitwarden unencrypted JSON export.
func ParseBitwarden(data []byte) (*ImportResult, error) {
	var export bitwardenExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("bitwarden: invalid JSON: %w", err)
	}

	result := &ImportResult{}
	for _, item := range export.Items {
		rec, warn := convertBitwardenItem(item)
		if rec == nil {
			result.Skipped++
			if warn != "" {
				result.Warnings = append(result.Warnings, warn)
			}
			continue
		}
		for _, f := range item.Fields {
			if f.Name != "" {
				rec.CustomFields = append(rec.CustomFields, CustomField{Label: f.Name, Value: f.Value})
			}
		}
		result.Records = append(result.Records, *rec)
	}
	return result, nil
}

func convertBitwardenItem(item bitwardenItem) (*EntryRecord, string) {
	rec := &EntryRecord{Name: item.Name, Notes: item.Notes}

	switch item.Type {
	case 1: // login
		rec.Type = "password"
		if item.Login != nil {
			rec.Username = item.Login.Username
			rec.Password = item.Login.Password
			rec.TOTPSecret = item.Login.TOTP
			if len(item.Login.URIs) > 0 {
				rec.URL = item.Login.URIs[0].URI
			}
			if rec.TOTPSecret != "" {
				rec.Type = "totp"
			}
		}
	case 2: // secure note
		rec.Type = "note"
	case 3: // card
		rec.Type = "credit-card"
		if item.Card != nil {
			rec.CardNumber = item.Card.Number
			rec.HolderName = item.Card.CardholderName
			rec.ExpiryMonth = item.Card.ExpMonth
			rec.ExpiryYear = item.Card.ExpYear
			rec.CVV = item.Card.Code
		}
	case 4: // identity
		rec.Type = "identity"
		if item.Identity != nil {
			id := item.Identity
			rec.FirstName = id.FirstName
			rec.LastName = id.LastName
			rec.Company = id.Company
			rec.Email = id.Email
			rec.Phone = id.Phone
			rec.Street = id.Address1
			rec.City = id.City
			rec.State = id.State
			rec.PostalCode = id.PostalCode
			rec.Country = id.Country
		}
	default:
		return nil, fmt.Sprintf("skipping unknown Bitwarden item type %d (%s)", item.Type, item.Name)
	}

	if rec.Name == "" {
		rec.Name = strings.TrimSpace(rec.Username + " " + rec.URL)
		if rec.Name == "" {
			return nil, "skipping unnamed entry"
		}
	}
	return rec, ""
}

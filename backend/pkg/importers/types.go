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

// Package importers parses external password manager export formats into
// a common EntryRecord representation. Records are plaintext — callers
// must encrypt before uploading to the Passbubble API.
package importers

// EntryRecord is the plaintext intermediate format used during import.
// Every field maps directly to the Passbubble EntryData JSON blob.
type EntryRecord struct {
	// Metadata (stored unencrypted on server)
	Name string
	URL  string
	Type string // "password", "totp", "note", "credit-card", etc.

	// Plaintext fields (must be encrypted before upload)
	Username    string
	Password    string
	TOTPSecret  string
	Notes       string

	// Credit card
	CardNumber  string
	HolderName  string
	ExpiryMonth string
	ExpiryYear  string
	CVV         string

	// Identity
	FirstName  string
	LastName   string
	Company    string
	Email      string
	Phone      string
	Street     string
	City       string
	State      string
	PostalCode string
	Country    string

	// License
	LicenseKey    string
	ProductName   string
	PurchaseEmail string

	// Generic key-value extras
	CustomFields []CustomField
}

// CustomField is a user-defined field inside an entry.
// Type is one of: text, password, totp, url, email, phone, note, ssh, file.
// Omitting Type is equivalent to "text" for backward compatibility.
type CustomField struct {
	Label    string
	Value    string
	Type     string // text|password|totp|url|email|phone|note|ssh|file
	Filename string // only for type=file
	MimeType string // only for type=file
}

// ImportResult summarises an import run.
type ImportResult struct {
	Records  []EntryRecord
	Skipped  int
	Warnings []string
}

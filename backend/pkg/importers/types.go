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

import "time"

// normalizeTimestamp parses a source timestamp into an RFC3339 UTC string.
// Returns "" when the input is empty or unparseable (caller falls back to NOW()).
func normalizeTimestamp(s string) string {
	if s == "" {
		return ""
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999999Z07:00", "2006-01-02 15:04:05"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

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

	// Original timestamps from the source export (RFC3339 UTC; "" = unknown).
	CreatedAt string
	UpdatedAt string

	// Folder hierarchy root→leaf (e.g. ["Business","Gewerbe"]; empty = root).
	// Segments are literal names and may themselves contain "/".
	FolderPath []string

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

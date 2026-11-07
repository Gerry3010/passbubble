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

// Package exporters serialises Passbubble entries into external password
// manager formats. All input data is plaintext — callers must decrypt
// entries before passing them here.
package exporters

// EntryRecord mirrors importers.EntryRecord and is kept in a separate package
// to avoid an import cycle with the importers package.
type EntryRecord struct {
	Name          string
	URL           string
	Type          string
	Username      string
	Password      string
	TOTPSecret    string
	Notes         string
	SignInWith    string
	Issuer        string
	Period        int
	Digits        int
	Algorithm     string
	CardNumber    string
	HolderName    string
	ExpiryMonth   string
	ExpiryYear    string
	CVV           string
	BankName      string
	IBAN          string
	BIC           string
	AccountNumber string
	AccountType   string
	Title         string
	FirstName     string
	LastName      string
	Company       string
	Email         string
	Phone         string
	Street        string
	City          string
	State         string
	PostalCode    string
	Country       string
	LicenseKey    string
	ProductName   string
	PurchaseEmail string
	PurchaseDate  string
	ExpiresAt     string
	CustomFields  []CustomField
	Tags          []string
	// UnknownFields holds what the entry has beyond the fields above: keys from
	// other clients or newer versions, whose meaning nobody here can vouch for.
	UnknownFields []UnknownField
}

// UnknownField is a value of an entry under the key it has in the entry JSON.
type UnknownField struct {
	Key   string
	Value string
}

// CustomField is a user-defined field on an entry.
// Type is one of: text, password, totp, url, email, phone, note, ssh, file.
// Omitting Type is equivalent to "text" for backward compatibility.
type CustomField struct {
	Label    string
	Value    string
	Type     string // text|password|totp|url|email|phone|note|ssh|file
	Filename string // only for type=file
	MimeType string // only for type=file
}

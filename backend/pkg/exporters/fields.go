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

import "strconv"

// keyedField is a value of a record under the key it has in the entry JSON.
type keyedField struct {
	key    string
	value  string
	secret bool // to be masked where a format can do that
}

// keyedFields lists the values a format may or may not have a place for: all of
// a record but its name, notes, tags, custom fields and unknown fields.
func (rec EntryRecord) keyedFields() []keyedField {
	number := func(n int) string {
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	return []keyedField{
		{key: "url", value: rec.URL},
		{key: "username", value: rec.Username},
		{key: "password", value: rec.Password, secret: true},
		{key: "totp_secret", value: rec.TOTPSecret, secret: true},
		{key: "sign_in_with", value: rec.SignInWith},
		{key: "issuer", value: rec.Issuer},
		{key: "period", value: number(rec.Period)},
		{key: "digits", value: number(rec.Digits)},
		{key: "algorithm", value: rec.Algorithm},
		{key: "card_number", value: rec.CardNumber, secret: true},
		{key: "holder_name", value: rec.HolderName},
		{key: "expiry_month", value: rec.ExpiryMonth},
		{key: "expiry_year", value: rec.ExpiryYear},
		{key: "cvv", value: rec.CVV, secret: true},
		{key: "bank_name", value: rec.BankName},
		{key: "iban", value: rec.IBAN, secret: true},
		{key: "bic", value: rec.BIC},
		{key: "account_number", value: rec.AccountNumber, secret: true},
		{key: "account_type", value: rec.AccountType},
		{key: "title", value: rec.Title},
		{key: "first_name", value: rec.FirstName},
		{key: "last_name", value: rec.LastName},
		{key: "company", value: rec.Company},
		{key: "email", value: rec.Email},
		{key: "phone", value: rec.Phone},
		{key: "street", value: rec.Street},
		{key: "city", value: rec.City},
		{key: "state", value: rec.State},
		{key: "postal_code", value: rec.PostalCode},
		{key: "country", value: rec.Country},
		{key: "product_name", value: rec.ProductName},
		{key: "license_key", value: rec.LicenseKey, secret: true},
		{key: "purchase_email", value: rec.PurchaseEmail},
		{key: "purchase_date", value: rec.PurchaseDate},
		{key: "expires_at", value: rec.ExpiresAt},
	}
}

// fieldsOutside returns the keyed fields with a value that are not among the keys
// a format has a place for.
func (rec EntryRecord) fieldsOutside(placed map[string]bool) []keyedField {
	var out []keyedField
	for _, f := range rec.keyedFields() {
		if f.value != "" && !placed[f.key] {
			out = append(out, f)
		}
	}
	return out
}

func keySet(keys ...string) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set
}

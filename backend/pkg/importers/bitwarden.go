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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// bitwardenTagsField is the custom field that carries the tags: Bitwarden has
// none of its own. The exporter and the app write the same name.
const bitwardenTagsField = "passbubble_tags"

// bitwardenFieldHidden is the field type Bitwarden masks.
const bitwardenFieldHidden = 1

// Bitwarden JSON export structures (unencrypted individual export format).
type bitwardenExport struct {
	Folders []bitwardenFolder `json:"folders"`
	Items   []bitwardenItem   `json:"items"`
}

type bitwardenFolder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type bitwardenItem struct {
	Type     int                `json:"type"` // 1=login, 2=note, 3=card, 4=identity
	Name     string             `json:"name"`
	Notes    string             `json:"notes"`
	FolderID string             `json:"folderId"`
	Login    *bitwardenLogin    `json:"login,omitempty"`
	Card     *bitwardenCard     `json:"card,omitempty"`
	Identity *bitwardenIdentity `json:"identity,omitempty"`
	Fields   []bitwardenField   `json:"fields,omitempty"`
	// Bitwarden timestamps (ISO8601 UTC).
	CreationDate string `json:"creationDate"`
	RevisionDate string `json:"revisionDate"`
}

type bitwardenLogin struct {
	Username         string           `json:"username"`
	Password         string           `json:"password"`
	URIs             []bitwardenURI   `json:"uris,omitempty"`
	TOTP             string           `json:"totp,omitempty"`
	Fido2Credentials []bitwardenFido2 `json:"fido2Credentials,omitempty"`
}

// bitwardenFido2 is a passkey as Bitwarden exports it. The counter is left out
// on purpose: Passbubble's is always 0 (docs/passkeys.md).
type bitwardenFido2 struct {
	CredentialID    string        `json:"credentialId"`
	KeyType         string        `json:"keyType"`
	KeyAlgorithm    string        `json:"keyAlgorithm"`
	KeyCurve        string        `json:"keyCurve"`
	KeyValue        string        `json:"keyValue"`
	RPID            string        `json:"rpId"`
	RPName          string        `json:"rpName"`
	UserHandle      string        `json:"userHandle"`
	UserName        string        `json:"userName"`
	UserDisplayName string        `json:"userDisplayName"`
	Discoverable    bitwardenBool `json:"discoverable"`
	CreationDate    string        `json:"creationDate"`
}

// bitwardenBool reads Bitwarden's "true"/"false" strings, and a JSON boolean
// from files other tools wrote.
type bitwardenBool bool

func (b *bitwardenBool) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch v := v.(type) {
	case bool:
		*b = bitwardenBool(v)
	case string:
		*b = bitwardenBool(strings.EqualFold(strings.TrimSpace(v), "true"))
	case nil:
		*b = false
	default:
		return fmt.Errorf("bitwarden: discoverable is %T, want a string", v)
	}
	return nil
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
	Type  int    `json:"type"`
}

// ParseBitwarden parses a Bitwarden unencrypted JSON export.
func ParseBitwarden(data []byte) (*ImportResult, error) {
	var export bitwardenExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("bitwarden: invalid JSON: %w", err)
	}

	// Map folder id → name for folder assignment.
	folderName := make(map[string]string, len(export.Folders))
	for _, f := range export.Folders {
		folderName[f.ID] = f.Name
	}

	result := &ImportResult{}
	for _, item := range export.Items {
		rec, warn, skippedPasskeys := convertBitwardenItem(item)
		if rec == nil {
			result.Skipped++
			if warn != "" {
				result.Warnings = append(result.Warnings, warn)
			}
			continue
		}
		result.SkippedPasskeys += len(skippedPasskeys)
		result.Warnings = append(result.Warnings, skippedPasskeys...)
		for _, f := range item.Fields {
			if f.Name == bitwardenTagsField {
				rec.Tags = append(rec.Tags, splitTags(f.Value, ",")...)
				continue
			}
			if f.Name != "" {
				rec.CustomFields = append(rec.CustomFields, bitwardenCustomField(f))
			}
		}
		rec.CreatedAt = normalizeTimestamp(item.CreationDate)
		rec.UpdatedAt = normalizeTimestamp(item.RevisionDate)
		// Bitwarden nests folders via "/" in the folder name.
		if name := folderName[item.FolderID]; name != "" {
			rec.FolderPath = splitFolderName(name)
		}
		result.Records = append(result.Records, *rec)
	}
	return result, nil
}

// bitwardenCustomField maps a field to its kind, as far as the format tells it:
// a hidden field is a password, or a file when it holds a base64 data: URI (how
// the exporter writes files). Other field types are plain text.
func bitwardenCustomField(f bitwardenField) CustomField {
	cf := CustomField{Label: f.Name, Value: f.Value}
	if f.Type != bitwardenFieldHidden {
		return cf
	}
	if mime, content, ok := splitBase64DataURI(f.Value); ok {
		return CustomField{Label: f.Name, Value: content, Type: "file", Filename: f.Name, MimeType: mime}
	}
	cf.Type = "password"
	return cf
}

// splitBase64DataURI splits "data:<mime>;base64,<content>".
func splitBase64DataURI(value string) (mime, content string, ok bool) {
	rest, ok := strings.CutPrefix(value, "data:")
	if !ok {
		return "", "", false
	}
	mime, content, ok = strings.Cut(rest, ";base64,")
	if !ok {
		return "", "", false
	}
	if _, err := base64.StdEncoding.DecodeString(content); err != nil {
		return "", "", false
	}
	return mime, content, true
}

// splitFolderName splits a "/"-nested folder name into path segments,
// trimming surrounding whitespace and dropping empty segments.
func splitFolderName(name string) []string {
	parts := strings.Split(name, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// convertBitwardenItem maps one item. A nil record means the item is skipped,
// for the reason in warn; skippedPasskeys explains each passkey left out of a
// record that is kept.
func convertBitwardenItem(item bitwardenItem) (rec *EntryRecord, warn string, skippedPasskeys []string) {
	rec = &EntryRecord{Name: item.Name, Notes: item.Notes}

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
			for _, c := range item.Login.Fido2Credentials {
				pk, err := convertBitwardenPasskey(c)
				if err != nil {
					skippedPasskeys = append(skippedPasskeys,
						fmt.Sprintf("%s: skipped a passkey for %q: %v", bitwardenItemLabel(item), c.RPID, err))
					continue
				}
				rec.Passkeys = append(rec.Passkeys, pk)
			}
			rec.settleLoginType()
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
			rec.Title = id.Title
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
		return nil, fmt.Sprintf("skipping unknown Bitwarden item type %d (%s)", item.Type, item.Name), nil
	}

	if rec.Name == "" {
		rec.Name = strings.TrimSpace(rec.Username + " " + rec.URL)
		if rec.Name == "" {
			return nil, "skipping unnamed entry", nil
		}
	}
	return rec, "", skippedPasskeys
}

// bitwardenItemLabel names an item in a warning, also one without a name.
func bitwardenItemLabel(item bitwardenItem) string {
	if item.Name != "" {
		return item.Name
	}
	if item.Login != nil && item.Login.Username != "" {
		return item.Login.Username
	}
	return "unnamed entry"
}

// convertBitwardenPasskey maps a Bitwarden passkey to the stored form, along
// the table in docs/passkeys.md. Anything but ES256 is refused.
func convertBitwardenPasskey(c bitwardenFido2) (Passkey, error) {
	if !strings.EqualFold(c.KeyType, "public-key") || !strings.EqualFold(c.KeyAlgorithm, "ECDSA") ||
		!strings.EqualFold(c.KeyCurve, "P-256") {
		return Passkey{}, fmt.Errorf("unsupported key (%s %s %s), only ECDSA P-256", c.KeyType, c.KeyAlgorithm, c.KeyCurve)
	}
	if strings.TrimSpace(c.RPID) == "" {
		return Passkey{}, errors.New("no relying party id")
	}
	id, err := bitwardenCredentialID(c.CredentialID)
	if err != nil {
		return Passkey{}, err
	}
	key, err := normalisePasskeyPrivateKey(c.KeyValue)
	if err != nil {
		return Passkey{}, err
	}
	userHandle := ""
	if c.UserHandle != "" {
		if userHandle, err = normaliseBase64URL(c.UserHandle); err != nil {
			return Passkey{}, fmt.Errorf("user handle is not base64: %w", err)
		}
	}
	return Passkey{
		CredentialID:    encodeBase64URL(id),
		RPID:            strings.TrimSpace(c.RPID),
		RPName:          c.RPName,
		UserHandle:      userHandle,
		UserName:        c.UserName,
		UserDisplayName: c.UserDisplayName,
		KeyAlgorithm:    coseES256,
		PrivateKey:      key,
		Discoverable:    bool(c.Discoverable),
		CreatedAt:       normalizeTimestamp(c.CreationDate),
	}, nil
}

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

package vault

import (
	"fmt"

	"github.com/Gerry3010/passbubble/cli/pkg/keyring"
)

// KeyringAdapter wraps Vault to implement keyring.KeyringInterface.
// This allows the TUI (which was written against the keyring interface) to
// talk to the API-backed vault without modification.
type KeyringAdapter struct {
	v *Vault
}

// NewKeyringAdapter creates a KeyringAdapter from an authenticated Vault.
func NewKeyringAdapter(v *Vault) *KeyringAdapter {
	return &KeyringAdapter{v: v}
}

func (a *KeyringAdapter) IsAvailable() bool {
	return a.v.cfg.IsLoggedIn()
}

func (a *KeyringAdapter) Store(service, username, password string) error {
	if !a.v.IsUnlocked() {
		return fmt.Errorf("vault is locked — master password required")
	}
	_, err := a.v.CreateEntry(service, "password", "", &EntryData{
		Username: username,
		Password: password,
	}, nil, "", "", nil)
	return err
}

func (a *KeyringAdapter) StoreEntry(e keyring.Entry) error {
	if !a.v.IsUnlocked() {
		return fmt.Errorf("vault is locked — master password required")
	}
	data := &EntryData{
		Username:  e.Username,
		Notes:     e.Notes,
		Issuer:    e.Issuer,
		Period:    e.Period,
		Digits:    e.Digits,
		Algorithm: e.Algorithm,
	}
	// keyring.Entry has one secret field; the entry data has one per kind.
	if e.SecretType == keyring.SecretTypeTOTP {
		data.TOTPSecret = e.Password
	} else {
		data.Password = e.Password
	}
	entryType := string(e.SecretType)
	if entryType == "" {
		entryType = "password"
	}
	_, err := a.v.CreateEntry(e.Service, entryType, "", data, nil, "", "", nil)
	return err
}

// findEntry returns the one entry service and username name. An empty username
// means the caller does not know it. Names are not unique, so several matches
// are an error: guessing would hand out or delete another entry. Callers that
// know the entry's ID should go through Vault instead.
func (a *KeyringAdapter) findEntry(service, username string) (*Entry, error) {
	if !a.v.IsUnlocked() {
		return nil, fmt.Errorf("vault is locked — master password required")
	}
	entries, err := a.v.ListEntries() // search stops at 50 hits and could miss a twin
	if err != nil {
		return nil, err
	}
	var found *Entry
	for _, e := range entries {
		if e.Name != service {
			continue
		}
		full, err := a.v.GetEntry(e.ID)
		if err != nil {
			return nil, err
		}
		if full.Data == nil || (username != "" && full.Data.Username != username) {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("%w for %s", keyring.ErrAmbiguous, service)
		}
		found = full
	}
	if found == nil {
		return nil, fmt.Errorf("no entry found for %s", service)
	}
	return found, nil
}

// toKeyringEntry converts a decrypted entry. Its secret goes into Password,
// which holds the TOTP secret for TOTP entries.
func toKeyringEntry(name, url, entryType string, data *EntryData) keyring.Entry {
	secret := data.Password
	if entryType == string(keyring.SecretTypeTOTP) {
		secret = data.TOTPSecret
	}
	return keyring.Entry{
		Service:    name,
		Username:   data.Username,
		Password:   secret,
		Label:      url,
		SecretType: keyring.SecretType(entryType),
		Issuer:     data.Issuer,
		Period:     data.Period,
		Digits:     data.Digits,
		Algorithm:  data.Algorithm,
		Notes:      data.Notes,
	}
}

func (a *KeyringAdapter) Get(service, username string) (string, error) {
	entry, err := a.GetEntry(service, username)
	if err != nil {
		return "", err
	}
	return entry.Password, nil
}

func (a *KeyringAdapter) GetEntry(service, username string) (*keyring.Entry, error) {
	full, err := a.findEntry(service, username)
	if err != nil {
		return nil, err
	}
	entry := toKeyringEntry(full.Name, full.URL, full.Type, full.Data)
	return &entry, nil
}

func (a *KeyringAdapter) Delete(service, username string) error {
	full, err := a.findEntry(service, username)
	if err != nil {
		return err
	}
	return a.v.DeleteEntry(full.ID)
}

func (a *KeyringAdapter) List() ([]keyring.Entry, error) {
	// Returns metadata only — password field is empty until GetEntry is called.
	entries, err := a.v.ListEntries()
	if err != nil {
		return nil, err
	}
	result := make([]keyring.Entry, len(entries))
	for i, e := range entries {
		result[i] = keyring.Entry{
			Service:    e.Name,
			SecretType: keyring.SecretType(e.Type),
			Label:      e.URL,
		}
	}
	return result, nil
}

func (a *KeyringAdapter) Search(pattern string) ([]keyring.Entry, error) {
	entries, err := a.v.SearchEntries(pattern)
	if err != nil {
		return nil, err
	}
	result := make([]keyring.Entry, len(entries))
	for i, e := range entries {
		result[i] = keyring.Entry{
			Service:    e.Name,
			SecretType: keyring.SecretType(e.Type),
			Label:      e.URL,
		}
	}
	return result, nil
}

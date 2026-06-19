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
	}, nil, "", "")
	return err
}

func (a *KeyringAdapter) StoreEntry(e keyring.Entry) error {
	if !a.v.IsUnlocked() {
		return fmt.Errorf("vault is locked — master password required")
	}
	data := &EntryData{
		Username:   e.Username,
		Password:   e.Password,
		TOTPSecret: e.Password, // TOTP: password field holds the secret
		Notes:      e.Notes,
		Issuer:     e.Issuer,
		Period:     e.Period,
		Digits:     e.Digits,
		Algorithm:  e.Algorithm,
	}
	entryType := string(e.SecretType)
	if entryType == "" {
		entryType = "password"
	}
	// For TOTP entries, TOTPSecret holds the secret; Password holds it too.
	if e.SecretType == keyring.SecretTypeTOTP {
		data.TOTPSecret = e.Password
		data.Password = ""
	}
	_, err := a.v.CreateEntry(e.Service, entryType, "", data, nil, "", "")
	return err
}

func (a *KeyringAdapter) Get(service, username string) (string, error) {
	if !a.v.IsUnlocked() {
		return "", fmt.Errorf("vault is locked — master password required")
	}
	entries, err := a.v.SearchEntries(service)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Name != service {
			continue
		}
		full, err := a.v.GetEntry(e.ID)
		if err != nil {
			return "", err
		}
		if full.Data == nil {
			continue
		}
		if username == "" || full.Data.Username == username {
			return full.Data.Password, nil
		}
	}
	return "", fmt.Errorf("no entry found for %s", service)
}

func (a *KeyringAdapter) GetEntry(service, username string) (*keyring.Entry, error) {
	if !a.v.IsUnlocked() {
		return nil, fmt.Errorf("vault is locked — master password required")
	}
	entries, err := a.v.SearchEntries(service)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Name != service {
			continue
		}
		full, err := a.v.GetEntry(e.ID)
		if err != nil {
			return nil, err
		}
		if full.Data == nil {
			continue
		}
		if username != "" && full.Data.Username != username {
			continue
		}
		secret := full.Data.Password
		if full.Type == "totp" {
			secret = full.Data.TOTPSecret
		}
		return &keyring.Entry{
			Service:    full.Name,
			Username:   full.Data.Username,
			Password:   secret,
			SecretType: keyring.SecretType(full.Type),
			Issuer:     full.Data.Issuer,
			Period:     full.Data.Period,
			Digits:     full.Data.Digits,
			Algorithm:  full.Data.Algorithm,
			Notes:      full.Data.Notes,
		}, nil
	}
	return nil, fmt.Errorf("no entry found for %s", service)
}

func (a *KeyringAdapter) Delete(service, username string) error {
	entries, err := a.v.SearchEntries(service)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name == service {
			return a.v.DeleteEntry(e.ID)
		}
	}
	return fmt.Errorf("no entry found for %s", service)
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

// BackupData is the legacy JSON backup format (used by the old pkg/backup package).
// Kept for compatibility with the TUI's backup screen.
type BackupData struct {
	Passwords []keyring.Entry    `json:"passwords"`
	Metadata  BackupDataMetadata `json:"metadata"`
}

type BackupDataMetadata struct {
	BackupDate    string `json:"backup_date"`
	PasswordCount int    `json:"password_count"`
}


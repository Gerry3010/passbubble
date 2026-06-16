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

// Package keyring provides a compatibility shim so the TUI can interact with
// the vault through the same interface as the old GNOME Keyring backend.
// Call SetGlobal to inject the active vault adapter before starting the TUI.
package keyring

// SecretType represents the type of secret stored
type SecretType string

const (
	SecretTypePassword    SecretType = "password"
	SecretTypeTOTP        SecretType = "totp"
	SecretTypeNote        SecretType = "note"
	SecretTypeAPIKey      SecretType = "api-key"
	SecretTypeSSHKey      SecretType = "ssh-key"
	SecretTypeCert        SecretType = "certificate"
)

// Entry represents a secret entry
type Entry struct {
	Service    string     `json:"service"`
	Username   string     `json:"username,omitempty"`
	Password   string     `json:"password"`
	Label      string     `json:"label,omitempty"`
	SecretType SecretType `json:"secret_type,omitempty"`
	Issuer     string     `json:"issuer,omitempty"`
	Period     int        `json:"period,omitempty"`
	Digits     int        `json:"digits,omitempty"`
	Algorithm  string     `json:"algorithm,omitempty"`
	CreatedAt  string     `json:"created_at,omitempty"`
	UpdatedAt  string     `json:"updated_at,omitempty"`
	Notes      string     `json:"notes,omitempty"`
}

// KeyringInterface defines the operations available on a secret store.
type KeyringInterface interface {
	Store(service, username, password string) error
	StoreEntry(entry Entry) error
	Get(service, username string) (string, error)
	GetEntry(service, username string) (*Entry, error)
	Delete(service, username string) error
	List() ([]Entry, error)
	Search(pattern string) ([]Entry, error)
	IsAvailable() bool
}

var global KeyringInterface

// SetGlobal registers the active implementation (called from CLI before TUI starts).
func SetGlobal(k KeyringInterface) {
	global = k
}

// New returns the active global implementation.
// Panics if SetGlobal was not called first.
func New() KeyringInterface {
	if global == nil {
		return &noopKeyring{}
	}
	return global
}

// noopKeyring is returned when no vault is configured yet.
type noopKeyring struct{}

func (n *noopKeyring) Store(_, _, _ string) error         { return errNotConfigured }
func (n *noopKeyring) StoreEntry(_ Entry) error           { return errNotConfigured }
func (n *noopKeyring) Get(_, _ string) (string, error)    { return "", errNotConfigured }
func (n *noopKeyring) GetEntry(_, _ string) (*Entry, error) { return nil, errNotConfigured }
func (n *noopKeyring) Delete(_, _ string) error            { return errNotConfigured }
func (n *noopKeyring) List() ([]Entry, error)              { return nil, errNotConfigured }
func (n *noopKeyring) Search(_ string) ([]Entry, error)    { return nil, errNotConfigured }
func (n *noopKeyring) IsAvailable() bool                   { return false }

var errNotConfigured = &Error{"not connected to server — run 'pwmgr login' first"}

type Error struct{ msg string }

func (e *Error) Error() string { return e.msg }

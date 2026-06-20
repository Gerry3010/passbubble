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

// Package vault provides high-level vault operations over the Passbubble REST API.
// It handles token refresh, client-side E2E decryption, and key management.
package vault

import (
	"encoding/json"
	"fmt"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
)

// CustomField is a user-defined key/value pair available on any entry type.
type CustomField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// EntryData is the plaintext JSON stored inside encrypted_data.
// All fields are optional; only those relevant to the entry type are populated.
type EntryData struct {
	// ── password / api-key / ssh-key / totp ─────────────────────────────────
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	TOTPSecret string `json:"totp_secret,omitempty"`
	Notes      string `json:"notes,omitempty"`

	// TOTP metadata
	Issuer    string `json:"issuer,omitempty"`
	Period    int    `json:"period,omitempty"`
	Digits    int    `json:"digits,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`

	// ── credit-card ──────────────────────────────────────────────────────────
	CardNumber  string `json:"card_number,omitempty"`
	HolderName  string `json:"holder_name,omitempty"`
	ExpiryMonth string `json:"expiry_month,omitempty"`
	ExpiryYear  string `json:"expiry_year,omitempty"`
	CVV         string `json:"cvv,omitempty"`

	// ── bank-account ─────────────────────────────────────────────────────────
	BankName      string `json:"bank_name,omitempty"`
	IBAN          string `json:"iban,omitempty"`
	BIC           string `json:"bic,omitempty"`
	AccountNumber string `json:"account_number,omitempty"`
	AccountType   string `json:"account_type,omitempty"` // "checking" | "savings"

	// ── identity ─────────────────────────────────────────────────────────────
	Title      string `json:"title,omitempty"`
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	Company    string `json:"company,omitempty"`
	Email      string `json:"email,omitempty"`
	Phone      string `json:"phone,omitempty"`
	Street     string `json:"street,omitempty"`
	City       string `json:"city,omitempty"`
	State      string `json:"state,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	Country    string `json:"country,omitempty"`

	// ── license ──────────────────────────────────────────────────────────────
	ProductName   string `json:"product_name,omitempty"`
	LicenseKey    string `json:"license_key,omitempty"`
	PurchaseEmail string `json:"purchase_email,omitempty"`
	PurchaseDate  string `json:"purchase_date,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`

	// ── universal ────────────────────────────────────────────────────────────
	CustomFields []CustomField `json:"custom_fields,omitempty"`
}

// Entry is a vault entry with decrypted fields.
type Entry struct {
	ID         string
	Name       string
	URL        string
	Type       string
	FolderID   *string
	Permission string
	CreatedAt  string     // RFC3339 timestamp (metadata, no decryption needed)
	UpdatedAt  string     // RFC3339 timestamp (metadata, no decryption needed)
	Data       *EntryData // nil until Unlock + fetch
}

// Folder is a vault folder node. Children is populated for the tree returned by ListFolders.
type Folder struct {
	ID        string
	Name      string
	ParentID  *string
	Children  []*Folder
	CreatedAt string
}

// Vault manages a user session against the API.
type Vault struct {
	cfg     *config.Config
	client  *apiclient.Client
	cfgPath string

	// Unlocked state (in-process memory, never persisted)
	privX25519  []byte
	privMLKEM   []byte
	accessToken string
}

// New creates a Vault from persisted config.
// Call Authenticate() to obtain a valid access token, then Unlock() for crypto ops.
func New(cfg *config.Config, cfgPath string) *Vault {
	c := apiclient.New(cfg.ServerURL)
	return &Vault{cfg: cfg, client: c, cfgPath: cfgPath}
}

// Authenticate refreshes the access token using the stored refresh token.
func (v *Vault) Authenticate() error {
	if !v.cfg.IsLoggedIn() {
		return fmt.Errorf("not logged in — run 'pwmgr login' first")
	}
	resp, err := v.client.Refresh(v.cfg.RefreshToken)
	if err != nil {
		return fmt.Errorf("refresh token: %w", err)
	}
	v.accessToken = resp.AccessToken
	v.cfg.RefreshToken = resp.RefreshToken
	v.client.SetToken(v.accessToken)
	// Persist the rotated refresh token
	_ = v.cfg.Save(v.cfgPath)
	return nil
}

// Unlock decrypts the stored private keys using the master password.
// Must be called before any entry read/write operations.
func (v *Vault) Unlock(masterPassword string) error {
	saltBytes, err := crypto.B64Dec(v.cfg.KDFSalt)
	if err != nil {
		return fmt.Errorf("decode kdf salt: %w", err)
	}
	kdfTime := uint32(3)
	kdfMem := uint32(64 * 1024)
	if v.cfg.KDFTime > 0 {
		kdfTime = uint32(v.cfg.KDFTime)
	}
	if v.cfg.KDFMemory > 0 {
		kdfMem = uint32(v.cfg.KDFMemory)
	}

	masterKey := crypto.DeriveKey(masterPassword, &crypto.KDFParams{
		Salt:   saltBytes,
		Time:   kdfTime,
		Memory: kdfMem,
	})

	encPrivX25519, err := crypto.B64Dec(v.cfg.EncPrivX25519)
	if err != nil {
		return fmt.Errorf("decode enc_priv_x25519: %w", err)
	}
	privX25519, err := crypto.Decrypt(masterKey, encPrivX25519)
	if err != nil {
		return fmt.Errorf("wrong master password")
	}

	encPrivMLKEM, err := crypto.B64Dec(v.cfg.EncPrivMLKEM768)
	if err != nil {
		return fmt.Errorf("decode enc_priv_mlkem768: %w", err)
	}
	privMLKEM, err := crypto.Decrypt(masterKey, encPrivMLKEM)
	if err != nil {
		return fmt.Errorf("wrong master password")
	}

	v.privX25519 = privX25519
	v.privMLKEM = privMLKEM
	return nil
}

// IsUnlocked reports whether private keys are available in memory.
func (v *Vault) IsUnlocked() bool {
	return v.privX25519 != nil
}

// Lock clears the decrypted private keys from memory. After Lock, entry
// read/write operations fail until Unlock is called again.
func (v *Vault) Lock() {
	for i := range v.privX25519 {
		v.privX25519[i] = 0
	}
	for i := range v.privMLKEM {
		v.privMLKEM[i] = 0
	}
	v.privX25519 = nil
	v.privMLKEM = nil
}

// Client returns the underlying API client (for advanced use by CLI commands).
func (v *Vault) Client() *apiclient.Client {
	return v.client
}

// Config returns the persisted config.
func (v *Vault) Config() *config.Config {
	return v.cfg
}

// ListEntries returns all entries (metadata only, no decryption needed).
func (v *Vault) ListEntries() ([]Entry, error) {
	apiEntries, err := v.client.ListEntries()
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(apiEntries))
	for i, e := range apiEntries {
		entries[i] = Entry{
			ID:         e.ID,
			Name:       e.Name,
			URL:        e.URL,
			Type:       e.Type,
			FolderID:   e.FolderID,
			Permission: e.Permission,
			CreatedAt:  e.CreatedAt,
			UpdatedAt:  e.UpdatedAt,
		}
	}
	return entries, nil
}

// GetEntry fetches and decrypts a single entry.
func (v *Vault) GetEntry(id string) (*Entry, error) {
	if !v.IsUnlocked() {
		return nil, fmt.Errorf("vault is locked")
	}
	apiEntry, err := v.client.GetEntry(id)
	if err != nil {
		return nil, err
	}
	data, err := v.decryptEntry(apiEntry)
	if err != nil {
		return nil, fmt.Errorf("decrypt entry: %w", err)
	}
	return &Entry{
		ID:         apiEntry.ID,
		Name:       apiEntry.Name,
		URL:        apiEntry.URL,
		Type:       apiEntry.Type,
		FolderID:   apiEntry.FolderID,
		Permission: apiEntry.Permission,
		CreatedAt:  apiEntry.CreatedAt,
		UpdatedAt:  apiEntry.UpdatedAt,
		Data:       data,
	}, nil
}

// CreateEntry encrypts and uploads a new entry. createdAt/updatedAt are optional
// RFC3339 timestamps (used by import to preserve source dates; "" → server NOW()).
func (v *Vault) CreateEntry(name, entryType, url string, data *EntryData, folderID *string, createdAt, updatedAt string, matchPatterns []string) (*Entry, error) {
	if !v.IsUnlocked() {
		return nil, fmt.Errorf("vault is locked")
	}

	// Generate random data key
	dataKey, err := crypto.RandKey()
	if err != nil {
		return nil, err
	}

	// Encrypt entry data
	plaintext, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	ciphertext, err := crypto.Encrypt(dataKey, plaintext)
	if err != nil {
		return nil, err
	}

	// Split ciphertext into nonce + encrypted_data (backend stores them separately)
	// Encrypt() prepends nonce (12 bytes for AES-GCM), so:
	// We'll store the full nonce||ciphertext in encrypted_data, and data_nonce is empty
	// to keep it simple (backend accepts base64 blobs).
	// Actually the backend schema has separate data_nonce. Let's keep nonce embedded:
	// encrypted_data = full blob, data_nonce = first 12 bytes (0s placeholder)
	placeholder := make([]byte, 12) // server stores but doesn't use it
	encDataB64 := crypto.B64Enc(ciphertext)
	dataNonceB64 := crypto.B64Enc(placeholder)

	// Encrypt data key for ourselves
	pubX25519, err := crypto.B64Dec(v.cfg.PubX25519)
	if err != nil {
		return nil, fmt.Errorf("decode pub_x25519: %w", err)
	}
	pubMLKEM, err := crypto.B64Dec(v.cfg.PubMLKEM768)
	if err != nil {
		return nil, fmt.Errorf("decode pub_mlkem768: %w", err)
	}
	encKey, err := crypto.EncryptDataKey(dataKey, pubX25519, pubMLKEM)
	if err != nil {
		return nil, fmt.Errorf("encrypt data key: %w", err)
	}

	req := apiclient.CreateEntryRequest{
		FolderID:      folderID,
		Type:          entryType,
		Name:          name,
		URL:           url,
		MatchPatterns: matchPatterns,
		EncryptedData: encDataB64,
		DataNonce:     dataNonceB64,
		EntryKeys: []apiclient.EntryKey{
			{UserID: v.cfg.UserID, EncryptedKey: crypto.B64Enc(encKey)},
		},
		CreatedAt: optTime(createdAt),
		UpdatedAt: optTime(updatedAt),
	}

	apiEntry, err := v.client.CreateEntry(req)
	if err != nil {
		return nil, err
	}
	return &Entry{
		ID:         apiEntry.ID,
		Name:       apiEntry.Name,
		URL:        apiEntry.URL,
		Type:       apiEntry.Type,
		Permission: "owner",
		Data:       data,
	}, nil
}

// UpdateEntry re-encrypts and updates an existing entry. matchPatterns nil keeps
// the existing patterns; a non-nil (possibly empty) slice replaces them.
func (v *Vault) UpdateEntry(id, name, url string, data *EntryData, matchPatterns []string) error {
	if !v.IsUnlocked() {
		return fmt.Errorf("vault is locked")
	}

	// Fetch current entry to get its encrypted key
	apiEntry, err := v.client.GetEntry(id)
	if err != nil {
		return err
	}
	if apiEntry.EntryKey == nil {
		return fmt.Errorf("no entry key for current user")
	}

	// Decrypt the existing data key
	encKey, err := crypto.B64Dec(apiEntry.EntryKey.EncryptedKey)
	if err != nil {
		return fmt.Errorf("decode entry key: %w", err)
	}
	dataKey, err := crypto.DecryptDataKey(encKey, v.privX25519, v.privMLKEM)
	if err != nil {
		return fmt.Errorf("decrypt data key: %w", err)
	}

	// Re-encrypt entry data with same data key
	plaintext, err := json.Marshal(data)
	if err != nil {
		return err
	}
	ciphertext, err := crypto.Encrypt(dataKey, plaintext)
	if err != nil {
		return err
	}
	placeholder := make([]byte, 12)

	req := apiclient.UpdateEntryRequest{
		// Preserve the current folder: the backend always overwrites folder_id,
		// so we must echo it back or the entry would silently move to the root.
		FolderID:      apiEntry.FolderID,
		Name:          name,
		URL:           url,
		MatchPatterns: matchPatterns,
		EncryptedData: crypto.B64Enc(ciphertext),
		DataNonce:     crypto.B64Enc(placeholder),
	}
	_, err = v.client.UpdateEntry(id, req)
	return err
}

// MoveEntry assigns an entry to a folder (nil = move to root). Metadata-only,
// no decryption required: name/url/data are preserved server-side.
func (v *Vault) MoveEntry(id string, folderID *string) error {
	_, err := v.client.UpdateEntry(id, apiclient.UpdateEntryRequest{FolderID: folderID})
	return err
}

// optTime returns a pointer to s, or nil when s is empty (omits the JSON field).
func optTime(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// DeleteEntry removes an entry.
func (v *Vault) DeleteEntry(id string) error {
	return v.client.DeleteEntry(id)
}

// SearchEntries searches entries by name/URL.
func (v *Vault) SearchEntries(query string) ([]Entry, error) {
	apiEntries, err := v.client.SearchEntries(query)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(apiEntries))
	for i, e := range apiEntries {
		entries[i] = Entry{
			ID:   e.ID,
			Name: e.Name,
			URL:  e.URL,
			Type: e.Type,
		}
	}
	return entries, nil
}

// --- Folders ---

// ListFolders returns the user's folder tree (roots with nested Children).
func (v *Vault) ListFolders() ([]*Folder, error) {
	apiFolders, err := v.client.ListFolders()
	if err != nil {
		return nil, err
	}
	roots := make([]*Folder, len(apiFolders))
	for i := range apiFolders {
		roots[i] = convertFolder(&apiFolders[i])
	}
	return roots, nil
}

// CreateFolder creates a folder under parentID (nil = root) and returns its ID.
func (v *Vault) CreateFolder(name string, parentID *string) (string, error) {
	return v.client.CreateFolder(apiclient.CreateFolderRequest{Name: name, ParentID: parentID})
}

// RenameFolder updates a folder's name and/or parent.
func (v *Vault) RenameFolder(id, name string, parentID *string) error {
	return v.client.UpdateFolder(id, apiclient.CreateFolderRequest{Name: name, ParentID: parentID})
}

// DeleteFolder removes a folder.
func (v *Vault) DeleteFolder(id string) error {
	return v.client.DeleteFolder(id)
}

// convertFolder maps an apiclient.FolderResponse tree node to a vault.Folder.
func convertFolder(f *apiclient.FolderResponse) *Folder {
	out := &Folder{
		ID:        f.ID,
		Name:      f.Name,
		ParentID:  f.ParentID,
		CreatedAt: f.CreatedAt,
	}
	for _, c := range f.Children {
		out.Children = append(out.Children, convertFolder(c))
	}
	return out
}

func (v *Vault) decryptEntry(e *apiclient.EntryResponse) (*EntryData, error) {
	if e.EntryKey == nil {
		return nil, fmt.Errorf("no entry key returned")
	}
	encKey, err := crypto.B64Dec(e.EntryKey.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("decode entry key: %w", err)
	}
	dataKey, err := crypto.DecryptDataKey(encKey, v.privX25519, v.privMLKEM)
	if err != nil {
		return nil, err
	}
	ciphertext, err := crypto.B64Dec(e.EncryptedData)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted data: %w", err)
	}
	plaintext, err := crypto.Decrypt(dataKey, ciphertext)
	if err != nil {
		return nil, err
	}
	var data EntryData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("unmarshal entry data: %w", err)
	}
	return &data, nil
}

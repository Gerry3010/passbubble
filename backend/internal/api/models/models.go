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

package models

// ─── Auth ──────────────────────────────────────────────────────────────────

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds
	TokenType    string `json:"token_type"` // "Bearer"
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type RegisterRequest struct {
	Email           string `json:"email"`
	Name            string `json:"name"`
	Password        string `json:"password"`
	InvitationToken string `json:"invitation_token"`
	// Public keys (base64-encoded)
	PubX25519   string `json:"pub_x25519"`
	PubMLKEM768 string `json:"pub_mlkem768"`
	// Encrypted private keys (base64-encoded, encrypted with master key)
	EncPrivX25519   string `json:"enc_priv_x25519"`
	EncPrivMLKEM768 string `json:"enc_priv_mlkem768"`
	// KDF parameters
	KDFSalt string `json:"kdf_salt"` // base64
}

// ─── Users ─────────────────────────────────────────────────────────────────

type UserResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type UserPublicKeys struct {
	UserID      string `json:"user_id"`
	PubX25519   string `json:"pub_x25519"`
	PubMLKEM768 string `json:"pub_mlkem768"`
}

type UpdateUserRequest struct {
	Status string `json:"status,omitempty"` // "active", "disabled"
	Role   string `json:"role,omitempty"`   // "admin", "user"
}

// ─── Invitations ───────────────────────────────────────────────────────────

type InviteRequest struct {
	Email string `json:"email"`
}

type InvitationResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	Used      bool   `json:"used"`
}

// ─── Folders ───────────────────────────────────────────────────────────────

type CreateFolderRequest struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id,omitempty"`
}

type FolderResponse struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	ParentID  *string           `json:"parent_id,omitempty"`
	Children  []*FolderResponse `json:"children,omitempty"`
	CreatedAt string            `json:"created_at"`
}

type ShareFolderRequest struct {
	UserID     string `json:"user_id"`
	Permission string `json:"permission"` // "read", "write"
}

// ─── Entries ───────────────────────────────────────────────────────────────

type EntryKey struct {
	UserID       string `json:"user_id"`
	EncryptedKey string `json:"encrypted_key"` // base64
}

type CreateEntryRequest struct {
	FolderID      *string    `json:"folder_id,omitempty"`
	Type          string     `json:"type"` // "password","totp","note","api-key","ssh-key"
	Name          string     `json:"name"`
	URL           string     `json:"url,omitempty"`
	EncryptedData string     `json:"encrypted_data"` // base64: AES-256-GCM encrypted JSON payload
	DataNonce     string     `json:"data_nonce"`     // base64: 12-byte GCM nonce
	EntryKeys     []EntryKey `json:"entry_keys"`     // one per authorized user
	// Optional original timestamps (used by import to preserve source dates).
	// Omitted/empty → server uses NOW().
	CreatedAt *string `json:"created_at,omitempty"`
	UpdatedAt *string `json:"updated_at,omitempty"`
}

type UpdateEntryRequest struct {
	FolderID      *string    `json:"folder_id,omitempty"`
	Name          string     `json:"name,omitempty"`
	URL           string     `json:"url,omitempty"`
	EncryptedData string     `json:"encrypted_data,omitempty"`
	DataNonce     string     `json:"data_nonce,omitempty"`
	EntryKeys     []EntryKey `json:"entry_keys,omitempty"`
}

type EntryResponse struct {
	ID            string     `json:"id"`
	FolderID      *string    `json:"folder_id,omitempty"`
	OwnerID       string     `json:"owner_id"`
	Type          string     `json:"type"`
	Name          string     `json:"name"`
	URL           string     `json:"url,omitempty"`
	EncryptedData string     `json:"encrypted_data,omitempty"` // only on single GET
	DataNonce     string     `json:"data_nonce,omitempty"`
	EntryKey      *EntryKey  `json:"entry_key,omitempty"` // caller's key on single GET
	CreatedAt     string     `json:"created_at"`
	UpdatedAt     string     `json:"updated_at"`
}

type ShareEntryRequest struct {
	UserID       string `json:"user_id"`
	Permission   string `json:"permission"`    // "read", "write"
	EncryptedKey string `json:"encrypted_key"` // base64: data_key encrypted with recipient's pub key
}

// ─── Generate ──────────────────────────────────────────────────────────────

type GenerateRequest struct {
	Length      int    `json:"length,omitempty"`
	Type        string `json:"type,omitempty"`
	Count       int    `json:"count,omitempty"`
	NoAmbiguous bool   `json:"no_ambiguous,omitempty"`
	ExcludeChars string `json:"exclude_chars,omitempty"`
}

type GeneratedPassword struct {
	Password string `json:"password"`
	Strength int    `json:"strength"` // 0-100
}

type GenerateResponse struct {
	Passwords []GeneratedPassword `json:"passwords"`
}

// ─── Backup ────────────────────────────────────────────────────────────────

type BackupRequest struct {
	Encrypt bool `json:"encrypt,omitempty"`
}

type BackupInfo struct {
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

type RestoreRequest struct {
	Filename string `json:"filename"`
}

// ─── Errors ────────────────────────────────────────────────────────────────

type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

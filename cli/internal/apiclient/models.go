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

package apiclient

// --- Auth ---

type RegisterRequest struct {
	Email           string `json:"email"`
	Name            string `json:"name"`
	Password        string `json:"password"`
	InvitationToken string `json:"invitation_token,omitempty"`
	PubX25519       string `json:"pub_x25519"`
	PubMLKEM768     string `json:"pub_mlkem768"`
	EncPrivX25519   string `json:"enc_priv_x25519"`
	EncPrivMLKEM768 string `json:"enc_priv_mlkem768"`
	KDFSalt         string `json:"kdf_salt"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	ExpiresIn       int    `json:"expires_in"`
	UserID          string `json:"user_id"`
	Role            string `json:"role"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	PubX25519       string `json:"pub_x25519"`
	PubMLKEM768     string `json:"pub_mlkem768"`
	EncPrivX25519   string `json:"enc_priv_x25519"`
	EncPrivMLKEM768 string `json:"enc_priv_mlkem768"`
	KDFSalt         string `json:"kdf_salt"`
	KDFTime         int    `json:"kdf_time"`
	KDFMemory       int    `json:"kdf_memory"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// --- Users ---

type UserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

type UserPublicKeys struct {
	UserID      string `json:"user_id"`
	PubX25519   string `json:"pub_x25519"`
	PubMLKEM768 string `json:"pub_mlkem768"`
}

// --- Folders ---

type FolderResponse struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	ParentID *string           `json:"parent_id"`
	Children []*FolderResponse `json:"children,omitempty"`
}

// --- Entries ---

type EntryKey struct {
	UserID       string `json:"user_id"`
	EncryptedKey string `json:"encrypted_key"` // base64
}

type CreateEntryRequest struct {
	FolderID      *string    `json:"folder_id,omitempty"`
	Type          string     `json:"type"`
	Name          string     `json:"name"`
	URL           string     `json:"url,omitempty"`
	EncryptedData string     `json:"encrypted_data"` // base64
	DataNonce     string     `json:"data_nonce"`     // base64
	EntryKeys     []EntryKey `json:"entry_keys"`
}

type UpdateEntryRequest struct {
	FolderID      *string `json:"folder_id,omitempty"`
	Name          string  `json:"name,omitempty"`
	URL           string  `json:"url,omitempty"`
	EncryptedData string  `json:"encrypted_data,omitempty"` // base64
	DataNonce     string  `json:"data_nonce,omitempty"`     // base64
	EncryptedKey  string  `json:"encrypted_key,omitempty"`  // base64, caller's new key
}

type EntryResponse struct {
	ID            string    `json:"id"`
	FolderID      *string   `json:"folder_id"`
	Type          string    `json:"type"`
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	EncryptedData string    `json:"encrypted_data"` // base64
	DataNonce     string    `json:"data_nonce"`     // base64
	EntryKey      *EntryKey `json:"entry_key"`      // caller's key, set on single GET
	Permission    string    `json:"permission"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}

type ShareEntryRequest struct {
	UserID       string `json:"user_id"`
	Permission   string `json:"permission"` // "read" or "write"
	EncryptedKey string `json:"encrypted_key"` // base64
}

// --- Generate ---

type GenerateRequest struct {
	Length      int    `json:"length"`
	Type        string `json:"type"`
	Count       int    `json:"count"`
	NoAmbiguous bool   `json:"no_ambiguous"`
	ExcludeChars string `json:"exclude_chars,omitempty"`
}

type GeneratedPassword struct {
	Password string `json:"password"`
	Strength int    `json:"strength"`
}

type GenerateResponse struct {
	Passwords []GeneratedPassword `json:"passwords"`
}

// --- Errors ---

type ErrorResponse struct {
	Error string `json:"error"`
}

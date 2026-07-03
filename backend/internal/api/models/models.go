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
	TOTPEnabled bool   `json:"totp_enabled"`
}

type UserPublicKeys struct {
	UserID      string `json:"user_id"`
	PubX25519   string `json:"pub_x25519"`
	PubMLKEM768 string `json:"pub_mlkem768"`
}

// UpdateKeysRequest rotates the caller's own key material — used to retrofit a
// real ML-KEM-768 keypair onto an X25519-only account (post-quantum upgrade).
// All four fields are base64-encoded; private keys are master-key-encrypted.
type UpdateKeysRequest struct {
	PubX25519       string `json:"pub_x25519"`
	PubMLKEM768     string `json:"pub_mlkem768"`
	EncPrivX25519   string `json:"enc_priv_x25519"`
	EncPrivMLKEM768 string `json:"enc_priv_mlkem768"`
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
	MatchPatterns []string   `json:"match_patterns,omitempty"` // plaintext autofill URL patterns
	EncryptedData string     `json:"encrypted_data"`           // base64: AES-256-GCM encrypted JSON payload
	DataNonce     string     `json:"data_nonce"`               // base64: 12-byte GCM nonce
	EntryKeys     []EntryKey `json:"entry_keys"`               // one per authorized user
	// Optional original timestamps (used by import to preserve source dates).
	// Omitted/empty → server uses NOW().
	CreatedAt *string `json:"created_at,omitempty"`
	UpdatedAt *string `json:"updated_at,omitempty"`
}

type UpdateEntryRequest struct {
	FolderID      *string    `json:"folder_id,omitempty"`
	Name          string     `json:"name,omitempty"`
	URL           string     `json:"url,omitempty"`
	MatchPatterns []string   `json:"match_patterns,omitempty"` // nil = keep existing; [] = clear
	EncryptedData string     `json:"encrypted_data,omitempty"`
	DataNonce     string     `json:"data_nonce,omitempty"`
	EntryKeys     []EntryKey `json:"entry_keys,omitempty"`
}

type EntryResponse struct {
	ID            string    `json:"id"`
	FolderID      *string   `json:"folder_id,omitempty"`
	OwnerID       string    `json:"owner_id"`
	Type          string    `json:"type"`
	Name          string    `json:"name"`
	URL           string    `json:"url,omitempty"`
	MatchPatterns []string  `json:"match_patterns,omitempty"` // plaintext autofill URL patterns
	Favorite      bool      `json:"favorite,omitempty"`
	EncryptedData string    `json:"encrypted_data,omitempty"` // only on single GET
	DataNonce     string    `json:"data_nonce,omitempty"`
	EntryKey      *EntryKey `json:"entry_key,omitempty"` // caller's key on single GET
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
	DeletedAt     *string   `json:"deleted_at,omitempty"` // set only in trash listings
}

type SetFavoriteRequest struct {
	Favorite bool `json:"favorite"`
}

// EntryVersionResponse is one history snapshot of an entry. The full variant
// (single GET) carries the blob plus the caller's contemporaneous wrapped key
// in the same shape as EntryResponse, so clients can reuse their decrypt path.
type EntryVersionResponse struct {
	ID            string    `json:"id"`
	EntryID       string    `json:"entry_id"`
	Name          string    `json:"name"`
	URL           string    `json:"url,omitempty"`
	EditedBy      *string   `json:"edited_by,omitempty"`
	EncryptedData string    `json:"encrypted_data,omitempty"` // only on single GET
	DataNonce     string    `json:"data_nonce,omitempty"`
	EntryKey      *EntryKey `json:"entry_key,omitempty"` // caller's key on single GET
	CreatedAt     string    `json:"created_at"`
}

type ShareEntryRequest struct {
	UserID       string `json:"user_id"`
	Permission   string `json:"permission"`    // "read", "write"
	EncryptedKey string `json:"encrypted_key"` // base64: data_key encrypted with recipient's pub key
}

// ─── Share Links ───────────────────────────────────────────────────────────

type CreateShareLinkRequest struct {
	EncryptedPayload string `json:"encrypted_payload"` // base64: AES-256-GCM ciphertext encrypted with the link key (key never sent to server)
	PayloadNonce     string `json:"payload_nonce"`     // base64: 12-byte GCM nonce
	ExpiresAt        string `json:"expires_at"`        // RFC3339
	MaxViews         *int   `json:"max_views,omitempty"`
	Password         string `json:"password,omitempty"` // optional extra layer, hashed server-side, never stored in plaintext
}

type ShareLinkResponse struct {
	ID           string  `json:"id"`
	Token        string  `json:"token"`
	EntryID      *string `json:"entry_id,omitempty"`
	FolderID     *string `json:"folder_id,omitempty"`
	ResourceName string  `json:"resource_name"` // entry or folder name (for display)
	HasPassword  bool    `json:"has_password"`
	MaxViews     *int    `json:"max_views,omitempty"`
	ViewCount    int     `json:"view_count"`
	ExpiresAt    string  `json:"expires_at"`
	CreatedAt    string  `json:"created_at"`
	RevokedAt    *string `json:"revoked_at,omitempty"`
}

// PublicShareLinkResponse is returned by the unauthenticated GET /share/{token}.
// RequiresPassword=true with no payload means the caller must resubmit with ?password=.
type PublicShareLinkResponse struct {
	RequiresPassword bool   `json:"requires_password"`
	EncryptedPayload string `json:"encrypted_payload,omitempty"`
	PayloadNonce     string `json:"payload_nonce,omitempty"`
}

// ─── Shares aggregation ────────────────────────────────────────────────────

type DirectShareResponse struct {
	ResourceID   string `json:"resource_id"`
	ResourceName string `json:"resource_name"`
	UserID       string `json:"user_id"`
	UserEmail    string `json:"user_email"`
	Permission   string `json:"permission"`
	CreatedAt    string `json:"created_at"`
}

type MySharesResponse struct {
	ShareLinks   []ShareLinkResponse   `json:"share_links"`
	EntryShares  []DirectShareResponse `json:"entry_shares"`
	FolderShares []DirectShareResponse `json:"folder_shares"`
}

// ─── Jobs (import/export progress ledger) ──────────────────────────────────

type CreateJobRequest struct {
	Type        string `json:"type"`         // "import" | "export"
	Format      string `json:"format"`       // "csv-generic","csv-chrome","csv-lastpass","csv-1password","bitwarden","keepass","csv"
	DupStrategy string `json:"dup_strategy"` // "skip" | "overwrite"
	TotalItems  int    `json:"total_items"`
	ClientName  string `json:"client_name,omitempty"`
}

type UpdateJobRequest struct {
	Status         string `json:"status,omitempty"`
	ProcessedItems *int   `json:"processed_items,omitempty"`
	CreatedItems   *int   `json:"created_items,omitempty"`
	UpdatedItems   *int   `json:"updated_items,omitempty"`
	SkippedItems   *int   `json:"skipped_items,omitempty"`
	FailedItems    *int   `json:"failed_items,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

type JobResponse struct {
	ID             string  `json:"id"`
	Type           string  `json:"type"`
	Format         string  `json:"format"`
	Status         string  `json:"status"`
	DupStrategy    string  `json:"dup_strategy"`
	TotalItems     int     `json:"total_items"`
	ProcessedItems int     `json:"processed_items"`
	CreatedItems   int     `json:"created_items"`
	UpdatedItems   int     `json:"updated_items"`
	SkippedItems   int     `json:"skipped_items"`
	FailedItems    int     `json:"failed_items"`
	ErrorMessage   *string `json:"error_message,omitempty"`
	ClientName     *string `json:"client_name,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	FinishedAt     *string `json:"finished_at,omitempty"`
}

// ─── Account 2FA (TOTP) ────────────────────────────────────────────────────

type VerifyTOTPRequest struct {
	PendingToken string `json:"pending_token"`
	Code         string `json:"code"`
}

type ConfirmTOTPRequest struct {
	Secret string `json:"secret"`
	Code   string `json:"code"`
}

type DisableTOTPRequest struct {
	Code     string `json:"code,omitempty"`
	Password string `json:"password,omitempty"`
}

type RequestTOTPRecoveryRequest struct {
	PendingToken string `json:"pending_token"`
}

type SetupTOTPResponse struct {
	Secret     string `json:"secret"`      // base32, shown to user / encoded in QR
	OTPAuthURL string `json:"otpauth_url"` // otpauth:// URI for authenticator apps
}

// TwoFARequiredResponse is returned by Login (HTTP 202) when the account has
// 2FA enabled: the client must call /auth/verify-totp with the pending token.
type TwoFARequiredResponse struct {
	Status       string `json:"status"` // always "2fa_required"
	PendingToken string `json:"pending_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// ─── Generate ──────────────────────────────────────────────────────────────

type GenerateRequest struct {
	Length       int    `json:"length,omitempty"`
	Type         string `json:"type,omitempty"`
	Count        int    `json:"count,omitempty"`
	NoAmbiguous  bool   `json:"no_ambiguous,omitempty"`
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

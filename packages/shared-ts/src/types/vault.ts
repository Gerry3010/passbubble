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

export interface KDFParams {
  salt: Uint8Array;
  time: number;
  memory: number;
}

// Plaintext payload stored inside an encrypted entry.
// Mirrors EntryData in cli/internal/vault/vault.go exactly.
export interface EntryData {
  // password / api-key / ssh-key / totp
  username?: string;
  password?: string;
  totp_secret?: string;
  notes?: string;
  // "Sign in with <provider>" for sites where the login is an SSO redirect
  // (google | apple | microsoft | github | facebook). Lives in the encrypted
  // data so it syncs across devices like any other entry field.
  sign_in_with?: string;
  // TOTP metadata
  issuer?: string;
  period?: number;
  digits?: number;
  algorithm?: string;
  // credit-card
  card_number?: string;
  holder_name?: string;
  expiry_month?: string;
  expiry_year?: string;
  cvv?: string;
  // bank-account
  bank_name?: string;
  iban?: string;
  bic?: string;
  account_number?: string;
  account_type?: string;
  // identity
  title?: string;
  first_name?: string;
  last_name?: string;
  company?: string;
  email?: string;
  phone?: string;
  street?: string;
  city?: string;
  state?: string;
  postal_code?: string;
  country?: string;
  // license
  product_name?: string;
  license_key?: string;
  purchase_email?: string;
  purchase_date?: string;
  expires_at?: string;
  // server (SSH / remote login). `port` is stored as text; readers accept a
  // number too. `private_key` is kept verbatim (its final newline matters).
  host?: string;
  port?: string;
  private_key?: string;
  key_passphrase?: string;
  // Passkeys of a `password` entry (docs/passkeys.md). Clients that do not
  // handle passkeys keep this key unchanged.
  passkeys?: PasskeyRecord[];
  // universal
  // type as set by the app: 'text' (default), 'password', 'ssh', 'totp', 'file';
  // file fields also carry filename + mime_type. Writers must keep keys they
  // do not know about.
  custom_fields?: Array<{ label: string; value: string; type?: string; filename?: string; mime_type?: string }>;
  // Free-form labels for filtering, normalised on save (see vault/tags.ts).
  // The key is omitted when an entry has no tags.
  tags?: string[];
}

// One stored passkey (docs/passkeys.md, "Storage"). Ids and keys are base64url
// without padding. Unknown keys written by other clients are kept.
export interface PasskeyRecord {
  credential_id: string;
  rp_id: string;
  rp_name?: string;
  user_handle: string;
  user_name: string;
  user_display_name?: string;
  /** COSE algorithm; only -7 (ES256). */
  key_algorithm: number;
  /** PKCS#8 DER of the P-256 private key. */
  private_key: string;
  /** Always 0. */
  sign_count: number;
  discoverable: boolean;
  prf_secret?: string;
  /** RFC 3339, UTC. */
  created_at: string;
  [key: string]: unknown;
}

// In-memory session state held in the background service worker.
// Private keys are NEVER persisted to any storage.
export interface UnlockedSession {
  privX25519: Uint8Array;
  privMLKEM: Uint8Array;
  pubX25519: Uint8Array;
  pubMLKEM: Uint8Array;
  userId: string;
  userEmail: string;
  userName: string;
  role: string;
  accessToken: string;
  refreshToken: string;
  accessTokenExpiresAt: number;
  serverUrl: string;
  // Stored so the popup can re-unlock after SW termination without re-login
  kdfSalt: Uint8Array;
  kdfTime: number;
  kdfMemory: number;
  encPrivX25519: Uint8Array;
  encPrivMLKEM: Uint8Array;
}

export interface SessionInfo {
  isLoggedIn: boolean;
  isUnlocked: boolean;
  userEmail?: string;
  userName?: string;
  serverUrl?: string;
}

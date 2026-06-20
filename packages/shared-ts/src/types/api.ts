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

// Mirrors backend/internal/api/models + cli/internal/apiclient/models exactly.
// All base64 fields use standard encoding (with + / = padding), NOT url-safe.

export interface LoginRequest {
  email: string;
  password: string;
}

export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  token_type: string;
  user_id: string;
  email: string;
  name: string;
  role: string;
  enc_priv_x25519: string;
  enc_priv_mlkem768: string;
  pub_x25519: string;
  pub_mlkem768: string;
  kdf_salt: string;
  kdf_time: number;
  kdf_memory: number;
  // Present only on the intermediate 2FA step (HTTP 202): the password was
  // accepted but the caller must complete /auth/verify-totp with pending_token.
  status?: string;
  pending_token?: string;
}

export interface RefreshResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  token_type: string;
}

export interface MeResponse {
  id: string;
  email: string;
  name: string;
  role: string;
  status: string;
  created_at: string;
}

export interface EntryKey {
  user_id: string;
  encrypted_key: string;
}

export interface EntryResponse {
  id: string;
  folder_id?: string;
  owner_id: string;
  type: string;
  name: string;
  url?: string;
  /** Plaintext autofill URL patterns (host / *.host wildcards). */
  match_patterns?: string[];
  encrypted_data?: string;
  data_nonce?: string;
  entry_key?: EntryKey;
  created_at: string;
  updated_at: string;
}

export interface CreateEntryRequest {
  folder_id?: string;
  type: string;
  name: string;
  url?: string;
  match_patterns?: string[];
  encrypted_data: string;
  data_nonce: string;
  entry_keys: EntryKey[];
}

export interface UpdateEntryRequest {
  folder_id?: string;
  type?: string;
  name?: string;
  url?: string;
  /** nil = keep existing; [] = clear. */
  match_patterns?: string[];
  encrypted_data?: string;
  data_nonce?: string;
  entry_keys?: EntryKey[];
}

export interface FolderResponse {
  id: string;
  name: string;
  parent_id?: string;
  created_at: string;
  children?: FolderResponse[];
}

export interface UserPublicKeys {
  id: string;
  pub_x25519: string;
  pub_mlkem768: string;
}

export interface UserSearchResult {
  id: string;
  email: string;
  name: string;
  role: string;
  status: string;
  created_at: string;
}

export interface GenerateRequest {
  type?: string;
  length?: number;
  count?: number;
  exclude_ambiguous?: boolean;
  include_symbols?: boolean;
  include_numbers?: boolean;
  include_uppercase?: boolean;
  include_lowercase?: boolean;
  words?: number;
  separator?: string;
}

export interface GeneratedPassword {
  password: string;
  strength: number;
}

export interface GenerateResponse {
  passwords: GeneratedPassword[];
}

export interface ApiErrorBody {
  error: string;
}

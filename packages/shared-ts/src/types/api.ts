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
  encrypted_data: string;
  data_nonce: string;
  entry_keys: EntryKey[];
}

export interface UpdateEntryRequest {
  folder_id?: string;
  type?: string;
  name?: string;
  url?: string;
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

export interface ApiError {
  error: string;
}

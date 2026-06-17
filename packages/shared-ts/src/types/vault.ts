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
  // universal
  custom_fields?: Array<{ label: string; value: string }>;
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

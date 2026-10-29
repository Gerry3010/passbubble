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

// Device login state, persisted in chrome.storage.local so a browser restart or
// extension update only locks the vault instead of signing out. Holds the
// refresh token (the ONE copy — every refresh rotates it in place) and the
// already-encrypted session material needed to unlock with the master password
// or PIN. The master password and the decrypted private keys are never stored.

import browser from 'webextension-polyfill';
import { STORAGE_KEYS } from '../shared/constants.js';
import type { LoginResponse } from '@passbubble/shared-ts';

export interface StoredAccount {
  refreshToken: string;
  encPrivX25519: string;
  encPrivMLKEM: string;
  kdfSalt: string;
  kdfTime: number;
  kdfMemory: number;
  userId: string;
  userEmail: string;
  userName: string;
  role: string;
  pubX25519: string;
  pubMLKEM: string;
}

const ACCOUNT_KEYS = [
  STORAGE_KEYS.REFRESH_TOKEN,
  STORAGE_KEYS.ENC_PRIV_X25519,
  STORAGE_KEYS.ENC_PRIV_MLKEM,
  STORAGE_KEYS.KDF_SALT,
  STORAGE_KEYS.KDF_TIME,
  STORAGE_KEYS.KDF_MEMORY,
  STORAGE_KEYS.USER_ID,
  STORAGE_KEYS.USER_EMAIL,
  STORAGE_KEYS.USER_NAME,
  STORAGE_KEYS.ROLE,
  STORAGE_KEYS.PUB_X25519,
  STORAGE_KEYS.PUB_MLKEM,
];

// The PIN-local duplicate of the login state written by older versions.
interface LegacyPinBootstrap {
  refresh_token?: string;
  enc_priv_x25519?: string;
  enc_priv_mlkem?: string;
  kdf_salt?: string;
  kdf_time?: number;
  kdf_memory?: number;
  user_id?: string;
  user_email?: string;
  user_name?: string;
  role?: string;
  pub_x25519?: string;
  pub_mlkem?: string;
}

// A usable login needs a refresh token and the material to derive + decrypt the
// private keys. (The ML-KEM key may be empty for pre-PQ accounts.)
function isComplete(s: Record<string, unknown>): boolean {
  return (
    typeof s[STORAGE_KEYS.REFRESH_TOKEN] === 'string' &&
    !!s[STORAGE_KEYS.REFRESH_TOKEN] &&
    typeof s[STORAGE_KEYS.ENC_PRIV_X25519] === 'string' &&
    !!s[STORAGE_KEYS.ENC_PRIV_X25519] &&
    typeof s[STORAGE_KEYS.KDF_SALT] === 'string' &&
    !!s[STORAGE_KEYS.KDF_SALT]
  );
}

function fromStored(s: Record<string, unknown>): StoredAccount {
  return {
    refreshToken: s[STORAGE_KEYS.REFRESH_TOKEN] as string,
    encPrivX25519: s[STORAGE_KEYS.ENC_PRIV_X25519] as string,
    encPrivMLKEM: (s[STORAGE_KEYS.ENC_PRIV_MLKEM] as string) ?? '',
    kdfSalt: s[STORAGE_KEYS.KDF_SALT] as string,
    kdfTime: s[STORAGE_KEYS.KDF_TIME] as number,
    kdfMemory: s[STORAGE_KEYS.KDF_MEMORY] as number,
    userId: (s[STORAGE_KEYS.USER_ID] as string) ?? '',
    userEmail: (s[STORAGE_KEYS.USER_EMAIL] as string) ?? '',
    userName: (s[STORAGE_KEYS.USER_NAME] as string) ?? '',
    role: (s[STORAGE_KEYS.ROLE] as string) ?? '',
    pubX25519: (s[STORAGE_KEYS.PUB_X25519] as string) ?? '',
    pubMLKEM: (s[STORAGE_KEYS.PUB_MLKEM] as string) ?? '',
  };
}

function fromPinBootstrap(b: LegacyPinBootstrap): Record<string, unknown> {
  return {
    [STORAGE_KEYS.REFRESH_TOKEN]: b.refresh_token,
    [STORAGE_KEYS.ENC_PRIV_X25519]: b.enc_priv_x25519,
    [STORAGE_KEYS.ENC_PRIV_MLKEM]: b.enc_priv_mlkem ?? '',
    [STORAGE_KEYS.KDF_SALT]: b.kdf_salt,
    [STORAGE_KEYS.KDF_TIME]: b.kdf_time,
    [STORAGE_KEYS.KDF_MEMORY]: b.kdf_memory,
    [STORAGE_KEYS.USER_ID]: b.user_id ?? '',
    [STORAGE_KEYS.USER_EMAIL]: b.user_email ?? '',
    [STORAGE_KEYS.USER_NAME]: b.user_name ?? '',
    [STORAGE_KEYS.ROLE]: b.role ?? '',
    [STORAGE_KEYS.PUB_X25519]: b.pub_x25519 ?? '',
    [STORAGE_KEYS.PUB_MLKEM]: b.pub_mlkem ?? '',
  };
}

function pickAccountKeys(s: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const k of ACCOUNT_KEYS) if (s[k] !== undefined) out[k] = s[k];
  return out;
}

/**
 * Moves login state written by older versions into storage.local and removes
 * the old copies. Sources, freshest first: the bootstrap in storage.session
 * (rotated on every refresh, but wiped on browser close / extension update) and
 * the PIN record's duplicate in storage.local. Returns the migrated account, or
 * null when there was nothing to migrate.
 */
export async function migrateLegacyAuthStorage(): Promise<StoredAccount | null> {
  let legacy: Record<string, unknown> | null = null;
  try {
    const s = await browser.storage.session.get(ACCOUNT_KEYS);
    if (isComplete(s)) legacy = pickAccountKeys(s);
  } catch {
    /* storage.session unavailable */
  }
  const pinData = await browser.storage.local.get(STORAGE_KEYS.PIN_BOOTSTRAP);
  const pinBootstrap = pinData[STORAGE_KEYS.PIN_BOOTSTRAP] as LegacyPinBootstrap | undefined;
  if (!legacy && pinBootstrap) {
    const converted = fromPinBootstrap(pinBootstrap);
    if (isComplete(converted)) legacy = converted;
  }
  if (!legacy) return null;

  await browser.storage.local.set(legacy);
  // Single source of truth from here on: drop the duplicates.
  try {
    await browser.storage.session.remove(ACCOUNT_KEYS);
  } catch {
    /* ignore */
  }
  if (pinBootstrap) await browser.storage.local.remove(STORAGE_KEYS.PIN_BOOTSTRAP);
  return fromStored(legacy);
}

/** The device's login state, or null when signed out. Migrates the old storage
 * layout on first read, so existing logins keep working without a re-login. */
export async function loadAccount(): Promise<StoredAccount | null> {
  const s = await browser.storage.local.get(ACCOUNT_KEYS);
  if (isComplete(s)) return fromStored(s);
  return migrateLegacyAuthStorage();
}

/** Persist the login state from a successful login (LOGIN / VERIFY_TOTP). */
export async function saveAccountFromLogin(resp: LoginResponse): Promise<void> {
  await browser.storage.local.set({
    [STORAGE_KEYS.REFRESH_TOKEN]: resp.refresh_token,
    [STORAGE_KEYS.ENC_PRIV_X25519]: resp.enc_priv_x25519,
    [STORAGE_KEYS.ENC_PRIV_MLKEM]: resp.enc_priv_mlkem768,
    [STORAGE_KEYS.KDF_SALT]: resp.kdf_salt,
    [STORAGE_KEYS.KDF_TIME]: resp.kdf_time,
    [STORAGE_KEYS.KDF_MEMORY]: resp.kdf_memory,
    [STORAGE_KEYS.USER_ID]: resp.user_id,
    [STORAGE_KEYS.USER_EMAIL]: resp.email,
    [STORAGE_KEYS.USER_NAME]: resp.name,
    [STORAGE_KEYS.ROLE]: resp.role,
    [STORAGE_KEYS.PUB_X25519]: resp.pub_x25519,
    [STORAGE_KEYS.PUB_MLKEM]: resp.pub_mlkem768,
  });
  // A fresh login supersedes anything an older version may have left behind.
  try {
    await browser.storage.session.remove(ACCOUNT_KEYS);
  } catch {
    /* ignore */
  }
  await browser.storage.local.remove(STORAGE_KEYS.PIN_BOOTSTRAP);
}

/** Store the rotated refresh token. The server deletes the previous token on
 * every refresh, so this must run right after the refresh response arrives. */
export async function saveRefreshToken(token: string): Promise<void> {
  await browser.storage.local.set({ [STORAGE_KEYS.REFRESH_TOKEN]: token });
}

/** Forget the login state (logout / refresh token rejected by the server). */
export async function clearAccount(): Promise<void> {
  await browser.storage.local.remove([...ACCOUNT_KEYS, STORAGE_KEYS.PIN_BOOTSTRAP]);
  try {
    await browser.storage.session.remove(ACCOUNT_KEYS);
  } catch {
    /* ignore */
  }
}

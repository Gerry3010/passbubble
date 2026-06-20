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

// Device-local PIN quick-unlock state, persisted in chrome.storage.local so the
// PIN can unlock after a browser restart (storage.session is wiped on close).
// The PIN itself is never stored — only the master key wrapped under a
// PIN-derived key, plus the already-encrypted session bootstrap.

import browser from 'webextension-polyfill';
import { STORAGE_KEYS } from '../shared/constants.js';

// The (already-encrypted / non-secret) session material needed to rebuild an
// UnlockedSession after a browser restart. Mirrors what persistLoginResponse
// writes to storage.session.
export interface PinBootstrap {
  refresh_token: string;
  enc_priv_x25519: string;
  enc_priv_mlkem: string;
  kdf_salt: string;
  kdf_time: number;
  kdf_memory: number;
  user_id: string;
  user_email: string;
  user_name: string;
  role: string;
  pub_x25519: string;
  pub_mlkem: string;
}

export interface PinRecord {
  enabled: boolean;
  salt: string;
  wrapped: string;
  kdfTime: number;
  kdfMemory: number;
  maxTries: number;
  failCount: number;
  intervalDays: number;
  lastMasterUnlock: number; // ms epoch
  bootstrap?: PinBootstrap;
}

const ALL_PIN_KEYS = [
  STORAGE_KEYS.PIN_ENABLED,
  STORAGE_KEYS.PIN_SALT,
  STORAGE_KEYS.PIN_WRAPPED,
  STORAGE_KEYS.PIN_KDF_TIME,
  STORAGE_KEYS.PIN_KDF_MEMORY,
  STORAGE_KEYS.PIN_MAX_TRIES,
  STORAGE_KEYS.PIN_FAIL_COUNT,
  STORAGE_KEYS.PIN_PW_INTERVAL_DAYS,
  STORAGE_KEYS.PIN_LAST_MASTER_UNLOCK,
  STORAGE_KEYS.PIN_BOOTSTRAP,
];

export async function loadPin(): Promise<PinRecord> {
  const s = await browser.storage.local.get(ALL_PIN_KEYS);
  return {
    enabled: !!s[STORAGE_KEYS.PIN_ENABLED],
    salt: (s[STORAGE_KEYS.PIN_SALT] as string) ?? '',
    wrapped: (s[STORAGE_KEYS.PIN_WRAPPED] as string) ?? '',
    kdfTime: (s[STORAGE_KEYS.PIN_KDF_TIME] as number) ?? 0,
    kdfMemory: (s[STORAGE_KEYS.PIN_KDF_MEMORY] as number) ?? 0,
    maxTries: (s[STORAGE_KEYS.PIN_MAX_TRIES] as number) ?? 0,
    failCount: (s[STORAGE_KEYS.PIN_FAIL_COUNT] as number) ?? 0,
    intervalDays: (s[STORAGE_KEYS.PIN_PW_INTERVAL_DAYS] as number) ?? 0,
    lastMasterUnlock: (s[STORAGE_KEYS.PIN_LAST_MASTER_UNLOCK] as number) ?? 0,
    bootstrap: (s[STORAGE_KEYS.PIN_BOOTSTRAP] as PinBootstrap | undefined) ?? undefined,
  };
}

export async function savePin(rec: PinRecord): Promise<void> {
  await browser.storage.local.set({
    [STORAGE_KEYS.PIN_ENABLED]: rec.enabled,
    [STORAGE_KEYS.PIN_SALT]: rec.salt,
    [STORAGE_KEYS.PIN_WRAPPED]: rec.wrapped,
    [STORAGE_KEYS.PIN_KDF_TIME]: rec.kdfTime,
    [STORAGE_KEYS.PIN_KDF_MEMORY]: rec.kdfMemory,
    [STORAGE_KEYS.PIN_MAX_TRIES]: rec.maxTries,
    [STORAGE_KEYS.PIN_FAIL_COUNT]: rec.failCount,
    [STORAGE_KEYS.PIN_PW_INTERVAL_DAYS]: rec.intervalDays,
    [STORAGE_KEYS.PIN_LAST_MASTER_UNLOCK]: rec.lastMasterUnlock,
    [STORAGE_KEYS.PIN_BOOTSTRAP]: rec.bootstrap,
  });
}

// Persist just the failure counter (used between attempts so killing the worker
// mid-attempt cannot reset the count).
export async function savePinFailCount(count: number): Promise<void> {
  await browser.storage.local.set({ [STORAGE_KEYS.PIN_FAIL_COUNT]: count });
}

// Keep the PIN bootstrap's refresh token in sync with the rotated token from a
// background refresh (the server invalidates the previous token on each refresh).
// Touches ONLY the refresh token — failure counter and interval are left intact.
// No-op when no PIN/bootstrap is stored.
export async function savePinRefreshToken(token: string): Promise<void> {
  const s = await browser.storage.local.get([
    STORAGE_KEYS.PIN_ENABLED,
    STORAGE_KEYS.PIN_BOOTSTRAP,
  ]);
  if (!s[STORAGE_KEYS.PIN_ENABLED]) return;
  const bootstrap = s[STORAGE_KEYS.PIN_BOOTSTRAP] as PinBootstrap | undefined;
  if (!bootstrap) return;
  await browser.storage.local.set({
    [STORAGE_KEYS.PIN_BOOTSTRAP]: { ...bootstrap, refresh_token: token },
  });
}

// Persist the rotated refresh token + last-master-unlock after a successful
// unlock, keeping the local bootstrap usable across restarts.
export async function updatePinAfterUnlock(
  rec: PinRecord,
  refreshToken: string,
  resetInterval: boolean,
): Promise<void> {
  if (!rec.enabled || !rec.bootstrap) return;
  const bootstrap = { ...rec.bootstrap, refresh_token: refreshToken };
  await browser.storage.local.set({
    [STORAGE_KEYS.PIN_BOOTSTRAP]: bootstrap,
    [STORAGE_KEYS.PIN_FAIL_COUNT]: 0,
    ...(resetInterval ? { [STORAGE_KEYS.PIN_LAST_MASTER_UNLOCK]: Date.now() } : {}),
  });
}

export async function clearPin(): Promise<void> {
  await browser.storage.local.remove(ALL_PIN_KEYS);
}

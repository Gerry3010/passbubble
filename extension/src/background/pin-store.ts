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
// PIN-derived key. The encrypted private keys + refresh token it unlocks come
// from the device login state (account-store.ts); the PIN keeps no copy of them.

import browser from 'webextension-polyfill';
import { STORAGE_KEYS } from '../shared/constants.js';

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
  // Legacy duplicate of the login state; removed together with the PIN.
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
  });
}

// Persist just the failure counter (used between attempts so killing the worker
// mid-attempt cannot reset the count).
export async function savePinFailCount(count: number): Promise<void> {
  await browser.storage.local.set({ [STORAGE_KEYS.PIN_FAIL_COUNT]: count });
}

// After a successful unlock: reset the failure counter and, for a master-password
// unlock, restart the PIN re-auth interval. No-op when no PIN is configured.
export async function resetPinAfterUnlock(rec: PinRecord, resetInterval: boolean): Promise<void> {
  if (!rec.enabled) return;
  await browser.storage.local.set({
    [STORAGE_KEYS.PIN_FAIL_COUNT]: 0,
    ...(resetInterval ? { [STORAGE_KEYS.PIN_LAST_MASTER_UNLOCK]: Date.now() } : {}),
  });
}

export async function clearPin(): Promise<void> {
  await browser.storage.local.remove(ALL_PIN_KEYS);
}

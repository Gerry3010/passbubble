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

// A short device-local history of why the vault locked (reason and time only, no
// secrets), so an unexpected lock can be explained on the unlock screen.
//
// A lock the extension performs itself is recorded directly. The browser wiping
// storage.session leaves no such trace, so unlocking also sets a marker in
// storage.local: if the session is gone while the marker is still there, it ended
// without a lock. Writing here is best-effort and must never get in the way.

import browser from 'webextension-polyfill';
import { STORAGE_KEYS } from '../shared/constants.js';
import type { LockEvent, LockReason } from '../shared/lock-info.js';

const MAX_EVENTS = 20;

async function appendEvent(event: LockEvent): Promise<void> {
  const data = await browser.storage.local.get(STORAGE_KEYS.LOCK_LOG);
  const log = (data[STORAGE_KEYS.LOCK_LOG] as LockEvent[] | undefined) ?? [];
  await browser.storage.local.set({ [STORAGE_KEYS.LOCK_LOG]: [...log, event].slice(-MAX_EVENTS) });
}

/** Record that the vault was just unlocked. */
export async function noteUnlocked(): Promise<void> {
  try {
    await browser.storage.local.set({ [STORAGE_KEYS.SESSION_EXPECTED]: Date.now() });
  } catch {
    /* best-effort */
  }
}

/** Record a lock the extension itself performs. */
export async function recordLock(
  reason: LockReason,
  details: Pick<LockEvent, 'idleMinutes' | 'settingSource'> = {},
): Promise<void> {
  try {
    await browser.storage.local.remove(STORAGE_KEYS.SESSION_EXPECTED);
    await appendEvent({ reason, at: Date.now(), ...details });
  } catch {
    /* best-effort */
  }
}

/** The session is gone from memory and storage.session: record `reason`, unless
 * a lock was already recorded for it (no marker left). */
export async function recordSessionGone(reason: LockReason): Promise<void> {
  try {
    const data = await browser.storage.local.get(STORAGE_KEYS.SESSION_EXPECTED);
    if (data[STORAGE_KEYS.SESSION_EXPECTED] === undefined) return;
  } catch {
    return;
  }
  await recordLock(reason);
}

/** The session could not be written to storage.session, so it only lives until
 * the service worker stops. */
export async function recordPersistFailure(): Promise<void> {
  try {
    await appendEvent({ reason: 'persist-failed', at: Date.now() });
  } catch {
    /* best-effort */
  }
}

/** The most recent lock, or null if none was recorded. */
export async function readLastLock(): Promise<LockEvent | null> {
  try {
    const data = await browser.storage.local.get(STORAGE_KEYS.LOCK_LOG);
    const log = (data[STORAGE_KEYS.LOCK_LOG] as LockEvent[] | undefined) ?? [];
    return [...log].reverse().find((e) => e.reason !== 'persist-failed') ?? null;
  } catch {
    return null;
  }
}

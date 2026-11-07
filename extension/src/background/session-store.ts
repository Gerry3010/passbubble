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

// Unlocked-session state. The live copy is kept in memory; a serialized copy
// lives in chrome.storage.session (RAM only, cleared on browser close and
// extension update) so that Chrome terminating the idle service worker after
// ~30 s does not lock the vault. storage.session keeps its default access level
// (trusted extension contexts only) — content scripts can never read the keys.
// The decrypted private keys are never written to storage.local / disk.
//
// The refresh token is deliberately NOT part of the session: its single source
// of truth is the login state in storage.local (account-store.ts).

import browser from 'webextension-polyfill';
import type { UnlockedSession } from '@passbubble/shared-ts';
import type { EntryResponse } from '@passbubble/shared-ts';
import { STORAGE_KEYS } from '../shared/constants.js';
import { b64Dec, b64Enc } from '../shared/utils.js';
import { recordPersistFailure } from './lock-log.js';

let session: UnlockedSession | null = null;
let entriesCache: EntryResponse[] | null = null;
let entryTagsCache: EntryTagsCache | null = null;

// JSON-safe shape of an UnlockedSession in storage.session (byte arrays as base64).
interface StoredSession {
  v: 1;
  privX25519: string;
  privMLKEM: string;
  pubX25519: string;
  pubMLKEM: string;
  userId: string;
  userEmail: string;
  userName: string;
  role: string;
  accessToken: string;
  accessTokenExpiresAt: number;
  serverUrl: string;
  kdfSalt: string;
  kdfTime: number;
  kdfMemory: number;
  encPrivX25519: string;
  encPrivMLKEM: string;
}

function serialize(s: UnlockedSession): StoredSession {
  return {
    v: 1,
    privX25519: b64Enc(s.privX25519),
    privMLKEM: b64Enc(s.privMLKEM),
    pubX25519: b64Enc(s.pubX25519),
    pubMLKEM: b64Enc(s.pubMLKEM),
    userId: s.userId,
    userEmail: s.userEmail,
    userName: s.userName,
    role: s.role,
    accessToken: s.accessToken,
    accessTokenExpiresAt: s.accessTokenExpiresAt,
    serverUrl: s.serverUrl,
    kdfSalt: b64Enc(s.kdfSalt),
    kdfTime: s.kdfTime,
    kdfMemory: s.kdfMemory,
    encPrivX25519: b64Enc(s.encPrivX25519),
    encPrivMLKEM: b64Enc(s.encPrivMLKEM),
  };
}

function deserialize(d: StoredSession): UnlockedSession {
  return {
    privX25519: b64Dec(d.privX25519),
    privMLKEM: b64Dec(d.privMLKEM),
    pubX25519: b64Dec(d.pubX25519),
    pubMLKEM: b64Dec(d.pubMLKEM),
    userId: d.userId,
    userEmail: d.userEmail,
    userName: d.userName,
    role: d.role,
    accessToken: d.accessToken,
    refreshToken: '',
    accessTokenExpiresAt: d.accessTokenExpiresAt,
    serverUrl: d.serverUrl,
    kdfSalt: b64Dec(d.kdfSalt),
    kdfTime: d.kdfTime,
    kdfMemory: d.kdfMemory,
    encPrivX25519: b64Dec(d.encPrivX25519),
    encPrivMLKEM: b64Dec(d.encPrivMLKEM),
  };
}

/** The live unlocked session, or null while locked (or before ensureSessionRestored ran). */
export function getSession(): UnlockedSession | null {
  return session;
}

/** Make `s` the unlocked session and persist it to storage.session. Persisting is
 * best-effort: if it fails the vault still works, it just won't survive a
 * service-worker restart. */
export async function setSession(s: UnlockedSession): Promise<void> {
  session = s;
  try {
    await browser.storage.session.set({ [STORAGE_KEYS.UNLOCKED_SESSION]: serialize(s) });
  } catch {
    /* storage.session unavailable — memory-only session */
    await recordPersistFailure();
  }
}

/** Lock: drop the session from memory AND storage.session, plus the entries
 * and tags caches. */
export async function clearSession(): Promise<void> {
  session = null;
  entriesCache = null;
  entryTagsCache = null;
  try {
    await browser.storage.session.remove(STORAGE_KEYS.UNLOCKED_SESSION);
  } catch {
    /* ignore */
  }
}

/** The unlocked session persisted in storage.session, or null. Applies no lock
 * policy — the caller decides whether it may be used (see ensureSessionRestored). */
export async function readStoredSession(): Promise<UnlockedSession | null> {
  try {
    const data = await browser.storage.session.get(STORAGE_KEYS.UNLOCKED_SESSION);
    const stored = data[STORAGE_KEYS.UNLOCKED_SESSION] as StoredSession | undefined;
    if (!stored || stored.v !== 1) return null;
    return deserialize(stored);
  } catch {
    return null;
  }
}

/** Adopt an already-persisted session (from readStoredSession) into memory. */
export function adoptSession(s: UnlockedSession): void {
  session = s;
}

export function getEntriesCache(): EntryResponse[] | null {
  return entriesCache;
}

export function setEntriesCache(entries: EntryResponse[]): void {
  entriesCache = entries;
}

// Decrypted tags per entry id for the popup's tag filter (memory only, like
// the entries cache). `stamp` fingerprints the entry listing the tags were
// read for (ids + updated_at), so an entry added, removed or rewritten — here
// or on another device — makes the cache stale on the next listing refresh.
export interface EntryTagsCache {
  stamp: string;
  tags: Record<string, string[]>;
}

export function getEntryTagsCache(): EntryTagsCache | null {
  return entryTagsCache;
}

export function setEntryTagsCache(cache: EntryTagsCache): void {
  entryTagsCache = cache;
}

/** Drop the tags cache after an entry's encrypted data was written. */
export function clearEntryTagsCache(): void {
  entryTagsCache = null;
}

// Host of the frame that has a login form, per tab. The login form often lives
// in a cross-origin iframe (SSO), so the top tab URL is the wrong host to add as
// a match pattern / pre-fill the popup search with. The content script reports
// its frame host here; the popup reads it for the active tab.
const loginFrameHostByTab = new Map<number, string>();

export function setLoginFrameHost(tabId: number, host: string): void {
  if (host) loginFrameHostByTab.set(tabId, host);
}

export function getLoginFrameHost(tabId: number): string {
  return loginFrameHostByTab.get(tabId) ?? '';
}

export function clearLoginFrameHost(tabId: number): void {
  loginFrameHostByTab.delete(tabId);
}

// The entry most recently filled into each tab. Used to pick the right TOTP
// code right after a login fill — the 2FA page often no longer matches the
// entry's URL patterns (different path/subdomain), but it is almost always the
// entry that was just filled. Cleared on tab navigation/close.
const lastFilledEntryByTab = new Map<number, string>();

export function setLastFilledEntry(tabId: number, entryId: string): void {
  if (entryId) lastFilledEntryByTab.set(tabId, entryId);
}

export function getLastFilledEntry(tabId: number): string {
  return lastFilledEntryByTab.get(tabId) ?? '';
}

export function clearLastFilledEntry(tabId: number): void {
  lastFilledEntryByTab.delete(tabId);
}

// In-memory session state. Private keys are NEVER written to any storage.
// Lost on service worker termination — this is intentional (re-prompt for master password).

import type { UnlockedSession } from '@passbubble/shared-ts';
import type { EntryResponse } from '@passbubble/shared-ts';

let session: UnlockedSession | null = null;
let entriesCache: EntryResponse[] | null = null;

export function getSession(): UnlockedSession | null {
  return session;
}

export function setSession(s: UnlockedSession): void {
  session = s;
}

export function clearSession(): void {
  session = null;
  entriesCache = null;
}

export function getEntriesCache(): EntryResponse[] | null {
  return entriesCache;
}

export function setEntriesCache(entries: EntryResponse[]): void {
  entriesCache = entries;
}

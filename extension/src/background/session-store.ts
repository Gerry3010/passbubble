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

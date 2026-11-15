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

// Which entry holds which passkey, so a sign-in request can be answered (or
// handed to the browser) without decrypting the vault every time. Holds rp
// ids, credential ids, user names and entry names only; the private keys are
// read from the freshly decrypted entry when the user picks one.
//
// Built lazily, like the tags cache, for a listing stamp. The listing itself
// is fetched again at most every REFRESH_MS, so passkeys added on another
// device show up within that time. Cleared with the session on lock.

import { decryptEntry, normalizeBase64url } from '@passbubble/shared-ts';
import type { EntryResponse, PasskeyRecord, UnlockedSession } from '@passbubble/shared-ts';
import {
  getPasskeyIndexCache,
  getSession,
  listingStamp,
  setEntriesCache,
  setPasskeyIndexCache,
  type PasskeyIndexRow,
} from './session-store.js';
import { loadEntriesCache, vaultClient } from './vault-client.js';

export type { PasskeyIndexRow } from './session-store.js';

export const REFRESH_MS = 30_000;

/** The passkey records of decrypted entry data that are usable at all. */
export function passkeysOf(data: unknown): PasskeyRecord[] {
  const list = (data as { passkeys?: unknown } | null)?.passkeys;
  if (!Array.isArray(list)) return [];
  return list.filter(
    (p): p is PasskeyRecord =>
      typeof p === 'object' &&
      p !== null &&
      typeof (p as PasskeyRecord).credential_id === 'string' &&
      typeof (p as PasskeyRecord).rp_id === 'string' &&
      typeof (p as PasskeyRecord).private_key === 'string',
  );
}

/** A credential id in its stored form, or '' when it is not base64 at all. */
export function credentialIdKey(id: string): string {
  try {
    return normalizeBase64url(id);
  } catch {
    return '';
  }
}

function rowsOf(entry: EntryResponse, records: PasskeyRecord[]): PasskeyIndexRow[] {
  const rows: PasskeyIndexRow[] = [];
  for (const p of records) {
    const credentialId = credentialIdKey(p.credential_id);
    if (!credentialId) continue;
    rows.push({
      entryId: entry.id,
      entryName: entry.name,
      credentialId,
      rpId: p.rp_id.toLowerCase(),
      userName: typeof p.user_name === 'string' ? p.user_name : '',
    });
  }
  return rows;
}

async function build(session: UnlockedSession): Promise<{ stamp: string; rows: PasskeyIndexRow[] }> {
  const full = await vaultClient(session).listEntriesFull();
  const rows: PasskeyIndexRow[] = [];
  for (const entry of full) {
    if (entry.type !== 'password') continue;
    try {
      rows.push(...rowsOf(entry, passkeysOf(await decryptEntry(entry, session))));
    } catch {
      // an entry we cannot read holds no passkey we could use
    }
  }
  return { stamp: listingStamp(full), rows };
}

/** All passkeys of the vault (see the module comment for freshness). */
export async function getPasskeyIndex(session: UnlockedSession): Promise<PasskeyIndexRow[]> {
  const cached = getPasskeyIndexCache();
  const now = Date.now();
  let listing: EntryResponse[];
  if (cached && now - cached.checkedAt < REFRESH_MS) {
    listing = await loadEntriesCache(session);
  } else {
    listing = await vaultClient(session).listEntries();
    setEntriesCache(listing);
  }
  const stamp = listingStamp(listing);
  if (cached?.stamp === stamp) {
    setPasskeyIndexCache({ ...cached, checkedAt: Math.max(cached.checkedAt, now) });
    return cached.rows;
  }
  const built = await build(session);
  // A lock while decrypting already cleared the cache: do not refill it.
  if (getSession()) setPasskeyIndexCache({ stamp: built.stamp, checkedAt: now, rows: built.rows });
  return built.rows;
}

/** Forget the index after a passkey was written here. */
export function invalidatePasskeyIndex(): void {
  setPasskeyIndexCache(null);
}

/** The rows for `rpId`, narrowed to `allowIds` (stored-form credential ids)
 * when that list is not empty. */
export function rowsForRp(rows: PasskeyIndexRow[], rpId: string, allowIds: string[] = []): PasskeyIndexRow[] {
  const allowed = new Set(allowIds.map(credentialIdKey).filter(Boolean));
  return rows.filter((r) => r.rpId === rpId && (allowed.size === 0 || allowed.has(r.credentialId)));
}

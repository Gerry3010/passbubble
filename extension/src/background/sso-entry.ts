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

// Cross-device "sign in with <provider>" storage: the provider lives in the
// entry's encrypted data (EntryData.sign_in_with), so it syncs to every device
// like any other vault field. The device-local record in sso-memory.ts remains
// as a fallback for sites without a matching entry.

import { PassbubbleClient, decryptEntry, encryptEntry } from '@passbubble/shared-ts';
import { getEntriesCache, getSession, setEntriesCache } from './session-store.js';
import { matchEntriesForUrl } from './autofill-service.js';
import { isSsoProvider, type SsoProvider } from '../shared/sso.js';
import type { EntryResponse } from '@passbubble/shared-ts';

// Only login-ish entries can carry a sign-in-with provider.
const LOGIN_TYPES = new Set(['password', 'totp', 'api-key', 'ssh-key']);

// Cap the per-lookup decryption work: URL matches are typically 1–3 entries.
const MAX_MATCHES = 6;

function loginMatchesFor(url: string): EntryResponse[] {
  const cache = getEntriesCache();
  if (!cache) return [];
  return matchEntriesForUrl(url, cache)
    .filter((e) => LOGIN_TYPES.has(e.type))
    .slice(0, MAX_MATCHES);
}

/**
 * The provider stored on an entry matching `url`, or null. Reads the freshest
 * copy of each candidate from the server (the cache is metadata-only).
 */
export async function ssoFromEntries(
  url: string,
): Promise<{ provider: SsoProvider; entryId: string } | null> {
  const session = getSession();
  if (!session) return null;
  const client = new PassbubbleClient(session.serverUrl);
  client.setTokens(session.accessToken, '', 900);
  for (const m of loginMatchesFor(url)) {
    try {
      const data = await decryptEntry(await client.getEntry(m.id), session);
      if (isSsoProvider(data.sign_in_with)) return { provider: data.sign_in_with, entryId: m.id };
    } catch {
      // unreadable entry — try the next match
    }
  }
  return null;
}

/**
 * Write (provider) or clear (null) sign_in_with on every login entry matching
 * `host`. No-ops when locked or when nothing would change, so repeated calls
 * don't pile up entry versions. Best-effort per entry (read-only shares fail
 * server-side and are skipped).
 */
export async function persistSsoToEntries(host: string, provider: SsoProvider | null): Promise<void> {
  const session = getSession();
  if (!session || !host) return;
  const client = new PassbubbleClient(session.serverUrl);
  client.setTokens(session.accessToken, '', 900);
  let changed = false;
  for (const m of loginMatchesFor(`https://${host}/`)) {
    try {
      const data = await decryptEntry(await client.getEntry(m.id), session);
      if ((data.sign_in_with ?? null) === provider) continue;
      if (provider) data.sign_in_with = provider;
      else delete data.sign_in_with;
      const encrypted = await encryptEntry(data, session);
      await client.updateEntry(m.id, { ...encrypted });
      changed = true;
    } catch {
      // read-only share / decryption failure — skip this entry
    }
  }
  if (changed) {
    try {
      setEntriesCache(await client.listEntries());
    } catch {
      // cache refresh is a convenience; the next list call rebuilds it
    }
  }
}

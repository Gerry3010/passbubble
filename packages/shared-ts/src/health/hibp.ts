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

// Have-I-Been-Pwned breach check via the k-anonymity range API: only the
// first 5 hex chars of the password's SHA-1 ever leave the device; the
// matching is done locally against the returned suffix list.

const HIBP_RANGE_URL = 'https://api.pwnedpasswords.com/range/';

async function sha1Hex(input: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-1', new TextEncoder().encode(input));
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
    .toUpperCase();
}

/** Cache of range responses per 5-char prefix, so reused passwords and
 * same-prefix passwords cost one request. In-memory only. */
const rangeCache = new Map<string, Map<string, number>>();

async function fetchRange(prefix: string): Promise<Map<string, number>> {
  const cached = rangeCache.get(prefix);
  if (cached) return cached;
  const resp = await fetch(HIBP_RANGE_URL + prefix, {
    // Padding makes every response the same shape, so HIBP cannot infer
    // anything from response sizes either.
    headers: { 'Add-Padding': 'true' },
  });
  if (!resp.ok) throw new Error(`HIBP range query failed: ${resp.status}`);
  const suffixes = new Map<string, number>();
  for (const line of (await resp.text()).split('\n')) {
    const [suffix, count] = line.trim().split(':');
    if (!suffix || !count) continue;
    const n = parseInt(count, 10);
    if (n > 0) suffixes.set(suffix.toUpperCase(), n); // padding rows have count 0
  }
  rangeCache.set(prefix, suffixes);
  return suffixes;
}

/**
 * How often a password appears in known breaches (0 = not found).
 * Zero-knowledge friendly: only sha1(password)[0..5) is sent.
 */
export async function pwnedCount(password: string): Promise<number> {
  const hash = await sha1Hex(password);
  const suffixes = await fetchRange(hash.slice(0, 5));
  return suffixes.get(hash.slice(5)) ?? 0;
}

/** Test-only: reset the in-memory range cache. */
export function clearHibpCache(): void {
  rangeCache.clear();
}

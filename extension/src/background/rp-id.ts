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

// Origin and rp id rules for passkey requests (docs/passkeys.md, "Origin and
// rp id check"). The page URL comes from the browser's sender information,
// never from the page. Anything that fails here goes to the browser
// unchanged, which applies the same rules and reports its own error.
//
// Deliberately not shared/utils.ts: normaliseHost strips "www.", but
// www.example.com and example.com are different rp ids.

import { parse } from 'tldts';

export type OriginCheck =
  | { ok: true; origin: string; host: string; rpId: string }
  | { ok: false; reason: string };

const IPV4 = /^\d{1,3}(\.\d{1,3}){3}$/;

/** True for an IPv4 address or a bracketed IPv6 address (URL.hostname form). */
export function isIpHost(host: string): boolean {
  return host.startsWith('[') || IPV4.test(host);
}

/** Lowercase, no trailing dot, no port, punycode. Null when the value cannot
 * be a domain at all. */
export function normalizeRpId(raw: unknown): string | null {
  if (typeof raw !== 'string' || !raw || raw.length > 253 + 7) return null;
  let s = raw.toLowerCase().replace(/:\d{1,5}$/, '');
  if (s.endsWith('.')) s = s.slice(0, -1);
  if (!s || /[\s/?#@:\\[\]%]/.test(s)) return null;
  let host: string;
  try {
    host = new URL(`https://${s}/`).hostname;
  } catch {
    return null;
  }
  if (!host || host.startsWith('.') || host.includes('..')) return null;
  return host;
}

function isLocalhost(host: string): boolean {
  return host === 'localhost' || host.endsWith('.localhost');
}

/**
 * Checks a request from the page at `pageUrl` for `requestedRpId` (the
 * page's host when the request names none). The page must be https, or http
 * on localhost; IP addresses go to the browser. The rp id must be the host or
 * a parent domain of it on a label boundary, and not a public suffix (Public
 * Suffix List including its private section, so github.io is one).
 */
export function checkRequestOrigin(pageUrl: string, requestedRpId: unknown): OriginCheck {
  let url: URL;
  try {
    url = new URL(pageUrl);
  } catch {
    return { ok: false, reason: 'no page url' };
  }
  const rawHost = url.hostname;
  if (!rawHost) return { ok: false, reason: 'no host' };
  if (isIpHost(rawHost)) return { ok: false, reason: 'ip address' };
  const secure = url.protocol === 'https:' || (url.protocol === 'http:' && rawHost === 'localhost');
  if (!secure) return { ok: false, reason: 'insecure origin' };
  const host = rawHost.endsWith('.') ? rawHost.slice(0, -1) : rawHost;

  const rpId = requestedRpId === undefined || requestedRpId === null || requestedRpId === ''
    ? host
    : normalizeRpId(requestedRpId);
  if (!rpId) return { ok: false, reason: 'invalid rp id' };
  if (isIpHost(rpId)) return { ok: false, reason: 'ip rp id' };
  if (rpId !== host && !host.endsWith(`.${rpId}`)) return { ok: false, reason: 'rp id does not match the page' };

  if (rpId === 'localhost') {
    return isLocalhost(host) ? { ok: true, origin: url.origin, host, rpId } : { ok: false, reason: 'public suffix' };
  }
  const info = parse(rpId, { allowPrivateDomains: true });
  if (info.isIp) return { ok: false, reason: 'ip rp id' };
  if (!info.domain || info.publicSuffix === rpId) return { ok: false, reason: 'public suffix' };
  return { ok: true, origin: url.origin, host, rpId };
}

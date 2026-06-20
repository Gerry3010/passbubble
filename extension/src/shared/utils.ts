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

// Standard base64 encoding/decoding (with + / = padding, NOT url-safe).
// Matches Go's base64.StdEncoding used throughout the backend and CLI.

export function b64Enc(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes));
}

export function b64Dec(s: string): Uint8Array {
  const binary = atob(s);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

// Normalise a URL for host-based matching: strip www. prefix and path.
export function normaliseHost(url: string): string {
  try {
    const u = new URL(url.startsWith('http') ? url : `https://${url}`);
    return u.hostname.replace(/^www\./, '');
  } catch {
    return '';
  }
}

// Returns true if pageHost matches entryHost (including subdomain of entryHost).
export function hostMatches(pageHost: string, entryHost: string): boolean {
  if (!pageHost || !entryHost) return false;
  return pageHost === entryHost || pageHost.endsWith('.' + entryHost);
}

// Reduce a match pattern to a comparable host form: strip scheme, path, port and
// (for non-wildcard patterns) a leading "www." so it lines up with normaliseHost.
function normalisePattern(pattern: string): string {
  let s = pattern.trim().toLowerCase();
  if (!s) return '';
  if (s.includes('://')) {
    try {
      s = new URL(s).host;
    } catch {
      /* fall through with the raw value */
    }
  } else {
    s = s.split('/')[0]; // "github.com/login" -> "github.com"
  }
  s = s.replace(/:\d+$/, ''); // drop :port (normaliseHost uses hostname, no port)
  if (!s.includes('*')) s = s.replace(/^www\./, '');
  return s;
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

// Returns true if the page host satisfies a single match pattern.
//   "github.com"    → github.com and any subdomain (www./login.)
//   "*.github.com"  → any subdomain (but not the bare apex)
//   "127.0.0.1:8080"→ host match, port ignored
export function patternMatchesHost(pageHost: string, pattern: string): boolean {
  const pat = normalisePattern(pattern);
  if (!pageHost || !pat) return false;
  if (pat.includes('*')) {
    const re = new RegExp('^' + pat.split('*').map(escapeRegExp).join('.*') + '$');
    return re.test(pageHost);
  }
  return pageHost === pat || pageHost.endsWith('.' + pat);
}

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

// base64url without padding, as WebAuthn and docs/passkeys.md use it. The
// decoder also takes standard base64 (+ / and = padding), since imported
// passkeys and other clients may store ids that way.

const CHUNK = 0x8000;

export function base64urlEncode(bytes: Uint8Array): string {
  let binary = '';
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode(...bytes.subarray(i, i + CHUNK));
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

/** Decodes base64url or standard base64, with or without padding. Throws on
 * anything else (whitespace included), so a damaged id is never half-read. */
export function base64urlDecode(value: string): Uint8Array {
  if (typeof value !== 'string') throw new TypeError('base64url: not a string');
  const unpadded = value.replace(/=+$/, '');
  if (!/^[A-Za-z0-9+/_-]*$/.test(unpadded) || unpadded.length % 4 === 1) {
    throw new TypeError('base64url: invalid characters or length');
  }
  const std = unpadded.replace(/-/g, '+').replace(/_/g, '/');
  const binary = atob(std + '='.repeat((4 - (std.length % 4)) % 4));
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

/** The stored form of an id: base64url without padding, whatever it came in. */
export function normalizeBase64url(value: string): string {
  return base64urlEncode(base64urlDecode(value));
}

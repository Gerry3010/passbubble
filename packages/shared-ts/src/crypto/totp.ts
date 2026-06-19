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

// TOTP (RFC 6238) using Web Crypto, for displaying codes of stored 2FA secrets.

const BASE32_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';

/** Decodes an RFC 4648 base32 string (case-insensitive, padding/spaces ignored). */
export function base32Decode(input: string): Uint8Array {
  const clean = input
    .toUpperCase()
    .replace(/=+$/, '')
    .replace(/[\s-]/g, '');
  let bits = 0;
  let value = 0;
  const out: number[] = [];
  for (const ch of clean) {
    const idx = BASE32_ALPHABET.indexOf(ch);
    if (idx === -1) throw new Error('invalid base32 character');
    value = (value << 5) | idx;
    bits += 5;
    if (bits >= 8) {
      bits -= 8;
      out.push((value >>> bits) & 0xff);
    }
  }
  return new Uint8Array(out);
}

export interface TotpOptions {
  period?: number; // seconds, default 30
  digits?: number; // default 6
}

export interface TotpResult {
  code: string;
  secondsRemaining: number;
}

/**
 * Computes the current TOTP code for a base32 secret. [now] is in milliseconds
 * (defaults to Date.now()). Algorithm is HMAC-SHA1 per RFC 6238.
 */
export async function generateTotp(
  secret: string,
  opts: TotpOptions = {},
  now: number = Date.now(),
): Promise<TotpResult> {
  const period = opts.period ?? 30;
  const digits = opts.digits ?? 6;

  const key = base32Decode(secret);
  const counter = Math.floor(now / 1000 / period);

  // 8-byte big-endian counter
  const msg = new Uint8Array(8);
  let c = counter;
  for (let i = 7; i >= 0; i--) {
    msg[i] = c & 0xff;
    c = Math.floor(c / 256);
  }

  const cryptoKey = await crypto.subtle.importKey(
    'raw',
    key,
    { name: 'HMAC', hash: 'SHA-1' },
    false,
    ['sign'],
  );
  const sig = new Uint8Array(await crypto.subtle.sign('HMAC', cryptoKey, msg));

  const offset = sig[sig.length - 1] & 0x0f;
  const binary =
    ((sig[offset] & 0x7f) << 24) |
    ((sig[offset + 1] & 0xff) << 16) |
    ((sig[offset + 2] & 0xff) << 8) |
    (sig[offset + 3] & 0xff);

  const code = (binary % 10 ** digits).toString().padStart(digits, '0');
  const secondsRemaining = period - (Math.floor(now / 1000) % period);
  return { code, secondsRemaining };
}

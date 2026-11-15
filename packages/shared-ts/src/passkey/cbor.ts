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

// Minimal CBOR encoder for what an authenticator has to write: COSE keys and
// the attestation object. Output is CTAP2 canonical: shortest length forms,
// and map keys sorted by length of their encoding first, then bytewise. That
// gives {1, 3, -1, -2, -3} for a COSE key and {fmt, attStmt, authData} for an
// attestation object, the orders docs/passkeys.md prescribes.

export type CborValue =
  | number
  | string
  | boolean
  | null
  | Uint8Array
  | CborValue[]
  | CborMap;

/** A map with integer or text keys. Insertion order does not matter. */
export type CborMap = Map<number | string, CborValue>;

function head(major: number, length: number): number[] {
  if (!Number.isSafeInteger(length) || length < 0) throw new RangeError('cbor: bad length');
  const m = major << 5;
  if (length < 24) return [m | length];
  if (length < 0x100) return [m | 24, length];
  if (length < 0x10000) return [m | 25, length >> 8, length & 0xff];
  if (length < 0x100000000) {
    return [m | 26, (length >>> 24) & 0xff, (length >>> 16) & 0xff, (length >>> 8) & 0xff, length & 0xff];
  }
  const big = BigInt(length);
  const out = [m | 27];
  for (let shift = 56n; shift >= 0n; shift -= 8n) out.push(Number((big >> shift) & 0xffn));
  return out;
}

function encodeInto(value: CborValue, out: number[]): void {
  if (typeof value === 'number') {
    if (!Number.isSafeInteger(value)) throw new RangeError('cbor: only safe integers are supported');
    if (value >= 0) out.push(...head(0, value));
    else out.push(...head(1, -1 - value));
    return;
  }
  if (typeof value === 'string') {
    const bytes = new TextEncoder().encode(value);
    out.push(...head(3, bytes.length));
    for (const b of bytes) out.push(b);
    return;
  }
  if (typeof value === 'boolean') {
    out.push(value ? 0xf5 : 0xf4);
    return;
  }
  if (value === null) {
    out.push(0xf6);
    return;
  }
  if (value instanceof Uint8Array) {
    out.push(...head(2, value.length));
    for (const b of value) out.push(b);
    return;
  }
  if (Array.isArray(value)) {
    out.push(...head(4, value.length));
    for (const item of value) encodeInto(item, out);
    return;
  }
  if (value instanceof Map) {
    const entries = [...value.entries()].map(([k, v]) => {
      const key: number[] = [];
      encodeInto(k, key);
      return { key, v };
    });
    entries.sort((a, b) => compareKeys(a.key, b.key));
    for (let i = 1; i < entries.length; i++) {
      if (compareKeys(entries[i - 1].key, entries[i].key) === 0) throw new Error('cbor: duplicate map key');
    }
    out.push(...head(5, entries.length));
    for (const { key, v } of entries) {
      out.push(...key);
      encodeInto(v, out);
    }
    return;
  }
  throw new TypeError('cbor: unsupported value');
}

function compareKeys(a: number[], b: number[]): number {
  if (a.length !== b.length) return a.length - b.length;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return a[i] - b[i];
  return 0;
}

export function cborEncode(value: CborValue): Uint8Array {
  const out: number[] = [];
  encodeInto(value, out);
  return Uint8Array.from(out);
}

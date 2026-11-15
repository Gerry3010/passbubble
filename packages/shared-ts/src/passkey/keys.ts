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

// P-256 key encodings a passkey needs: PKCS#8 for the stored private key
// (docs/passkeys.md: writers store the 138-byte form with the public key,
// readers accept it with or without the optional parts), SPKI for
// getPublicKey(), and the COSE EC2 key inside the attested credential data.

import { p256 } from '@noble/curves/nist';
import { cborEncode } from './cbor.js';

// 1.2.840.10045.2.1 (id-ecPublicKey) and 1.2.840.10045.3.1.7 (prime256v1).
const OID_EC_PUBLIC_KEY = [0x2a, 0x86, 0x48, 0xce, 0x3d, 0x02, 0x01];
const OID_P256 = [0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07];

const SPKI_PREFIX = Uint8Array.from([
  0x30, 0x59, 0x30, 0x13, 0x06, 0x07, ...OID_EC_PUBLIC_KEY, 0x06, 0x08, ...OID_P256, 0x03, 0x42, 0x00,
]);

export interface P256KeyPair {
  /** The 32-byte private scalar. */
  privateKey: Uint8Array;
  /** The uncompressed public point: 0x04 ‖ x ‖ y (65 bytes). */
  publicKey: Uint8Array;
}

export function generateP256KeyPair(): P256KeyPair {
  const privateKey = p256.utils.randomSecretKey();
  return { privateKey, publicKey: p256.getPublicKey(privateKey, false) };
}

// ─── DER reading ────────────────────────────────────────────────────────────

interface Tlv {
  tag: number;
  value: Uint8Array;
  /** Offset just past this element. */
  end: number;
}

function readTlv(buf: Uint8Array, offset: number): Tlv {
  if (offset + 2 > buf.length) throw new Error('pkcs8: truncated');
  const tag = buf[offset];
  let length = buf[offset + 1];
  let pos = offset + 2;
  if (length & 0x80) {
    const n = length & 0x7f;
    if (n === 0 || n > 2 || pos + n > buf.length) throw new Error('pkcs8: bad length');
    length = 0;
    for (let i = 0; i < n; i++) length = (length << 8) | buf[pos + i];
    pos += n;
  }
  if (pos + length > buf.length) throw new Error('pkcs8: truncated');
  return { tag, value: buf.subarray(pos, pos + length), end: pos + length };
}

/** The elements of a constructed value, in order. */
function children(value: Uint8Array): Tlv[] {
  const out: Tlv[] = [];
  let pos = 0;
  while (pos < value.length) {
    const tlv = readTlv(value, pos);
    out.push(tlv);
    pos = tlv.end;
  }
  return out;
}

function sameBytes(a: Uint8Array, b: ArrayLike<number>): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

function isSmallInteger(tlv: Tlv | undefined, n: number): boolean {
  return !!tlv && tlv.tag === 0x02 && tlv.value.length === 1 && tlv.value[0] === n;
}

function checkCurveParams(params: Tlv | undefined): void {
  if (params === undefined) return;
  if (params.tag !== 0x06 || !sameBytes(params.value, OID_P256)) throw new Error('pkcs8: not a P-256 key');
}

/** ECPrivateKey (RFC 5915): version 1, the scalar, then optional [0] curve and
 * [1] public key. */
function parseEcPrivateKey(der: Uint8Array, outerHasCurve: boolean): P256KeyPair {
  const seq = readTlv(der, 0);
  if (seq.tag !== 0x30 || seq.end !== der.length) throw new Error('pkcs8: bad ECPrivateKey');
  const [version, scalar, ...rest] = children(seq.value);
  if (!isSmallInteger(version, 1) || scalar?.tag !== 0x04) throw new Error('pkcs8: bad ECPrivateKey');
  let embeddedPublic: Uint8Array | undefined;
  let curveSeen = outerHasCurve;
  for (const part of rest) {
    if (part.tag === 0xa0) {
      checkCurveParams(readTlv(part.value, 0));
      curveSeen = true;
    } else if (part.tag === 0xa1) {
      const bits = readTlv(part.value, 0);
      if (bits.tag !== 0x03 || bits.value[0] !== 0) throw new Error('pkcs8: bad public key');
      embeddedPublic = bits.value.subarray(1);
    }
  }
  if (!curveSeen) throw new Error('pkcs8: curve not stated');
  if (scalar.value.length > 32) throw new Error('pkcs8: not a P-256 key');
  // Some encoders drop leading zero bytes of the scalar.
  const privateKey = new Uint8Array(32);
  privateKey.set(scalar.value, 32 - scalar.value.length);
  if (!p256.utils.isValidSecretKey(privateKey)) throw new Error('pkcs8: invalid private key');
  const publicKey = p256.getPublicKey(privateKey, false);
  if (embeddedPublic && !sameBytes(embeddedPublic, publicKey)) {
    // A compressed copy is fine as long as it is the same point.
    const same = (() => {
      try {
        return p256.Point.fromBytes(embeddedPublic).equals(p256.Point.fromBytes(publicKey));
      } catch {
        return false;
      }
    })();
    if (!same) throw new Error('pkcs8: public key does not match the private key');
  }
  return { privateKey, publicKey };
}

/**
 * Reads a P-256 private key from PKCS#8 DER, with or without the optional
 * curve parameters and public key inside. A bare SEC1 ECPrivateKey (what
 * `openssl ec` writes) is accepted too. Throws for any other key.
 */
export function parsePkcs8P256(der: Uint8Array): P256KeyPair {
  const outer = readTlv(der, 0);
  if (outer.tag !== 0x30 || outer.end !== der.length) throw new Error('pkcs8: not a DER sequence');
  const parts = children(outer.value);
  if (isSmallInteger(parts[0], 1)) return parseEcPrivateKey(der, false);
  const [version, algorithm, privateKey] = parts;
  if (!isSmallInteger(version, 0) || algorithm?.tag !== 0x30 || privateKey?.tag !== 0x04) {
    throw new Error('pkcs8: not a PKCS#8 private key');
  }
  const [oid, params] = children(algorithm.value);
  if (oid?.tag !== 0x06 || !sameBytes(oid.value, OID_EC_PUBLIC_KEY)) throw new Error('pkcs8: not an EC key');
  checkCurveParams(params);
  return parseEcPrivateKey(privateKey.value, params !== undefined);
}

/** The long PKCS#8 form (138 bytes) that WebCrypto exports: curve in the
 * algorithm identifier, public key inside the ECPrivateKey. */
export function encodePkcs8P256(privateKey: Uint8Array, publicKey?: Uint8Array): Uint8Array {
  if (privateKey.length !== 32) throw new Error('pkcs8: private key must be 32 bytes');
  const pub = publicKey ?? p256.getPublicKey(privateKey, false);
  if (pub.length !== 65 || pub[0] !== 0x04) throw new Error('pkcs8: public key must be uncompressed');
  return Uint8Array.from([
    0x30, 0x81, 0x87,
    0x02, 0x01, 0x00,
    0x30, 0x13, 0x06, 0x07, ...OID_EC_PUBLIC_KEY, 0x06, 0x08, ...OID_P256,
    0x04, 0x6d,
    0x30, 0x6b,
    0x02, 0x01, 0x01,
    0x04, 0x20, ...privateKey,
    0xa1, 0x44, 0x03, 0x42, 0x00, ...pub,
  ]);
}

/** SubjectPublicKeyInfo DER (91 bytes), what getPublicKey() returns. */
export function encodeSpkiP256(publicKey: Uint8Array): Uint8Array {
  if (publicKey.length !== 65 || publicKey[0] !== 0x04) throw new Error('spki: public key must be uncompressed');
  const out = new Uint8Array(SPKI_PREFIX.length + 65);
  out.set(SPKI_PREFIX);
  out.set(publicKey, SPKI_PREFIX.length);
  return out;
}

/** COSE_Key for ES256 in canonical CBOR: {1: 2, 3: -7, -1: 1, -2: x, -3: y}. */
export function encodeCoseEc2Key(publicKey: Uint8Array): Uint8Array {
  if (publicKey.length !== 65 || publicKey[0] !== 0x04) throw new Error('cose: public key must be uncompressed');
  return cborEncode(
    new Map<number, number | Uint8Array>([
      [1, 2],
      [3, -7],
      [-1, 1],
      [-2, publicKey.slice(1, 33)],
      [-3, publicKey.slice(33, 65)],
    ]),
  );
}

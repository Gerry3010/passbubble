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

// The authenticator side of a passkey, as docs/passkeys.md defines it:
// creating a credential (key pair, record, attestation object) and signing an
// assertion. Only ES256. The sign counter is always 0 and never written back.

import { p256 } from '@noble/curves/nist';
import type { PasskeyRecord } from '../types/vault.js';
import { base64urlDecode, base64urlEncode } from './base64url.js';
import { cborEncode, type CborMap } from './cbor.js';
import { encodeCoseEc2Key, encodePkcs8P256, encodeSpkiP256, generateP256KeyPair, parsePkcs8P256 } from './keys.js';

export const PASSBUBBLE_AAGUID = '47683042-7680-4bfb-92d5-9410ed00226d';

export const AuthenticatorFlags = {
  UP: 0x01,
  UV: 0x04,
  BE: 0x08,
  BS: 0x10,
  AT: 0x40,
} as const;

/** UP | UV | BE | BS: what an assertion reports. */
export const ASSERTION_FLAGS = 0x1d;
/** ASSERTION_FLAGS | AT: what a creation reports. */
export const CREATION_FLAGS = 0x5d;

/** COSE algorithm id of ES256, the only one Passbubble creates. */
export const ES256 = -7;

/** The AAGUID as its 16 bytes. */
export function aaguidBytes(aaguid: string = PASSBUBBLE_AAGUID): Uint8Array {
  const hex = aaguid.replace(/-/g, '');
  if (!/^[0-9a-f]{32}$/i.test(hex)) throw new Error('aaguid: not a UUID');
  const out = new Uint8Array(16);
  for (let i = 0; i < 16; i++) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}

export async function sha256(bytes: Uint8Array): Promise<Uint8Array> {
  return new Uint8Array(await crypto.subtle.digest('SHA-256', bytes as BufferSource));
}

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let pos = 0;
  for (const p of parts) {
    out.set(p, pos);
    pos += p.length;
  }
  return out;
}

export interface AttestedCredentialData {
  credentialId: Uint8Array;
  /** Uncompressed P-256 public point. */
  publicKey: Uint8Array;
}

/**
 * SHA-256(rpId) ‖ flags ‖ sign count (always 0) ‖ attested credential data
 * (creation only: AAGUID, id length, id, COSE key).
 */
export async function buildAuthenticatorData(
  rpId: string,
  flags: number,
  attested?: AttestedCredentialData,
): Promise<Uint8Array> {
  const rpIdHash = await sha256(new TextEncoder().encode(rpId));
  const header = concat(rpIdHash, Uint8Array.of(flags & 0xff), new Uint8Array(4));
  if (!attested) return header;
  const { credentialId, publicKey } = attested;
  if (credentialId.length === 0 || credentialId.length > 1023) throw new Error('passkey: bad credential id length');
  const idLength = Uint8Array.of(credentialId.length >> 8, credentialId.length & 0xff);
  return concat(header, aaguidBytes(), idLength, credentialId, encodeCoseEc2Key(publicKey));
}

/** {"fmt": "none", "attStmt": {}, "authData": …} in canonical CBOR. */
export function encodeAttestationObject(authData: Uint8Array): Uint8Array {
  const attestation: CborMap = new Map();
  attestation.set('fmt', 'none');
  attestation.set('attStmt', new Map());
  attestation.set('authData', authData);
  return cborEncode(attestation);
}

export interface ClientDataParams {
  type: 'webauthn.create' | 'webauthn.get';
  /** The request's challenge, base64url without padding. */
  challenge: string;
  origin: string;
}

/** clientDataJSON with the keys in the order the contract fixes:
 * type, challenge, origin, crossOrigin (always false — top frame only). */
export function buildClientDataJSON({ type, challenge, origin }: ClientDataParams): Uint8Array {
  const json = JSON.stringify({ type, challenge, origin, crossOrigin: false });
  return new TextEncoder().encode(json);
}

export interface CreatePasskeyParams {
  rpId: string;
  rpName?: string;
  /** The site's user.id. */
  userHandle: Uint8Array;
  userName: string;
  userDisplayName?: string;
  /** Defaults to CREATION_FLAGS. */
  flags?: number;
  /** Defaults to now. */
  createdAt?: Date;
}

export interface CreatedPasskey {
  /** What goes into the entry's `passkeys` array. */
  record: PasskeyRecord;
  credentialId: Uint8Array;
  authenticatorData: Uint8Array;
  attestationObject: Uint8Array;
  /** SubjectPublicKeyInfo DER, for getPublicKey(). */
  publicKeySpki: Uint8Array;
  /** COSE_Key, as inside the authenticator data. */
  publicKeyCose: Uint8Array;
}

/** RFC 3339 in UTC without fractional seconds, e.g. 2026-11-14T03:00:00Z. */
function rfc3339(date: Date): string {
  return date.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

/**
 * Creates a new discoverable ES256 passkey: key pair, 16-byte credential id,
 * 32-byte prf_secret, the record to store and the attestation object
 * (fmt "none") to hand to the site.
 */
export async function createPasskey(params: CreatePasskeyParams): Promise<CreatedPasskey> {
  if (!params.rpId) throw new Error('passkey: rp id missing');
  if (params.userHandle.length === 0 || params.userHandle.length > 64) throw new Error('passkey: bad user handle');
  const { privateKey, publicKey } = generateP256KeyPair();
  const credentialId = crypto.getRandomValues(new Uint8Array(16));
  const prfSecret = crypto.getRandomValues(new Uint8Array(32));
  const authenticatorData = await buildAuthenticatorData(params.rpId, params.flags ?? CREATION_FLAGS, {
    credentialId,
    publicKey,
  });
  const record: PasskeyRecord = {
    credential_id: base64urlEncode(credentialId),
    rp_id: params.rpId,
    ...(params.rpName ? { rp_name: params.rpName } : {}),
    user_handle: base64urlEncode(params.userHandle),
    user_name: params.userName,
    ...(params.userDisplayName ? { user_display_name: params.userDisplayName } : {}),
    key_algorithm: ES256,
    private_key: base64urlEncode(encodePkcs8P256(privateKey, publicKey)),
    sign_count: 0,
    discoverable: true,
    prf_secret: base64urlEncode(prfSecret),
    created_at: rfc3339(params.createdAt ?? new Date()),
  };
  privateKey.fill(0);
  return {
    record,
    credentialId,
    authenticatorData,
    attestationObject: encodeAttestationObject(authenticatorData),
    publicKeySpki: encodeSpkiP256(publicKey),
    publicKeyCose: encodeCoseEc2Key(publicKey),
  };
}

export interface Assertion {
  credentialId: Uint8Array;
  authenticatorData: Uint8Array;
  /** DER-encoded ECDSA signature (low S). */
  signature: Uint8Array;
  userHandle: Uint8Array;
}

/**
 * Signs authenticatorData ‖ clientDataHash with the record's key. Refuses a
 * record for another rp id or algorithm, so a caller that picked the wrong
 * record fails instead of answering.
 */
export async function signAssertion(
  record: Pick<PasskeyRecord, 'credential_id' | 'rp_id' | 'user_handle' | 'key_algorithm' | 'private_key'>,
  rpId: string,
  clientDataHash: Uint8Array,
  flags: number = ASSERTION_FLAGS,
): Promise<Assertion> {
  if (record.rp_id !== rpId) throw new Error('passkey: record belongs to another rp id');
  if ((record.key_algorithm ?? ES256) !== ES256) throw new Error('passkey: unsupported algorithm');
  if (clientDataHash.length !== 32) throw new Error('passkey: client data hash must be 32 bytes');
  const { privateKey } = parsePkcs8P256(base64urlDecode(record.private_key));
  const authenticatorData = await buildAuthenticatorData(rpId, flags);
  try {
    const signature = p256
      .sign(concat(authenticatorData, clientDataHash), privateKey, { prehash: true, lowS: true, extraEntropy: true })
      .toBytes('der');
    return {
      credentialId: base64urlDecode(record.credential_id),
      authenticatorData,
      signature,
      userHandle: base64urlDecode(record.user_handle),
    };
  } finally {
    privateKey.fill(0);
  }
}

/** SPKI DER of a stored record's public key. */
export function publicKeySpkiOf(record: Pick<PasskeyRecord, 'private_key'>): Uint8Array {
  const { privateKey, publicKey } = parsePkcs8P256(base64urlDecode(record.private_key));
  privateKey.fill(0);
  return encodeSpkiP256(publicKey);
}

/** Re-encodes a private key (PKCS#8 in any accepted form, base64 or base64url)
 * into the stored long form, base64url. Throws for anything but P-256. */
export function normalizePasskeyPrivateKey(value: string): string {
  const { privateKey, publicKey } = parsePkcs8P256(base64urlDecode(value));
  try {
    return base64urlEncode(encodePkcs8P256(privateKey, publicKey));
  } finally {
    privateKey.fill(0);
  }
}

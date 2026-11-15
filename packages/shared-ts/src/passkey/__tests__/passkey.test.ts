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

import { describe, it, expect } from 'vitest';
import { createPublicKey, verify as nodeVerify, webcrypto } from 'node:crypto';
import { verifyAuthenticationResponse, verifyRegistrationResponse } from '@simplewebauthn/server';
import { decodeAttestationObject, isoCBOR, parseAuthenticatorData } from '@simplewebauthn/server/helpers';
import {
  ASSERTION_FLAGS,
  CREATION_FLAGS,
  PASSBUBBLE_AAGUID,
  aaguidBytes,
  base64urlDecode,
  base64urlEncode,
  buildAuthenticatorData,
  buildClientDataJSON,
  cborEncode,
  createPasskey,
  encodeAttestationObject,
  encodeCoseEc2Key,
  encodePkcs8P256,
  encodeSpkiP256,
  normalizeBase64url,
  normalizePasskeyPrivateKey,
  parsePkcs8P256,
  publicKeySpkiOf,
  sha256,
  signAssertion,
} from '../index.js';
import type { PasskeyRecord } from '../../types/vault.js';

const hex = (b: Uint8Array) => Buffer.from(b).toString('hex');
const std64 = (b: Uint8Array) => Buffer.from(b).toString('base64');

// openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256, then
// `openssl pkcs8 -topk8 -nocrypt` (long PKCS#8) and `openssl ec` (SEC1).
const OPENSSL_PKCS8 =
  'MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgKtv8pfxs7SaDjT6z8GIrVocvHs5nxE4ppvB45KjD5QahRANCAATq0E/FeXCwsWf+E0IATM7BamgKSQ4WkyrWYYtzl1GiF9kauF14wk6+/qT/IzufNXxm5GPAdZeGl6H+nbFbXq4G';
const OPENSSL_SEC1 =
  'MHcCAQEEICrb/KX8bO0mg40+s/BiK1aHLx7OZ8ROKabweOSow+UGoAoGCCqGSM49AwEHoUQDQgAE6tBPxXlwsLFn/hNCAEzOwWpoCkkOFpMq1mGLc5dRohfZGrhdeMJOvv6k/yM7nzV8ZuRjwHWXhpeh/p2xW16uBg==';
const OPENSSL_SPKI =
  'MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE6tBPxXlwsLFn/hNCAEzOwWpoCkkOFpMq1mGLc5dRohfZGrhdeMJOvv6k/yM7nzV8ZuRjwHWXhpeh/p2xW16uBg==';

describe('base64url', () => {
  it('encodes without padding and url-safe', () => {
    expect(base64urlEncode(Uint8Array.of(0xfb, 0xff))).toBe('-_8');
    expect(base64urlEncode(new Uint8Array())).toBe('');
  });

  it('decodes base64url and standard base64, padded or not', () => {
    for (const s of ['-_8', '+/8=', '+/8', '-_8=']) expect([...base64urlDecode(s)]).toEqual([0xfb, 0xff]);
    expect(normalizeBase64url('dXNlci0xMjM0')).toBe('dXNlci0xMjM0');
    expect(normalizeBase64url('q2D0bQ2u3E7XoWlS1yZbVw==')).toBe('q2D0bQ2u3E7XoWlS1yZbVw');
  });

  it('rejects garbage', () => {
    expect(() => base64urlDecode('ab cd')).toThrow();
    expect(() => base64urlDecode('a')).toThrow();
    expect(() => base64urlDecode('ab$d')).toThrow();
  });

  it('round-trips large buffers', () => {
    const big = Uint8Array.from({ length: 100_000 }, (_, i) => (i * 7919) & 0xff);
    expect(hex(base64urlDecode(base64urlEncode(big)))).toBe(hex(big));
  });
});

describe('cbor', () => {
  it('encodes integers in their shortest form', () => {
    expect(hex(cborEncode(0))).toBe('00');
    expect(hex(cborEncode(23))).toBe('17');
    expect(hex(cborEncode(24))).toBe('1818');
    expect(hex(cborEncode(256))).toBe('190100');
    expect(hex(cborEncode(65536))).toBe('1a00010000');
    expect(hex(cborEncode(2 ** 32))).toBe('1b0000000100000000');
    expect(hex(cborEncode(-1))).toBe('20');
    expect(hex(cborEncode(-7))).toBe('26');
    expect(hex(cborEncode(-25))).toBe('3818');
  });

  it('encodes strings, bytes, arrays, booleans', () => {
    expect(hex(cborEncode('fmt'))).toBe('63666d74');
    expect(hex(cborEncode(Uint8Array.of(1, 2)))).toBe('420102');
    expect(hex(cborEncode([1, 'a', true, false, null]))).toBe('8501616' + '1f5f4f6');
    expect(hex(cborEncode('ü'))).toBe('62c3bc');
  });

  it('sorts map keys canonically (length first, then bytes)', () => {
    const m = new Map<number | string, number>([
      ['authData', 1],
      [-3, 2],
      ['fmt', 3],
      [1, 4],
      ['attStmt', 5],
      [-1, 6],
    ]);
    // 1, -1, -3, "fmt", "attStmt", "authData"
    expect(hex(cborEncode(m))).toBe('a6' + '0104' + '2006' + '2202' + '63666d7403' + '6761747453746d7405' + '6861757468446174610' + '1');
  });

  it('rejects non-integer numbers', () => {
    expect(() => cborEncode(1.5)).toThrow();
  });

  it('decodes back with an independent decoder', () => {
    const value = new Map<number | string, unknown>([
      [1, 2],
      ['x', Uint8Array.of(9)],
    ]);
    const decoded = isoCBOR.decodeFirst<Map<number | string, unknown>>(cborEncode(value as never));
    expect(decoded.get(1)).toBe(2);
    expect([...(decoded.get('x') as Uint8Array)]).toEqual([9]);
  });
});

describe('P-256 keys', () => {
  it('parses the long PKCS#8 form openssl writes and re-encodes it byte for byte', () => {
    const der = new Uint8Array(Buffer.from(OPENSSL_PKCS8, 'base64'));
    expect(der.length).toBe(138);
    const { privateKey, publicKey } = parsePkcs8P256(der);
    expect(privateKey.length).toBe(32);
    expect(publicKey.length).toBe(65);
    expect(hex(encodePkcs8P256(privateKey, publicKey))).toBe(hex(der));
    expect(std64(encodeSpkiP256(publicKey))).toBe(OPENSSL_SPKI);
  });

  it('parses SEC1, the short PKCS#8 form and the form with inner parameters', () => {
    const long = parsePkcs8P256(new Uint8Array(Buffer.from(OPENSSL_PKCS8, 'base64')));
    const sec1 = parsePkcs8P256(new Uint8Array(Buffer.from(OPENSSL_SEC1, 'base64')));
    expect(hex(sec1.privateKey)).toBe(hex(long.privateKey));

    const d = [...long.privateKey];
    const algId = [0x30, 0x13, 0x06, 0x07, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x02, 0x01, 0x06, 0x08, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07];
    // Without the public key (67 bytes, e.g. Bitwarden / Java).
    const short = Uint8Array.from([0x30, 0x41, 0x02, 0x01, 0x00, ...algId, 0x04, 0x27, 0x30, 0x25, 0x02, 0x01, 0x01, 0x04, 0x20, ...d]);
    expect(hex(parsePkcs8P256(short).publicKey)).toBe(hex(long.publicKey));
    // With [0] curve parameters inside the ECPrivateKey.
    const params = [0xa0, 0x0a, 0x06, 0x08, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07];
    const inner = [0x30, 0x25 + params.length, 0x02, 0x01, 0x01, 0x04, 0x20, ...d, ...params];
    const withParams = Uint8Array.from([0x30, 3 + algId.length + 2 + inner.length, 0x02, 0x01, 0x00, ...algId, 0x04, inner.length, ...inner]);
    expect(hex(parsePkcs8P256(withParams).publicKey)).toBe(hex(long.publicKey));
  });

  it('rejects other curves, other key types and a mismatched public key', () => {
    const der = new Uint8Array(Buffer.from(OPENSSL_PKCS8, 'base64'));
    const otherCurve = der.slice();
    otherCurve[26] ^= 0x01; // last byte of the prime256v1 OID
    expect(() => parsePkcs8P256(otherCurve)).toThrow();
    const wrongPub = der.slice();
    wrongPub[137] ^= 0x01;
    expect(() => parsePkcs8P256(wrongPub)).toThrow();
    expect(() => parsePkcs8P256(Uint8Array.of(0x30, 0x00))).toThrow();
    expect(() => parsePkcs8P256(der.subarray(0, 100))).toThrow();
  });

  it('parses what WebCrypto exports and its signatures verify in WebCrypto', async () => {
    const pair = await webcrypto.subtle.generateKey({ name: 'ECDSA', namedCurve: 'P-256' }, true, ['sign', 'verify']);
    const pkcs8 = new Uint8Array(await webcrypto.subtle.exportKey('pkcs8', pair.privateKey));
    const spki = new Uint8Array(await webcrypto.subtle.exportKey('spki', pair.publicKey));
    expect(pkcs8.length).toBe(138);
    const record = {
      credential_id: base64urlEncode(Uint8Array.of(1, 2, 3)),
      rp_id: 'example.com',
      user_handle: 'dXNlcg',
      key_algorithm: -7,
      private_key: base64urlEncode(pkcs8),
    };
    expect(hex(publicKeySpkiOf(record))).toBe(hex(spki));
    const clientDataHash = await sha256(new TextEncoder().encode('{}'));
    const { authenticatorData, signature } = await signAssertion(record, 'example.com', clientDataHash);
    const signed = new Uint8Array([...authenticatorData, ...clientDataHash]);
    // Node verifies DER signatures directly against the SPKI.
    const key = createPublicKey({ key: Buffer.from(spki), format: 'der', type: 'spki' });
    expect(nodeVerify('sha256', signed, key, signature)).toBe(true);
  });

  it('signs verifiably with the openssl key (fixed vector)', async () => {
    const record = {
      credential_id: 'AQID',
      rp_id: 'login.example.co.uk',
      user_handle: 'dXNlcg',
      key_algorithm: -7,
      private_key: OPENSSL_PKCS8, // standard base64, as another client may store it
    };
    const hash = await sha256(new TextEncoder().encode('hello passkey'));
    const { authenticatorData, signature } = await signAssertion(record, 'login.example.co.uk', hash);
    expect(signature[0]).toBe(0x30);
    const key = createPublicKey({ key: Buffer.from(OPENSSL_SPKI, 'base64'), format: 'der', type: 'spki' });
    expect(nodeVerify('sha256', new Uint8Array([...authenticatorData, ...hash]), key, signature)).toBe(true);
    expect(normalizePasskeyPrivateKey(OPENSSL_SEC1)).toBe(base64urlEncode(new Uint8Array(Buffer.from(OPENSSL_PKCS8, 'base64'))));
  });

  it('writes the COSE key as {1:2, 3:-7, -1:1, -2:x, -3:y}', () => {
    const { publicKey } = parsePkcs8P256(new Uint8Array(Buffer.from(OPENSSL_PKCS8, 'base64')));
    const cose = encodeCoseEc2Key(publicKey);
    expect(hex(cose.subarray(0, 9))).toBe('a5' + '0102' + '0326' + '2001' + '21' + '58');
    expect(cose.length).toBe(77);
    const decoded = isoCBOR.decodeFirst<Map<number, unknown>>(cose);
    expect([...decoded.keys()]).toEqual([1, 3, -1, -2, -3]);
  });
});

describe('authenticator data and attestation', () => {
  it('builds the assertion form: rp id hash, flags, zero counter', async () => {
    const data = await buildAuthenticatorData('example.com', ASSERTION_FLAGS);
    expect(data.length).toBe(37);
    expect(hex(data.subarray(0, 32))).toBe(hex(await sha256(new TextEncoder().encode('example.com'))));
    expect(data[32]).toBe(0x1d);
    expect(hex(data.subarray(33))).toBe('00000000');
  });

  it('encodes the attestation object with keys fmt, attStmt, authData', () => {
    const obj = encodeAttestationObject(Uint8Array.of(7));
    expect(hex(obj)).toBe('a3' + '63666d74' + '646e6f6e65' + '6761747453746d74' + 'a0' + '6861757468446174' + '61' + '4107');
  });

  it('builds clientDataJSON with the fixed key order', () => {
    const json = new TextDecoder().decode(
      buildClientDataJSON({ type: 'webauthn.get', challenge: 'abc', origin: 'https://example.com' }),
    );
    expect(json).toBe('{"type":"webauthn.get","challenge":"abc","origin":"https://example.com","crossOrigin":false}');
  });

  it('knows its AAGUID', () => {
    expect(hex(aaguidBytes())).toBe(PASSBUBBLE_AAGUID.replace(/-/g, ''));
  });
});

describe('createPasskey', () => {
  it('returns a record that follows the storage contract', async () => {
    const created = await createPasskey({
      rpId: 'example.com',
      rpName: 'Example',
      userHandle: new TextEncoder().encode('user-1234'),
      userName: 'alice@example.com',
      userDisplayName: 'Alice',
      createdAt: new Date('2026-11-14T03:00:00.123Z'),
    });
    const r = created.record;
    expect(r).toMatchObject({
      rp_id: 'example.com',
      rp_name: 'Example',
      user_handle: 'dXNlci0xMjM0',
      user_name: 'alice@example.com',
      user_display_name: 'Alice',
      key_algorithm: -7,
      sign_count: 0,
      discoverable: true,
      created_at: '2026-11-14T03:00:00Z',
    });
    expect(base64urlDecode(r.credential_id).length).toBe(16);
    expect(base64urlDecode(r.prf_secret!).length).toBe(32);
    expect(base64urlDecode(r.private_key).length).toBe(138);
    for (const s of [r.credential_id, r.user_handle, r.private_key, r.prf_secret!]) expect(s).toMatch(/^[A-Za-z0-9_-]+$/);
    expect(hex(publicKeySpkiOf(r))).toBe(hex(created.publicKeySpki));

    const parsed = parseAuthenticatorData(created.authenticatorData);
    expect(parsed.flagsBuf[0]).toBe(CREATION_FLAGS);
    expect(parsed.counter).toBe(0);
    expect(hex(parsed.aaguid!)).toBe(PASSBUBBLE_AAGUID.replace(/-/g, ''));
    expect(hex(parsed.credentialID!)).toBe(hex(created.credentialId));
    const att = decodeAttestationObject(created.attestationObject);
    expect(att.get('fmt')).toBe('none');
    expect(hex(att.get('authData'))).toBe(hex(created.authenticatorData));
  });

  it('leaves out optional names that were not given', async () => {
    const { record } = await createPasskey({ rpId: 'example.com', userHandle: Uint8Array.of(1), userName: 'bob' });
    expect('rp_name' in record).toBe(false);
    expect('user_display_name' in record).toBe(false);
  });

  it('refuses bad user handles', async () => {
    await expect(createPasskey({ rpId: 'a.com', userHandle: new Uint8Array(), userName: 'x' })).rejects.toThrow();
    await expect(createPasskey({ rpId: 'a.com', userHandle: new Uint8Array(65), userName: 'x' })).rejects.toThrow();
  });
});

describe('end to end against @simplewebauthn/server', () => {
  const origin = 'https://login.example.com';
  const rpId = 'example.com';

  async function register(): Promise<{ record: PasskeyRecord; cose: Uint8Array }> {
    const challenge = base64urlEncode(webcrypto.getRandomValues(new Uint8Array(32)));
    const created = await createPasskey({ rpId, userHandle: Uint8Array.of(9, 9), userName: 'alice' });
    const clientDataJSON = buildClientDataJSON({ type: 'webauthn.create', challenge, origin });
    const id = created.record.credential_id;
    const result = await verifyRegistrationResponse({
      response: {
        id,
        rawId: id,
        type: 'public-key',
        response: {
          clientDataJSON: base64urlEncode(clientDataJSON),
          attestationObject: base64urlEncode(created.attestationObject),
          authenticatorData: base64urlEncode(created.authenticatorData),
          publicKey: base64urlEncode(created.publicKeySpki),
          publicKeyAlgorithm: -7,
          transports: ['internal', 'hybrid'],
        },
        clientExtensionResults: {},
        authenticatorAttachment: 'platform',
      },
      expectedChallenge: challenge,
      expectedOrigin: origin,
      expectedRPID: rpId,
      requireUserVerification: true,
    });
    expect(result.verified).toBe(true);
    const info = result.registrationInfo!;
    expect(info.fmt).toBe('none');
    expect(info.aaguid).toBe(PASSBUBBLE_AAGUID);
    expect(info.credential.id).toBe(id);
    expect(info.credentialDeviceType).toBe('multiDevice');
    expect(info.credentialBackedUp).toBe(true);
    expect(info.userVerified).toBe(true);
    return { record: created.record, cose: info.credential.publicKey };
  }

  it('registers and signs in', async () => {
    const { record, cose } = await register();
    const challenge = base64urlEncode(webcrypto.getRandomValues(new Uint8Array(32)));
    const clientDataJSON = buildClientDataJSON({ type: 'webauthn.get', challenge, origin });
    const assertion = await signAssertion(record, rpId, await sha256(clientDataJSON));
    const result = await verifyAuthenticationResponse({
      response: {
        id: record.credential_id,
        rawId: record.credential_id,
        type: 'public-key',
        response: {
          clientDataJSON: base64urlEncode(clientDataJSON),
          authenticatorData: base64urlEncode(assertion.authenticatorData),
          signature: base64urlEncode(assertion.signature),
          userHandle: base64urlEncode(assertion.userHandle),
        },
        clientExtensionResults: {},
        authenticatorAttachment: 'platform',
      },
      expectedChallenge: challenge,
      expectedOrigin: origin,
      expectedRPID: rpId,
      credential: { id: record.credential_id, publicKey: cose, counter: 0 },
      requireUserVerification: true,
    });
    expect(result.verified).toBe(true);
    expect(result.authenticationInfo.newCounter).toBe(0);
    expect(base64urlEncode(assertion.userHandle)).toBe(record.user_handle);
  });

  it('fails verification for a tampered client data or the wrong rp id', async () => {
    const { record, cose } = await register();
    const challenge = 'Y2hhbGxlbmdl';
    const clientDataJSON = buildClientDataJSON({ type: 'webauthn.get', challenge, origin });
    const assertion = await signAssertion(record, rpId, await sha256(clientDataJSON));
    const otherClientData = buildClientDataJSON({ type: 'webauthn.get', challenge, origin: 'https://evil.example.com' });
    await expect(
      verifyAuthenticationResponse({
        response: {
          id: record.credential_id,
          rawId: record.credential_id,
          type: 'public-key',
          response: {
            clientDataJSON: base64urlEncode(otherClientData),
            authenticatorData: base64urlEncode(assertion.authenticatorData),
            signature: base64urlEncode(assertion.signature),
          },
          clientExtensionResults: {},
        },
        expectedChallenge: challenge,
        expectedOrigin: 'https://evil.example.com',
        expectedRPID: rpId,
        credential: { id: record.credential_id, publicKey: cose, counter: 0 },
      }).then((r) => r.verified),
    ).resolves.toBe(false);
    await expect(signAssertion(record, 'other.com', await sha256(clientDataJSON))).rejects.toThrow(/rp id/);
  });

  it('refuses a record with another algorithm', async () => {
    const { record } = await register();
    await expect(signAssertion({ ...record, key_algorithm: -8 }, rpId, new Uint8Array(32))).rejects.toThrow();
    await expect(signAssertion(record, rpId, new Uint8Array(31))).rejects.toThrow();
  });
});

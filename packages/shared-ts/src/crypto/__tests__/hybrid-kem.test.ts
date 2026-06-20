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

import { describe, expect, it } from 'vitest';
import { MLKEM768_CT_SIZE, generateMLKEM768 } from '../mlkem.js';
import { generateX25519, x25519PublicKey, x25519SharedSecret } from '../x25519.js';
import { aesGcmEncrypt } from '../aes-gcm.js';
import { decryptDataKey, encryptDataKey } from '../hybrid-kem.js';

const X25519_PUB_LEN = 32;

// Builds a Flutter legacy (X25519-only) wire key: ephPub(32) || AES-GCM with the
// raw X25519 shared secret as the AES key. Mirrors Go's encryptDataKeyX25519Only.
async function legacyEncryptDataKey(dataKey: Uint8Array, recipPubX: Uint8Array): Promise<Uint8Array> {
  const ephPriv = generateX25519().priv;
  const ephPub = x25519PublicKey(ephPriv);
  const shared = x25519SharedSecret(ephPriv, recipPubX);
  const enc = await aesGcmEncrypt(shared, dataKey);
  const out = new Uint8Array(ephPub.length + enc.length);
  out.set(ephPub);
  out.set(enc, ephPub.length);
  return out;
}

describe('hybrid-kem', () => {
  it('encrypt + decrypt round-trip: recovered key matches original', async () => {
    const { priv: privX, pub: pubX } = generateX25519();
    const { priv: privM, pub: pubM } = await generateMLKEM768();
    const dataKey = crypto.getRandomValues(new Uint8Array(32));

    const encKey = await encryptDataKey(dataKey, pubX, pubM);
    const recovered = await decryptDataKey(encKey, privX, privM);

    expect(recovered).toEqual(dataKey);
  });

  it('wire format: ephemPub(32) || mlkemCT(1088) || gcm output', async () => {
    const { pub: pubX } = generateX25519();
    const { pub: pubM } = await generateMLKEM768();
    const dataKey = crypto.getRandomValues(new Uint8Array(32));

    const encKey = await encryptDataKey(dataKey, pubX, pubM);

    expect(encKey.length).toBeGreaterThan(X25519_PUB_LEN + MLKEM768_CT_SIZE + 12 + 16);
    // Offsets: 0-31 = x25519 ephemeral pub, 32-1119 = ml-kem ct, 1120+ = aes-gcm output
    expect(encKey.length - X25519_PUB_LEN - MLKEM768_CT_SIZE).toBe(
      12 + 32 + 16, // nonce(12) + key(32) + tag(16)
    );
  });

  it('wrong x25519 key: decrypt throws', async () => {
    const { pub: pubX } = generateX25519();
    const { priv: wrongPrivX } = generateX25519();
    const { priv: privM, pub: pubM } = await generateMLKEM768();
    const dataKey = crypto.getRandomValues(new Uint8Array(32));

    const encKey = await encryptDataKey(dataKey, pubX, pubM);
    await expect(decryptDataKey(encKey, wrongPrivX, privM)).rejects.toThrow();
  });

  it('truncated encKey throws', async () => {
    const { priv: privX } = generateX25519();
    const { priv: privM } = await generateMLKEM768();
    await expect(decryptDataKey(new Uint8Array(32), privX, privM)).rejects.toThrow();
  });

  it('decrypts the Flutter legacy (X25519-only) wire format', async () => {
    const { priv: privX, pub: pubX } = generateX25519();
    const { priv: privM } = await generateMLKEM768();
    const dataKey = crypto.getRandomValues(new Uint8Array(32));

    const legacyKey = await legacyEncryptDataKey(dataKey, pubX);
    // Legacy keys are far shorter than a hybrid key (~92 vs ~1180 bytes).
    expect(legacyKey.length).toBeLessThan(X25519_PUB_LEN + MLKEM768_CT_SIZE);

    const recovered = await decryptDataKey(legacyKey, privX, privM);
    expect(recovered).toEqual(dataKey);
  });

  it('two encryptions of same key produce different blobs', async () => {
    const { pub: pubX } = generateX25519();
    const { pub: pubM } = await generateMLKEM768();
    const dataKey = crypto.getRandomValues(new Uint8Array(32));

    const enc1 = await encryptDataKey(dataKey, pubX, pubM);
    const enc2 = await encryptDataKey(dataKey, pubX, pubM);
    expect(enc1).not.toEqual(enc2);
  });
});

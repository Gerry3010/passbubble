// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version. See <https://www.gnu.org/licenses/>.

// Loads the built web bundle (the exact code the browser runs) and exercises
// globalThis.passbubbleCrypto end-to-end: ML-KEM keygen + hybrid wrap/unwrap.
// X25519 keys come from node:crypto (raw RFC7748 bytes, like Flutter/Go produce).
import assert from 'node:assert';
import crypto from 'node:crypto';
import '../web/passbubble_crypto.js'; // IIFE → sets globalThis.passbubbleCrypto

const pc = globalThis.passbubbleCrypto;
assert.ok(pc && pc.generateMlKem768 && pc.encryptDataKey && pc.decryptDataKey,
  'passbubbleCrypto API not exposed by the bundle');

const rawTail = (key, type) => {
  const der = key.export({ type, format: 'der' });
  return new Uint8Array(der.subarray(der.length - 32)); // last 32 bytes = raw X25519 key
};

const { publicKey, privateKey } = crypto.generateKeyPairSync('x25519');
const pubX = rawTail(publicKey, 'spki');
const privX = rawTail(privateKey, 'pkcs8');

const { priv: mPriv, pub: mPub } = await pc.generateMlKem768();
assert.strictEqual(mPub.length, 1184, `ML-KEM pub should be 1184 bytes, got ${mPub.length}`);

const dataKey = new Uint8Array(32).map((_, i) => (i * 7) % 256);
const enc = await pc.encryptDataKey(dataKey, pubX, mPub);
assert.ok(enc.length > 32 + 1088, 'hybrid blob too short');

const dec = await pc.decryptDataKey(enc, privX, mPriv);
assert.deepStrictEqual(new Uint8Array(dec), dataKey, 'round-trip mismatch');

console.log('web_crypto bundle: hybrid round-trip OK');

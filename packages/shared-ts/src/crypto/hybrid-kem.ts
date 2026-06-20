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

// Hybrid KEM: X25519 + ML-KEM-768.
// Wire format: ephemPub(32) || mlkemCT(1088) || nonce(12)+gcmCT
// Mirrors Go EncryptDataKey / DecryptDataKey in backend/pkg/crypto/crypto.go.

import { aesGcmDecrypt, aesGcmEncrypt } from './aes-gcm.js';
import { hkdfSha256 } from './hkdf.js';
import { MLKEM768_CT_SIZE, MLKEM768_EK_SIZE, mlkemDecapsulate, mlkemEncapsulate } from './mlkem.js';
import { generateX25519, x25519PublicKey, x25519SharedSecret } from './x25519.js';

const X25519_PUB_LEN = 32;
// HKDF info string must match Go exactly: "passbubble-hybrid-kem-v1"
const HKDF_INFO = new TextEncoder().encode('passbubble-hybrid-kem-v1');

async function hybridKDF(classical: Uint8Array, pq: Uint8Array): Promise<Uint8Array> {
  const ikm = new Uint8Array(classical.length + pq.length);
  ikm.set(classical);
  ikm.set(pq, classical.length);
  return hkdfSha256(ikm, null, HKDF_INFO, 32);
}

// Flutter legacy X25519-only wire format: ephPub(32) || AES-256-GCM(dataKey) with
// the raw X25519 shared secret used directly as the AES key (no HKDF). Mirrors
// Go's encryptDataKeyX25519Only and is readable by decryptDataKey's legacy branch.
export async function encryptDataKeyX25519Only(
  dataKey: Uint8Array,
  recipX25519Pub: Uint8Array,
): Promise<Uint8Array> {
  const { priv: ephemPriv } = generateX25519();
  const ephemPub = x25519PublicKey(ephemPriv);
  const shared = x25519SharedSecret(ephemPriv, recipX25519Pub);
  const encDataKey = await aesGcmEncrypt(shared, dataKey);

  const out = new Uint8Array(ephemPub.length + encDataKey.length);
  out.set(ephemPub, 0);
  out.set(encDataKey, ephemPub.length);
  return out;
}

export async function encryptDataKey(
  dataKey: Uint8Array,
  recipX25519Pub: Uint8Array,
  recipMLKEMPub: Uint8Array,
): Promise<Uint8Array> {
  // Accounts without a valid ML-KEM public key (e.g. created by the X25519-only
  // Flutter app) can't be hybrid-encrypted — fall back to the legacy X25519-only
  // format, which all clients can decrypt. Hybrid accounts (1184-byte EK) keep PQ.
  if (recipMLKEMPub.length !== MLKEM768_EK_SIZE) {
    return encryptDataKeyX25519Only(dataKey, recipX25519Pub);
  }

  const { priv: ephemPriv } = generateX25519();
  const ephemPub = x25519PublicKey(ephemPriv);
  const sharedX25519 = x25519SharedSecret(ephemPriv, recipX25519Pub);

  const { ciphertext: mlkemCT, sharedSecret: sharedMLKEM } = await mlkemEncapsulate(recipMLKEMPub);

  const combined = await hybridKDF(sharedX25519, sharedMLKEM);
  const encDataKey = await aesGcmEncrypt(combined, dataKey);

  const out = new Uint8Array(X25519_PUB_LEN + MLKEM768_CT_SIZE + encDataKey.length);
  out.set(ephemPub, 0);
  out.set(mlkemCT, X25519_PUB_LEN);
  out.set(encDataKey, X25519_PUB_LEN + MLKEM768_CT_SIZE);
  return out;
}

// Decrypts a data key, auto-detecting the wire format (mirrors Go's
// DecryptDataKey in cli/internal/crypto/crypto.go):
//   - Hybrid (≥ 32+mlkemCTSize bytes): ephPub(32) || mlkem_ct || AES-GCM
//   - Legacy (< that):                 ephPub(32) || AES-GCM — produced by the
//     Flutter app (X25519-only, raw shared secret used directly as the AES key).
export async function decryptDataKey(
  encKey: Uint8Array,
  privX25519: Uint8Array,
  privMLKEM: Uint8Array,
): Promise<Uint8Array> {
  if (encKey.length >= X25519_PUB_LEN + MLKEM768_CT_SIZE) {
    const ephemPub = encKey.slice(0, X25519_PUB_LEN);
    const mlkemCT = encKey.slice(X25519_PUB_LEN, X25519_PUB_LEN + MLKEM768_CT_SIZE);
    const encDataKey = encKey.slice(X25519_PUB_LEN + MLKEM768_CT_SIZE);

    const sharedX25519 = x25519SharedSecret(privX25519, ephemPub);
    const sharedMLKEM = await mlkemDecapsulate(privMLKEM, mlkemCT);

    const combined = await hybridKDF(sharedX25519, sharedMLKEM);
    return aesGcmDecrypt(combined, encDataKey);
  }

  // Flutter legacy wire format: ephPub(32) || AES-256-GCM(nonce||ct||tag) with
  // the raw X25519 shared secret used directly as the AES key (no HKDF).
  if (encKey.length < X25519_PUB_LEN + 12) {
    throw new Error('hybrid-kem: encrypted key too short');
  }
  const ephemPub = encKey.slice(0, X25519_PUB_LEN);
  const encDataKey = encKey.slice(X25519_PUB_LEN);
  const shared = x25519SharedSecret(privX25519, ephemPub);
  return aesGcmDecrypt(shared, encDataKey);
}

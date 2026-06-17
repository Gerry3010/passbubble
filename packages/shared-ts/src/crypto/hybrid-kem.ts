// Hybrid KEM: X25519 + ML-KEM-768.
// Wire format: ephemPub(32) || mlkemCT(1088) || nonce(12)+gcmCT
// Mirrors Go EncryptDataKey / DecryptDataKey in backend/pkg/crypto/crypto.go.

import { aesGcmDecrypt, aesGcmEncrypt } from './aes-gcm.js';
import { hkdfSha256 } from './hkdf.js';
import { MLKEM768_CT_SIZE, mlkemDecapsulate, mlkemEncapsulate } from './mlkem.js';
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

export async function encryptDataKey(
  dataKey: Uint8Array,
  recipX25519Pub: Uint8Array,
  recipMLKEMPub: Uint8Array,
): Promise<Uint8Array> {
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

export async function decryptDataKey(
  encKey: Uint8Array,
  privX25519: Uint8Array,
  privMLKEM: Uint8Array,
): Promise<Uint8Array> {
  const minLen = X25519_PUB_LEN + MLKEM768_CT_SIZE + 1;
  if (encKey.length <= minLen) {
    throw new Error('hybrid-kem: encrypted key too short');
  }

  const ephemPub = encKey.slice(0, X25519_PUB_LEN);
  const mlkemCT = encKey.slice(X25519_PUB_LEN, X25519_PUB_LEN + MLKEM768_CT_SIZE);
  const encDataKey = encKey.slice(X25519_PUB_LEN + MLKEM768_CT_SIZE);

  const sharedX25519 = x25519SharedSecret(privX25519, ephemPub);
  const sharedMLKEM = await mlkemDecapsulate(privMLKEM, mlkemCT);

  const combined = await hybridKDF(sharedX25519, sharedMLKEM);
  return aesGcmDecrypt(combined, encDataKey);
}

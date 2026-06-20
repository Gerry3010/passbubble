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

// AES-256-GCM using Web Crypto API.
// Wire format matches Go crypto.Encrypt / Decrypt: nonce(12) || ciphertext.

const KEY_ALGO = { name: 'AES-GCM', length: 256 };
const NONCE_LEN = 12;

export async function aesGcmEncrypt(key: Uint8Array, plaintext: Uint8Array): Promise<Uint8Array> {
  const cryptoKey = await crypto.subtle.importKey('raw', key as unknown as BufferSource, KEY_ALGO, false, ['encrypt']);
  const nonce = crypto.getRandomValues(new Uint8Array(NONCE_LEN));
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce }, cryptoKey, plaintext as unknown as BufferSource);
  const out = new Uint8Array(NONCE_LEN + ct.byteLength);
  out.set(nonce);
  out.set(new Uint8Array(ct), NONCE_LEN);
  return out;
}

// Decrypts nonce(12) || ciphertext produced by aesGcmEncrypt / Go Encrypt.
export async function aesGcmDecrypt(key: Uint8Array, nonceAndCiphertext: Uint8Array): Promise<Uint8Array> {
  if (nonceAndCiphertext.length < NONCE_LEN) {
    throw new Error('aes-gcm: ciphertext too short');
  }
  const nonce = nonceAndCiphertext.slice(0, NONCE_LEN);
  const ct = nonceAndCiphertext.slice(NONCE_LEN);
  const cryptoKey = await crypto.subtle.importKey('raw', key as unknown as BufferSource, KEY_ALGO, false, ['decrypt']);
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce }, cryptoKey, ct);
  return new Uint8Array(pt);
}

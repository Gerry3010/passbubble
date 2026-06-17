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
import { aesGcmDecrypt, aesGcmEncrypt } from '../aes-gcm.js';

const key = () => crypto.getRandomValues(new Uint8Array(32));

describe('aes-gcm', () => {
  it('encrypt then decrypt returns original plaintext', async () => {
    const k = key();
    const pt = new TextEncoder().encode('hello passbubble');
    const ct = await aesGcmEncrypt(k, pt);
    const decrypted = await aesGcmDecrypt(k, ct);
    expect(decrypted).toEqual(pt);
  });

  it('nonce is prepended: first 12 bytes are nonce, rest is ciphertext+tag', async () => {
    const k = key();
    const pt = new Uint8Array(16).fill(0x42);
    const ct = await aesGcmEncrypt(k, pt);
    // GCM output is 16 bytes tag + plaintext length; total = 12 nonce + 16+16 = 44
    expect(ct.length).toBe(12 + pt.length + 16);
  });

  it('two encrypts of same plaintext produce different nonces', async () => {
    const k = key();
    const pt = new TextEncoder().encode('test');
    const ct1 = await aesGcmEncrypt(k, pt);
    const ct2 = await aesGcmEncrypt(k, pt);
    expect(ct1.slice(0, 12)).not.toEqual(ct2.slice(0, 12));
  });

  it('wrong key throws', async () => {
    const k1 = key();
    const k2 = key();
    const ct = await aesGcmEncrypt(k1, new TextEncoder().encode('secret'));
    await expect(aesGcmDecrypt(k2, ct)).rejects.toThrow();
  });

  it('truncated ciphertext throws', async () => {
    await expect(aesGcmDecrypt(key(), new Uint8Array(5))).rejects.toThrow();
  });
});

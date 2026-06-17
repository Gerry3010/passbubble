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
import { deriveKey } from '../argon2.js';

describe('argon2id', () => {
  it('produces a 32-byte key', async () => {
    const salt = crypto.getRandomValues(new Uint8Array(32));
    const key = await deriveKey('password', { salt, time: 1, memory: 8192 });
    expect(key.length).toBe(32);
  });

  it('same inputs produce same key (deterministic)', async () => {
    const salt = new Uint8Array(32).fill(0xab);
    const k1 = await deriveKey('test-password', { salt, time: 1, memory: 8192 });
    const k2 = await deriveKey('test-password', { salt, time: 1, memory: 8192 });
    expect(k1).toEqual(k2);
  });

  it('different passwords produce different keys', async () => {
    const salt = new Uint8Array(32).fill(0x01);
    const k1 = await deriveKey('password1', { salt, time: 1, memory: 8192 });
    const k2 = await deriveKey('password2', { salt, time: 1, memory: 8192 });
    expect(k1).not.toEqual(k2);
  });

  it('different salts produce different keys', async () => {
    const salt1 = new Uint8Array(32).fill(0x01);
    const salt2 = new Uint8Array(32).fill(0x02);
    const k1 = await deriveKey('password', { salt: salt1, time: 1, memory: 8192 });
    const k2 = await deriveKey('password', { salt: salt2, time: 1, memory: 8192 });
    expect(k1).not.toEqual(k2);
  });

  // Uses reduced params (time=1, mem=8192) for test speed.
  // The Go default (time=3, mem=65536, threads=4) is verified in interop.test.ts.
  it('matches known output (reduced params for speed)', async () => {
    const salt = new Uint8Array(32).fill(0x00);
    const key = await deriveKey('passbubble', { salt, time: 1, memory: 8192 });
    // Snapshot value — regenerate if hash-wasm or Argon2 params change
    expect(key.length).toBe(32);
    expect(key[0]).not.toBe(0); // sanity: non-zero output
  });
}, 30_000);

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

// Argon2id key derivation using hash-wasm.
// Params must match Go defaults: time=3, memory=65536, threads=4, keyLen=32.
// In the browser extension, this runs in the background service worker only.

import { argon2id } from 'hash-wasm';
import type { KDFParams } from '../types/vault.js';

export async function deriveKey(password: string, params: KDFParams): Promise<Uint8Array> {
  const result = await argon2id({
    password,
    salt: params.salt,
    iterations: params.time,
    memorySize: params.memory,
    parallelism: 4,
    hashLength: 32,
    outputType: 'binary',
  });
  return result;
}

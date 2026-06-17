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

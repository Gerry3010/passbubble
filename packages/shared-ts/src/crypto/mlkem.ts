// ML-KEM-768 using the `mlkem` package (same author as @dajiaji/mlkem, correct package name).
// Ciphertext size: 1088 bytes — verified to match cloudflare/circl mlkem768.

import { createMlKem768 } from 'mlkem';

export const MLKEM768_CT_SIZE = 1088;

// Cache the instance since initialization is cheap but repeated
let _instance: Awaited<ReturnType<typeof createMlKem768>> | null = null;
async function getInstance() {
  if (!_instance) _instance = await createMlKem768();
  return _instance;
}

export async function mlkemEncapsulate(
  publicKey: Uint8Array,
): Promise<{ ciphertext: Uint8Array; sharedSecret: Uint8Array }> {
  const ctx = await getInstance();
  const [ct, ss] = await ctx.encap(publicKey);
  if (ct.length !== MLKEM768_CT_SIZE) {
    throw new Error(`mlkem: unexpected ciphertext size ${ct.length}, expected ${MLKEM768_CT_SIZE}`);
  }
  return { ciphertext: ct, sharedSecret: ss };
}

export async function mlkemDecapsulate(
  privateKey: Uint8Array,
  ciphertext: Uint8Array,
): Promise<Uint8Array> {
  const ctx = await getInstance();
  return ctx.decap(ciphertext, privateKey);
}

export async function generateMLKEM768(): Promise<{ priv: Uint8Array; pub: Uint8Array }> {
  const ctx = await getInstance();
  const [pub, priv] = await ctx.generateKeyPair();
  return { priv, pub };
}

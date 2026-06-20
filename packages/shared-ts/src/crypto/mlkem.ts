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

// ML-KEM-768 using the `mlkem` package (same author as @dajiaji/mlkem, correct package name).
// Ciphertext size: 1088 bytes — verified to match cloudflare/circl mlkem768.

import { createMlKem768 } from 'mlkem';

export const MLKEM768_CT_SIZE = 1088;
// ML-KEM-768 encapsulation (public) key size. Used to detect accounts that have
// no valid ML-KEM key (e.g. X25519-only Flutter accounts) and fall back to the
// legacy encryption format.
export const MLKEM768_EK_SIZE = 1184;

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

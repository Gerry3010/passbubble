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

// High-level vault operations: unlock, getEntry, createEntry.
// Mirrors cli/internal/vault/vault.go.

import { PassbubbleClient } from '../api/client.js';
import { aesGcmDecrypt, aesGcmEncrypt } from '../crypto/aes-gcm.js';
import { deriveKey } from '../crypto/argon2.js';
import { decryptDataKey, encryptDataKey } from '../crypto/hybrid-kem.js';
import type { EntryResponse } from '../types/api.js';
import type { EntryData, KDFParams, UnlockedSession } from '../types/vault.js';

export type { EntryData } from '../types/vault.js';

export function b64Enc(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes));
}

export function b64Dec(s: string): Uint8Array {
  const binary = atob(s);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

export interface VaultEntry {
  id: string;
  name: string;
  url?: string;
  type: string;
  folderId?: string;
  data?: EntryData;
}

// Derive master key + decrypt private keys from the login response data.
export async function unlock(
  masterPassword: string,
  kdfParams: KDFParams,
  encPrivX25519Base64: string,
  encPrivMLKEMBase64: string,
): Promise<{ privX25519: Uint8Array; privMLKEM: Uint8Array }> {
  const masterKey = await deriveKey(masterPassword, kdfParams);

  const encPrivX25519 = b64Dec(encPrivX25519Base64);
  const privX25519 = await aesGcmDecrypt(masterKey, encPrivX25519);

  const encPrivMLKEM = b64Dec(encPrivMLKEMBase64);
  const privMLKEM = await aesGcmDecrypt(masterKey, encPrivMLKEM);

  return { privX25519, privMLKEM };
}

// Decrypt a single entry from the API response.
export async function decryptEntry(
  apiEntry: EntryResponse,
  session: Pick<UnlockedSession, 'privX25519' | 'privMLKEM'>,
): Promise<EntryData> {
  if (!apiEntry.entry_key || !apiEntry.encrypted_data) {
    throw new Error('vault: entry missing entry_key or encrypted_data');
  }
  const encKey = b64Dec(apiEntry.entry_key.encrypted_key);
  const dataKey = await decryptDataKey(encKey, session.privX25519, session.privMLKEM);
  const ciphertext = b64Dec(apiEntry.encrypted_data);
  const plaintext = await aesGcmDecrypt(dataKey, ciphertext);
  return JSON.parse(new TextDecoder().decode(plaintext)) as EntryData;
}

// Encrypt entry data and produce CreateEntryRequest fields.
export async function encryptEntry(
  data: EntryData,
  session: Pick<UnlockedSession, 'privX25519' | 'privMLKEM' | 'pubX25519' | 'pubMLKEM' | 'userId'>,
): Promise<{ encrypted_data: string; data_nonce: string; entry_keys: Array<{ user_id: string; encrypted_key: string }> }> {
  const dataKey = crypto.getRandomValues(new Uint8Array(32));

  const plaintext = new TextEncoder().encode(JSON.stringify(data));
  const ciphertext = await aesGcmEncrypt(dataKey, plaintext);

  const encKey = await encryptDataKey(dataKey, session.pubX25519, session.pubMLKEM);

  const placeholder = new Uint8Array(12); // data_nonce is unused; send 12 zero bytes like CLI

  return {
    encrypted_data: b64Enc(ciphertext),
    data_nonce: b64Enc(placeholder),
    entry_keys: [{ user_id: session.userId, encrypted_key: b64Enc(encKey) }],
  };
}

// Convenience: fetch + decrypt a single entry via the API client.
export async function getEntry(
  client: PassbubbleClient,
  id: string,
  session: Pick<UnlockedSession, 'privX25519' | 'privMLKEM'>,
): Promise<VaultEntry> {
  const apiEntry = await client.getEntry(id);
  const data = await decryptEntry(apiEntry, session);
  return {
    id: apiEntry.id,
    name: apiEntry.name,
    url: apiEntry.url,
    type: apiEntry.type,
    folderId: apiEntry.folder_id,
    data,
  };
}

// Convenience: encrypt + create an entry via the API client.
export async function createEntry(
  client: PassbubbleClient,
  name: string,
  type: string,
  url: string | undefined,
  data: EntryData,
  session: Pick<UnlockedSession, 'privX25519' | 'privMLKEM' | 'pubX25519' | 'pubMLKEM' | 'userId'>,
  folderId?: string,
): Promise<{ id: string }> {
  const encrypted = await encryptEntry(data, session);
  return client.createEntry({
    name,
    type,
    url,
    folder_id: folderId,
    ...encrypted,
  });
}

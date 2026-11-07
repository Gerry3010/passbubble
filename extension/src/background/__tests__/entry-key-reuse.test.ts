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

// Updates of an existing entry must keep its data key: every other recipient
// holds that same key wrapped for them, and the backend only upserts the
// entry_keys it receives. Runs the real crypto and session store against a fake
// server that treats entry_keys like the backend does.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { EntryData, EntryResponse, UnlockedSession } from '@passbubble/shared-ts';

const sessionStore = vi.hoisted(() => ({} as Record<string, unknown>));

vi.mock('webextension-polyfill', () => ({
  default: {
    storage: {
      sync: { get: vi.fn().mockResolvedValue({}), set: vi.fn() },
      session: {
        get: vi.fn(async (key: string) => (key in sessionStore ? { [key]: sessionStore[key] } : {})),
        set: vi.fn(async (obj: Record<string, unknown>) => void Object.assign(sessionStore, obj)),
        remove: vi.fn(async (key: string) => void delete sessionStore[key]),
      },
      local: { get: vi.fn().mockResolvedValue({}), set: vi.fn(), remove: vi.fn() },
    },
    action: { setBadgeText: vi.fn(), setBadgeBackgroundColor: vi.fn() },
    alarms: { create: vi.fn(), clear: vi.fn() },
  },
}));

const client = vi.hoisted(() => ({
  setTokens: vi.fn(),
  listEntries: vi.fn(),
  getEntry: vi.fn(),
  updateEntry: vi.fn(),
}));

vi.mock('@passbubble/shared-ts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@passbubble/shared-ts')>()),
  PassbubbleClient: vi.fn().mockImplementation(() => client),
}));

import {
  aesGcmEncrypt,
  b64Dec,
  b64Enc,
  decryptDataKey,
  decryptEntry,
  encryptDataKey,
  generateMLKEM768,
  generateX25519,
  type UpdateEntryRequest,
} from '@passbubble/shared-ts';
import { buildHandlers } from '../message-handler.js';
import { persistSsoToEntries } from '../sso-entry.js';
import { clearSession, setSession } from '../session-store.js';
import { MessageType, STORAGE_KEYS } from '../../shared/constants.js';

interface Keys {
  privX25519: Uint8Array;
  privMLKEM: Uint8Array;
  pubX25519: Uint8Array;
  pubMLKEM: Uint8Array;
}

async function makeKeys(): Promise<Keys> {
  const x = generateX25519();
  const m = await generateMLKEM768();
  return { privX25519: x.priv, privMLKEM: m.priv, pubX25519: x.pub, pubMLKEM: m.pub };
}

function makeSession(keys: Keys): UnlockedSession {
  return {
    ...keys,
    userId: 'owner',
    userEmail: 'owner@example.com',
    userName: 'Owner',
    role: 'user',
    accessToken: 'at',
    refreshToken: '',
    accessTokenExpiresAt: Date.now() + 3_600_000,
    serverUrl: 'https://srv',
    kdfSalt: new Uint8Array(16),
    kdfTime: 3,
    kdfMemory: 65536,
    encPrivX25519: new Uint8Array(60),
    encPrivMLKEM: new Uint8Array(60),
  };
}

// The one entry on the fake server, shared between "owner" and "friend".
interface StoredEntry {
  encrypted_data: string;
  data_nonce: string;
  folder_id?: string;
  match_patterns?: string[];
  keys: Record<string, string>;
}

const META = { id: 'e1', owner_id: 'owner', type: 'password', name: 'Shared', created_at: '', updated_at: '' };

function viewFor(stored: StoredEntry, userId: string): EntryResponse {
  return {
    ...META,
    url: 'https://example.com',
    folder_id: stored.folder_id,
    match_patterns: stored.match_patterns,
    encrypted_data: stored.encrypted_data,
    data_nonce: stored.data_nonce,
    entry_key: { user_id: userId, encrypted_key: stored.keys[userId] },
  };
}

describe('updates keep the entry data key', () => {
  let owner: Keys;
  let friend: Keys;
  let dataKey: Uint8Array;
  let stored: StoredEntry;
  let handlers: ReturnType<typeof buildHandlers>;

  beforeEach(async () => {
    for (const k of Object.keys(sessionStore)) delete sessionStore[k];
    owner = await makeKeys();
    friend = await makeKeys();
    dataKey = crypto.getRandomValues(new Uint8Array(32));
    const initial: EntryData = { username: 'alice', password: 'old-password', notes: 'keep me' };
    stored = {
      encrypted_data: b64Enc(await aesGcmEncrypt(dataKey, new TextEncoder().encode(JSON.stringify(initial)))),
      data_nonce: b64Enc(new Uint8Array(12)),
      folder_id: 'f-work',
      match_patterns: ['example.com'],
      keys: {
        owner: b64Enc(await encryptDataKey(dataKey, owner.pubX25519, owner.pubMLKEM)),
        friend: b64Enc(await encryptDataKey(dataKey, friend.pubX25519, friend.pubMLKEM)),
      },
    };

    client.getEntry.mockReset().mockImplementation(async () => viewFor(stored, 'owner'));
    client.listEntries.mockReset().mockImplementation(async () => [viewFor(stored, 'owner')]);
    // Like the backend: data is replaced, entry_keys are only upserted.
    client.updateEntry.mockReset().mockImplementation(async (_id: string, req: UpdateEntryRequest) => {
      if (req.encrypted_data) stored.encrypted_data = req.encrypted_data;
      if (req.data_nonce) stored.data_nonce = req.data_nonce;
      stored.folder_id = req.folder_id;
      if (req.match_patterns) stored.match_patterns = req.match_patterns;
      for (const ek of req.entry_keys ?? []) stored.keys[ek.user_id] = ek.encrypted_key;
    });

    await clearSession();
    await setSession(makeSession(owner));
    handlers = buildHandlers();
  });

  const friendReads = () => decryptEntry(viewFor(stored, 'friend'), friend);

  async function expectDataKeyKept() {
    const wrappedForOwner = b64Dec(stored.keys.owner);
    expect(await decryptDataKey(wrappedForOwner, owner.privX25519, owner.privMLKEM)).toEqual(dataKey);
    expect(await decryptDataKey(b64Dec(stored.keys.friend), friend.privX25519, friend.privMLKEM)).toEqual(dataKey);
  }

  it('UPDATE_SAVE: a second recipient can still read the entry', async () => {
    sessionStore[STORAGE_KEYS.PENDING_SAVE] = {
      name: 'Shared',
      url: 'https://example.com',
      username: 'alice',
      password: 'new-password',
    };

    const res = (await handlers[MessageType.UPDATE_SAVE]({ entryId: 'e1' }, {})) as { ok?: boolean };

    expect(res.ok).toBe(true);
    await expectDataKeyKept();
    expect(await friendReads()).toEqual({ username: 'alice', password: 'new-password', notes: 'keep me' });
  });

  it('SSO write path: a second recipient can still read the entry', async () => {
    await persistSsoToEntries('example.com', 'google');

    expect(client.updateEntry).toHaveBeenCalledTimes(1);
    await expectDataKeyKept();
    expect(await friendReads()).toEqual({
      username: 'alice',
      password: 'old-password',
      notes: 'keep me',
      sign_in_with: 'google',
    });
  });

  it('EDIT_ENTRY: a second recipient can still read the entry', async () => {
    const res = (await handlers[MessageType.EDIT_ENTRY](
      { entryId: 'e1', name: 'Shared', url: 'https://example.com', fields: { password: 'edited' } },
      {},
    )) as { ok?: boolean };

    expect(res.ok).toBe(true);
    await expectDataKeyKept();
    expect(await friendReads()).toEqual({ username: 'alice', password: 'edited', notes: 'keep me' });
  });

  it('nothing is overwritten when the data key cannot be unwrapped', async () => {
    stored.keys.owner = b64Enc(await encryptDataKey(dataKey, friend.pubX25519, friend.pubMLKEM));
    sessionStore[STORAGE_KEYS.PENDING_SAVE] = {
      name: 'Shared',
      url: 'https://example.com',
      username: 'alice',
      password: 'new-password',
    };

    const saved = (await handlers[MessageType.UPDATE_SAVE]({ entryId: 'e1' }, {})) as { ok?: boolean };
    const edited = (await handlers[MessageType.EDIT_ENTRY](
      { entryId: 'e1', name: 'Shared', url: '', fields: { password: 'edited' } },
      {},
    )) as { ok?: boolean; error?: string };
    await persistSsoToEntries('example.com', 'google');

    expect(saved.ok).toBe(false);
    expect(edited.ok).toBe(false);
    expect(edited.error).toMatch(/nothing was changed/);
    expect(client.updateEntry).not.toHaveBeenCalled();
    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeDefined();
  });
});

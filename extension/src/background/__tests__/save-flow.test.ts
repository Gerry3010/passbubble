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

import { describe, it, expect, vi, beforeEach } from 'vitest';

// In-memory session + local storage so the save flow + blocklist work. These are
// referenced only inside the lazy vi.fn callbacks (not at factory-eval time), so
// the hoisted vi.mock factory can use them.
const sessionStore: Record<string, unknown> = {};
const localStore: Record<string, unknown> = {};

vi.mock('webextension-polyfill', () => ({
  default: {
    storage: {
      sync: { get: vi.fn().mockResolvedValue({ server_url: 'https://srv' }), set: vi.fn() },
      session: {
        get: vi.fn(async (keys: string | string[]) => {
          const arr = Array.isArray(keys) ? keys : [keys];
          const out: Record<string, unknown> = {};
          for (const k of arr) if (k in sessionStore) out[k] = sessionStore[k];
          return out;
        }),
        set: vi.fn(async (obj: Record<string, unknown>) => Object.assign(sessionStore, obj)),
        remove: vi.fn(async (k: string) => {
          delete sessionStore[k];
        }),
      },
      local: {
        get: vi.fn(async (keys: string | string[]) => {
          const arr = Array.isArray(keys) ? keys : [keys];
          const out: Record<string, unknown> = {};
          for (const k of arr) if (k in localStore) out[k] = localStore[k];
          return out;
        }),
        set: vi.fn(async (obj: Record<string, unknown>) => Object.assign(localStore, obj)),
        remove: vi.fn(async (k: string) => {
          delete localStore[k];
        }),
      },
    },
    action: { setBadgeText: vi.fn(), setBadgeBackgroundColor: vi.fn() },
    alarms: { create: vi.fn(), clear: vi.fn() },
  },
}));

const createEntryMock = vi.hoisted(() => vi.fn().mockResolvedValue({ id: 'e1' }));
const encryptEntryMock = vi.hoisted(() =>
  vi.fn().mockResolvedValue({
    encrypted_data: 'ed',
    data_nonce: 'dn',
    entry_keys: [{ user_id: 'u', encrypted_key: 'ek' }],
  }),
);
const decryptEntryMock = vi.hoisted(() => vi.fn());
const getEntryMock = vi.hoisted(() => vi.fn(async (id: string) => ({ id })));
const updateEntryMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const listEntriesMock = vi.hoisted(() => vi.fn().mockResolvedValue([]));
const getEntriesCacheMock = vi.hoisted(() => vi.fn());

vi.mock('../session-store.js', () => ({
  // Include valid-length public keys so assertCanEncrypt() passes (ML-KEM-768
  // encapsulation key = 1184 bytes, X25519 public key = 32 bytes).
  getSession: vi.fn(() => ({
    serverUrl: 'https://srv',
    accessToken: 'at',
    pubMLKEM: new Uint8Array(1184),
    pubX25519: new Uint8Array(32),
  })),
  setSession: vi.fn(),
  clearSession: vi.fn(),
  getEntriesCache: getEntriesCacheMock,
  setEntriesCache: vi.fn(),
}));

vi.mock('@passbubble/shared-ts', () => ({
  PassbubbleClient: vi.fn().mockImplementation(() => ({
    setTokens: vi.fn(),
    listEntries: listEntriesMock,
    getEntry: getEntryMock,
    updateEntry: updateEntryMock,
  })),
  unlock: vi.fn(),
  decryptEntry: decryptEntryMock,
  encryptEntry: encryptEntryMock,
  createEntry: createEntryMock,
}));

import { buildHandlers } from '../message-handler.js';
import { MessageType, STORAGE_KEYS } from '../../shared/constants.js';

// A metadata-only cache entry (as listEntries returns) that matchEntriesForUrl
// will match against example.com.
const cacheEntry = { id: 'e1', name: 'Example', url: 'https://example.com', match_patterns: ['example.com'] };

describe('credential save flow', () => {
  beforeEach(() => {
    for (const k of Object.keys(sessionStore)) delete sessionStore[k];
    for (const k of Object.keys(localStore)) delete localStore[k];
    createEntryMock.mockClear();
    encryptEntryMock.mockClear();
    decryptEntryMock.mockReset();
    updateEntryMock.mockClear();
    getEntryMock.mockClear();
    getEntriesCacheMock.mockReset();
    getEntriesCacheMock.mockReturnValue(undefined); // no existing entries by default
  });

  it('OFFER_SAVE stashes a pending save that GET_PENDING_SAVE returns', async () => {
    const handlers = buildHandlers();
    await handlers[MessageType.OFFER_SAVE]({
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'pw',
    });

    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeDefined();
    const pending = (await handlers[MessageType.GET_PENDING_SAVE]({})) as { username: string } | null;
    expect(pending?.username).toBe('alice');
  });

  it('OFFER_SAVE skips a host the user previously dismissed', async () => {
    const handlers = buildHandlers();
    sessionStore[STORAGE_KEYS.DISMISSED_SAVE_HOSTS] = ['example.com'];

    const res = (await handlers[MessageType.OFFER_SAVE]({
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'pw',
    })) as { ok?: boolean; dismissed?: boolean };

    expect(res.dismissed).toBe(true);
    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeUndefined();
  });

  it('CONFIRM_SAVE creates the entry and clears the pending save', async () => {
    const handlers = buildHandlers();
    await handlers[MessageType.OFFER_SAVE]({
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'pw',
    });

    const res = (await handlers[MessageType.CONFIRM_SAVE]({})) as { ok?: boolean };
    expect(res.ok).toBe(true);
    expect(createEntryMock).toHaveBeenCalledTimes(1);
    // pending cleared
    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeUndefined();
  });

  it('OFFER_SAVE is skipped for a blocklisted host (and its subdomains)', async () => {
    const handlers = buildHandlers();
    await handlers[MessageType.BLOCKLIST_ADD]({ host: 'example.com' });

    const res = (await handlers[MessageType.OFFER_SAVE]({
      name: 'Example', url: 'https://login.example.com/login', username: 'a', password: 'p',
    })) as { ok?: boolean; blocked?: boolean };

    expect(res.blocked).toBe(true);
    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeUndefined();

    const { list } = (await handlers[MessageType.BLOCKLIST_GET]({})) as { list: string[] };
    expect(list).toContain('example.com');
  });

  it('DISMISS_SAVE clears the pending save and remembers the host', async () => {
    const handlers = buildHandlers();
    await handlers[MessageType.OFFER_SAVE]({
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'pw',
    });

    await handlers[MessageType.DISMISS_SAVE]({ host: 'example.com' });
    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeUndefined();
    expect(sessionStore[STORAGE_KEYS.DISMISSED_SAVE_HOSTS]).toContain('example.com');
  });

  it('OFFER_SAVE returns unchanged (no offer) when an identical entry exists', async () => {
    const handlers = buildHandlers();
    getEntriesCacheMock.mockReturnValue([cacheEntry]);
    decryptEntryMock.mockResolvedValue({ username: 'alice', password: 'pw' });

    const res = (await handlers[MessageType.OFFER_SAVE]({
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'pw',
    })) as { ok?: boolean; unchanged?: boolean };

    expect(res.unchanged).toBe(true);
    expect(res.ok).toBeFalsy();
    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeUndefined();
  });

  it('OFFER_SAVE suggests updating the entry when the password changed', async () => {
    const handlers = buildHandlers();
    getEntriesCacheMock.mockReturnValue([cacheEntry]);
    decryptEntryMock.mockResolvedValue({ username: 'alice', password: 'oldpw' });

    const res = (await handlers[MessageType.OFFER_SAVE]({
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'newpw',
    })) as { ok?: boolean; candidates?: { id: string; username: string }[]; suggestUpdateId?: string };

    expect(res.ok).toBe(true);
    expect(res.candidates).toEqual([{ id: 'e1', username: 'alice' }]);
    expect(res.suggestUpdateId).toBe('e1');
    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeDefined();
  });

  it('OFFER_SAVE offers a new entry (no update suggestion) for a new username', async () => {
    const handlers = buildHandlers();
    getEntriesCacheMock.mockReturnValue([cacheEntry]);
    decryptEntryMock.mockResolvedValue({ username: 'bob', password: 'x' });

    const res = (await handlers[MessageType.OFFER_SAVE]({
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'pw',
    })) as { ok?: boolean; candidates?: { id: string; username: string }[]; suggestUpdateId?: string };

    expect(res.ok).toBe(true);
    expect(res.candidates).toEqual([{ id: 'e1', username: 'bob' }]);
    expect(res.suggestUpdateId).toBeUndefined();
  });

  it('UPDATE_SAVE re-encrypts the pending credentials and updates the entry', async () => {
    const handlers = buildHandlers();
    sessionStore[STORAGE_KEYS.PENDING_SAVE] = {
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'newpw',
    };

    const res = (await handlers[MessageType.UPDATE_SAVE]({ entryId: 'e1' })) as { ok?: boolean; id?: string };

    expect(res.ok).toBe(true);
    expect(encryptEntryMock).toHaveBeenCalledTimes(1);
    expect(updateEntryMock).toHaveBeenCalledWith('e1', {
      encrypted_data: 'ed',
      data_nonce: 'dn',
      entry_keys: [{ user_id: 'u', encrypted_key: 'ek' }],
    });
    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeUndefined();
  });
});

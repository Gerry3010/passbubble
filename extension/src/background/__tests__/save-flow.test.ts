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
const reencryptEntryMock = vi.hoisted(() =>
  vi.fn().mockResolvedValue({
    encrypted_data: 'ed',
    data_nonce: 'dn',
  }),
);
const decryptEntryMock = vi.hoisted(() => vi.fn());
const getEntryMock = vi.hoisted(() => vi.fn(async (id: string) => ({ id })));
const updateEntryMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const listEntriesMock = vi.hoisted(() => vi.fn().mockResolvedValue([]));
const getEntriesCacheMock = vi.hoisted(() => vi.fn());
const clearEntryTagsCacheMock = vi.hoisted(() => vi.fn());

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
  readStoredSession: vi.fn(async () => null),
  adoptSession: vi.fn(),
  getEntriesCache: getEntriesCacheMock,
  setEntriesCache: vi.fn(),
  clearEntryTagsCache: clearEntryTagsCacheMock,
}));

vi.mock('@passbubble/shared-ts', async (importOriginal) => ({
  dropStrayTotpSecret: (await importOriginal<typeof import('@passbubble/shared-ts')>()).dropStrayTotpSecret,
  PassbubbleClient: vi.fn().mockImplementation(() => ({
    setTokens: vi.fn(),
    listEntries: listEntriesMock,
    getEntry: getEntryMock,
    updateEntry: updateEntryMock,
  })),
  unlock: vi.fn(),
  decryptEntry: decryptEntryMock,
  reencryptEntry: reencryptEntryMock,
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
    reencryptEntryMock.mockClear();
    decryptEntryMock.mockReset();
    updateEntryMock.mockClear();
    getEntryMock.mockClear();
    getEntriesCacheMock.mockReset();
    getEntriesCacheMock.mockReturnValue(undefined); // no existing entries by default
    clearEntryTagsCacheMock.mockClear();
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

  it('UPDATE_SAVE merges the pending credentials into the existing data and updates the entry', async () => {
    const handlers = buildHandlers();
    // The entry already carries fields beyond username/password — the update
    // must preserve them, not overwrite the whole data blob.
    decryptEntryMock.mockResolvedValue({
      username: 'alice', password: 'oldpw', totp_secret: 'JBSWY3DP', sign_in_with: 'google',
    });
    sessionStore[STORAGE_KEYS.PENDING_SAVE] = {
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'newpw',
    };

    const res = (await handlers[MessageType.UPDATE_SAVE]({ entryId: 'e1' })) as { ok?: boolean; id?: string };

    expect(res.ok).toBe(true);
    expect(reencryptEntryMock).toHaveBeenCalledTimes(1);
    expect(reencryptEntryMock.mock.calls[0][0]).toEqual({
      username: 'alice', password: 'newpw', totp_secret: 'JBSWY3DP', sign_in_with: 'google',
    });
    // No entry_keys: the data key stays, so every recipient's wrapped key stays valid.
    expect(updateEntryMock).toHaveBeenCalledWith('e1', {
      encrypted_data: 'ed',
      data_nonce: 'dn',
    });
    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeUndefined();
  });

  it('UPDATE_SAVE keeps tags, custom-field keys and keys it does not know', async () => {
    const handlers = buildHandlers();
    const existing = {
      username: 'alice',
      password: 'oldpw',
      tags: ['work', 'client'],
      custom_fields: [
        { label: 'Contract', value: 'Y29udHJhY3Q=', type: 'file', filename: 'contract.pdf', mime_type: 'application/pdf' },
      ],
      written_by_a_newer_app: { nested: true },
    };
    decryptEntryMock.mockResolvedValue(structuredClone(existing));
    sessionStore[STORAGE_KEYS.PENDING_SAVE] = {
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'newpw',
    };

    const res = (await handlers[MessageType.UPDATE_SAVE]({ entryId: 'e1' }, {})) as { ok?: boolean };

    expect(res.ok).toBe(true);
    expect(reencryptEntryMock.mock.calls[0][0]).toEqual({ ...existing, password: 'newpw' });
    expect(clearEntryTagsCacheMock).toHaveBeenCalled();
  });

  describe('UPDATE_SAVE and a stray totp_secret', () => {
    const pendingNewPassword = () => {
      sessionStore[STORAGE_KEYS.PENDING_SAVE] = {
        name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'newpw',
      };
    };

    it('drops a copy of the old password, which would otherwise pass for a secret', async () => {
      const handlers = buildHandlers();
      decryptEntryMock.mockResolvedValue({ username: 'alice', password: 'oldpw', totp_secret: 'oldpw' });
      pendingNewPassword();

      await handlers[MessageType.UPDATE_SAVE]({ entryId: 'e1' }, {});

      expect(reencryptEntryMock.mock.calls[0][0]).toEqual({ username: 'alice', password: 'newpw' });
    });

    it('keeps a real secret', async () => {
      const handlers = buildHandlers();
      decryptEntryMock.mockResolvedValue({ username: 'alice', password: 'oldpw', totp_secret: 'JBSWY3DP' });
      pendingNewPassword();

      await handlers[MessageType.UPDATE_SAVE]({ entryId: 'e1' }, {});

      expect(reencryptEntryMock.mock.calls[0][0]).toEqual({ username: 'alice', password: 'newpw', totp_secret: 'JBSWY3DP' });
    });
  });

  it('UPDATE_SAVE leaves the entry in its folder', async () => {
    const handlers = buildHandlers();
    getEntryMock.mockResolvedValueOnce({ id: 'e1', folder_id: 'f-work' } as never);
    decryptEntryMock.mockResolvedValue({ username: 'alice', password: 'oldpw' });
    sessionStore[STORAGE_KEYS.PENDING_SAVE] = {
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'newpw',
    };

    await handlers[MessageType.UPDATE_SAVE]({ entryId: 'e1' }, {});

    // The backend always overwrites folder_id, so it must be sent back.
    expect(updateEntryMock).toHaveBeenCalledWith('e1', expect.objectContaining({ folder_id: 'f-work' }));
  });

  it('UPDATE_SAVE refuses to overwrite an entry it cannot read', async () => {
    const handlers = buildHandlers();
    decryptEntryMock.mockRejectedValue(new Error('OperationError'));
    sessionStore[STORAGE_KEYS.PENDING_SAVE] = {
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'newpw',
    };

    const res = (await handlers[MessageType.UPDATE_SAVE]({ entryId: 'e1' }, {})) as { ok?: boolean; error?: string };

    expect(res.ok).toBe(false);
    expect(res.error).toMatch(/nothing was changed/);
    expect(reencryptEntryMock).not.toHaveBeenCalled();
    expect(updateEntryMock).not.toHaveBeenCalled();
    // The offer survives, so the user can still save it as a new entry.
    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeDefined();
  });
});

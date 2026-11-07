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

// Tags in the background: the LIST_ENTRY_TAGS bulk decrypt + its cache, tag
// normalisation on create, and the SSO write path keeping data it does not
// manage. Uses the real session store and vault client; only the browser API,
// the HTTP client and the entry crypto are mocked (the tag helpers are real).

import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { EntryData, EntryResponse, UnlockedSession } from '@passbubble/shared-ts';

vi.mock('webextension-polyfill', () => ({
  default: {
    storage: {
      sync: { get: vi.fn().mockResolvedValue({}), set: vi.fn() },
      session: { get: vi.fn().mockResolvedValue({}), set: vi.fn(), remove: vi.fn() },
      local: { get: vi.fn().mockResolvedValue({}), set: vi.fn(), remove: vi.fn() },
    },
    alarms: { create: vi.fn(), clear: vi.fn() },
  },
}));

const client = vi.hoisted(() => ({
  setTokens: vi.fn(),
  listEntries: vi.fn(),
  listEntriesFull: vi.fn(),
  getEntry: vi.fn(),
  updateEntry: vi.fn(),
}));
const decryptEntryMock = vi.hoisted(() => vi.fn());
const reencryptEntryMock = vi.hoisted(() => vi.fn());
const createEntryMock = vi.hoisted(() => vi.fn());

vi.mock('@passbubble/shared-ts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@passbubble/shared-ts')>()),
  PassbubbleClient: vi.fn().mockImplementation(() => client),
  decryptEntry: decryptEntryMock,
  reencryptEntry: reencryptEntryMock,
  createEntry: createEntryMock,
}));

import { buildHandlers } from '../message-handler.js';
import { persistSsoToEntries } from '../sso-entry.js';
import { clearSession, getEntryTagsCache, setSession } from '../session-store.js';
import { lockVault } from '../vault-client.js';
import { MessageType } from '../../shared/constants.js';

function makeSession(): UnlockedSession {
  return {
    privX25519: new Uint8Array(32),
    privMLKEM: new Uint8Array(64),
    pubX25519: new Uint8Array(32),
    pubMLKEM: new Uint8Array(1184),
    userId: 'u1',
    userEmail: 'alice@example.com',
    userName: 'Alice',
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

function entry(id: string, updatedAt = '2026-10-01T00:00:00Z', extra: Partial<EntryResponse> = {}): EntryResponse {
  return { id, owner_id: 'u1', type: 'password', name: id, created_at: '', updated_at: updatedAt, ...extra };
}

// Decrypted data per entry id; an id mapped to an Error fails to decrypt.
let vaultData: Record<string, EntryData | Error> = {};

describe('entry tags in the background', () => {
  let handlers: ReturnType<typeof buildHandlers>;

  beforeEach(async () => {
    await clearSession();
    await setSession(makeSession());
    vaultData = {
      a: { username: 'alice', tags: ['#Work ', 'work', 3 as unknown as string, 'Client'] },
      b: { username: 'bob' },
      c: new Error('OperationError'),
    };
    const listing = [entry('a'), entry('b'), entry('c')];
    client.listEntries.mockReset().mockResolvedValue(listing);
    client.listEntriesFull.mockReset().mockResolvedValue(listing);
    client.getEntry.mockReset().mockImplementation(async (id: string) => entry(id));
    client.updateEntry.mockReset().mockResolvedValue(undefined);
    decryptEntryMock.mockReset().mockImplementation(async (e: EntryResponse) => {
      const data = vaultData[e.id];
      if (data instanceof Error) throw data;
      return structuredClone(data ?? {});
    });
    reencryptEntryMock.mockReset().mockResolvedValue({
      encrypted_data: 'ed',
      data_nonce: 'dn',
    });
    createEntryMock.mockReset().mockResolvedValue({ id: 'new' });
    handlers = buildHandlers();
  });

  describe('LIST_ENTRY_TAGS', () => {
    it('answers { locked: true } while the vault is locked', async () => {
      await lockVault('manual');
      expect(await handlers[MessageType.LIST_ENTRY_TAGS]({}, {})).toEqual({ locked: true });
      expect(client.listEntriesFull).not.toHaveBeenCalled();
    });

    it('returns the normalised tags per entry, leaving out untagged and unreadable entries', async () => {
      const res = await handlers[MessageType.LIST_ENTRY_TAGS]({}, {});
      expect(res).toEqual({ tags: { a: ['Work', 'Client'] } });
    });

    it('serves repeated requests from the cache while the listing is unchanged', async () => {
      await handlers[MessageType.LIST_ENTRY_TAGS]({}, {});
      await handlers[MessageType.SEARCH_ENTRIES]({ query: '' }, {}); // popup reload, same listing
      const res = await handlers[MessageType.LIST_ENTRY_TAGS]({}, {});

      expect(res).toEqual({ tags: { a: ['Work', 'Client'] } });
      expect(client.listEntriesFull).toHaveBeenCalledTimes(1);
    });

    it('decrypts again once the listing shows a rewritten entry (e.g. edited on another device)', async () => {
      await handlers[MessageType.LIST_ENTRY_TAGS]({}, {});

      vaultData.b = { username: 'bob', tags: ['home'] };
      const changed = [entry('a'), entry('b', '2026-10-31T10:00:00Z'), entry('c')];
      client.listEntries.mockResolvedValue(changed);
      client.listEntriesFull.mockResolvedValue(changed);
      await handlers[MessageType.SEARCH_ENTRIES]({ query: '' }, {});

      const res = await handlers[MessageType.LIST_ENTRY_TAGS]({}, {});
      expect(res).toEqual({ tags: { a: ['Work', 'Client'], b: ['home'] } });
      expect(client.listEntriesFull).toHaveBeenCalledTimes(2);
    });

    it('drops the cache on lock', async () => {
      await handlers[MessageType.LIST_ENTRY_TAGS]({}, {});
      expect(getEntryTagsCache()).not.toBeNull();

      await handlers[MessageType.LOCK]({}, {});
      expect(getEntryTagsCache()).toBeNull();

      await setSession(makeSession());
      await handlers[MessageType.LIST_ENTRY_TAGS]({}, {});
      expect(client.listEntriesFull).toHaveBeenCalledTimes(2);
    });

    it('does not refill the cache when the vault locks mid-decrypt', async () => {
      decryptEntryMock.mockImplementationOnce(async () => {
        await lockVault('manual');
        return { tags: ['work'] };
      });
      await handlers[MessageType.LIST_ENTRY_TAGS]({}, {});
      expect(getEntryTagsCache()).toBeNull();
    });
  });

  describe('CREATE_ENTRY', () => {
    it('normalises the tags before encrypting', async () => {
      await handlers[MessageType.CREATE_ENTRY](
        { name: 'GitHub', type: 'password', data: { username: 'octocat', tags: [' #Work', 'work', '', 'my   team'] } },
        {},
      );
      const data = createEntryMock.mock.calls[0][4] as EntryData;
      expect(data).toEqual({ username: 'octocat', tags: ['Work', 'my team'] });
    });

    it('omits the tags key when no tag is left', async () => {
      await handlers[MessageType.CREATE_ENTRY](
        { name: 'GitHub', type: 'password', data: { username: 'octocat', tags: ['  ', '#'] } },
        {},
      );
      const data = createEntryMock.mock.calls[0][4] as EntryData;
      expect(data).toEqual({ username: 'octocat' });
      expect('tags' in data).toBe(false);
    });

    it('invalidates the tags cache even when the listing looks the same', async () => {
      await handlers[MessageType.LIST_ENTRY_TAGS]({}, {});
      await handlers[MessageType.CREATE_ENTRY]({ name: 'X', type: 'note', data: { notes: 'n' } }, {});
      await handlers[MessageType.LIST_ENTRY_TAGS]({}, {});
      expect(client.listEntriesFull).toHaveBeenCalledTimes(2);
    });
  });

  describe('persistSsoToEntries', () => {
    const stored = {
      username: 'alice',
      password: 'pw',
      tags: ['work'],
      custom_fields: [
        { label: 'Contract', value: 'Y29udHJhY3Q=', type: 'file', filename: 'contract.pdf', mime_type: 'application/pdf' },
      ],
      written_by_a_newer_app: { nested: [1, 2] },
    };

    beforeEach(() => {
      vaultData = { s: structuredClone(stored) as EntryData };
      const listing = [entry('s', '2026-10-01T00:00:00Z', { match_patterns: ['example.com'] })];
      client.listEntries.mockResolvedValue(listing);
      client.getEntry.mockImplementation(async (id: string) =>
        entry(id, '2026-10-01T00:00:00Z', { match_patterns: ['example.com'], folder_id: 'f-work' }),
      );
    });

    it('changes only sign_in_with and keeps tags, custom-field keys and unknown keys', async () => {
      await persistSsoToEntries('example.com', 'google');
      expect(reencryptEntryMock).toHaveBeenCalledTimes(1);
      expect(reencryptEntryMock.mock.calls[0][0]).toEqual({ ...stored, sign_in_with: 'google' });
    });

    it('leaves the entry in its folder and drops the tags cache', async () => {
      await handlers[MessageType.LIST_ENTRY_TAGS]({}, {});
      expect(getEntryTagsCache()).not.toBeNull();

      await persistSsoToEntries('example.com', 'google');
      expect(client.updateEntry).toHaveBeenCalledWith('s', expect.objectContaining({ folder_id: 'f-work' }));
      expect(getEntryTagsCache()).toBeNull();
    });

    it('clearing the provider keeps everything else as well', async () => {
      vaultData.s = { ...structuredClone(stored), sign_in_with: 'github' } as EntryData;
      await persistSsoToEntries('example.com', null);
      expect(reencryptEntryMock.mock.calls[0][0]).toEqual(stored);
    });
  });
});

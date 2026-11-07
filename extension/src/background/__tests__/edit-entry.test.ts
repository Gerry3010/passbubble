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

// EDIT_ENTRY in the background. Uses the real session store and vault client;
// only the browser API, the HTTP client and the entry crypto are mocked. The
// real crypto path is covered by entry-key-reuse.test.ts.

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
  setFavorite: vi.fn(),
}));
const decryptEntryMock = vi.hoisted(() => vi.fn());
const reencryptEntryMock = vi.hoisted(() => vi.fn());
const encryptEntryMock = vi.hoisted(() => vi.fn());

vi.mock('@passbubble/shared-ts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@passbubble/shared-ts')>()),
  PassbubbleClient: vi.fn().mockImplementation(() => client),
  decryptEntry: decryptEntryMock,
  reencryptEntry: reencryptEntryMock,
  encryptEntry: encryptEntryMock,
}));

import { buildHandlers } from '../message-handler.js';
import { persistSsoToEntries } from '../sso-entry.js';
import { clearSession, getEntriesCache, getEntryTagsCache, setEntryTagsCache, setSession } from '../session-store.js';
import { lockVault, VaultLockedError } from '../vault-client.js';
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

const API_ENTRY: EntryResponse = {
  id: 'e1',
  owner_id: 'u1',
  type: 'password',
  name: 'Old name',
  url: 'https://old.example',
  folder_id: 'f-work',
  match_patterns: ['old.example'],
  favorite: true,
  created_at: '',
  updated_at: '2026-10-01T00:00:00Z',
  encrypted_data: 'blob',
  entry_key: { user_id: 'u1', encrypted_key: 'wrapped' },
};

const STORED = {
  username: 'alice',
  password: 'old',
  notes: 'keep',
  period: 60,
  tags: ['work'],
  custom_fields: [{ label: 'File', value: 'Zm9v', type: 'file', filename: 'foo.txt', mime_type: 'text/plain' }],
  written_by_a_newer_app: { nested: [1, 2] },
};

describe('EDIT_ENTRY', () => {
  let handlers: ReturnType<typeof buildHandlers>;

  const edit = (payload: Record<string, unknown>) =>
    handlers[MessageType.EDIT_ENTRY]({ entryId: 'e1', name: 'Old name', url: 'https://old.example', ...payload }, {}) as Promise<{
      ok?: boolean;
      error?: string;
      locked?: boolean;
      loggedOut?: boolean;
    }>;

  const written = () => reencryptEntryMock.mock.calls[0][0] as EntryData & Record<string, unknown>;

  beforeEach(async () => {
    await clearSession();
    await setSession(makeSession());
    client.getEntry.mockReset().mockResolvedValue(API_ENTRY);
    client.updateEntry.mockReset().mockResolvedValue(undefined);
    client.listEntries.mockReset().mockResolvedValue([{ ...API_ENTRY, name: 'refreshed' }]);
    decryptEntryMock.mockReset().mockImplementation(async () => structuredClone(STORED));
    reencryptEntryMock.mockReset().mockResolvedValue({ encrypted_data: 'ed', data_nonce: 'dn' });
    encryptEntryMock.mockReset();
    handlers = buildHandlers();
  });

  it('answers like other handlers while the vault is locked', async () => {
    await lockVault('manual');
    expect(await edit({ fields: { password: 'new' } })).toEqual({ locked: true });
    expect(client.getEntry).not.toHaveBeenCalled();
    expect(client.updateEntry).not.toHaveBeenCalled();
  });

  it('answers locked when the vault turns out to be locked mid-request', async () => {
    client.getEntry.mockRejectedValue(new VaultLockedError(false));
    expect(await edit({ fields: { password: 'new' } })).toEqual({ locked: true, loggedOut: false });

    client.getEntry.mockResolvedValue(API_ENTRY);
    client.updateEntry.mockRejectedValue(new VaultLockedError(true));
    expect(await edit({ fields: { password: 'new' } })).toEqual({ locked: true, loggedOut: true });
  });

  it('applies the change to the current data and keeps everything else', async () => {
    expect(await edit({ fields: { password: 'new' } })).toEqual({ ok: true });
    expect(written()).toEqual({ ...STORED, password: 'new' });
  });

  it('encrypts with the existing data key, never a fresh one', async () => {
    await edit({ fields: { password: 'new' } });
    expect(reencryptEntryMock).toHaveBeenCalledTimes(1);
    expect(reencryptEntryMock.mock.calls[0][1]).toBe(API_ENTRY);
    expect(encryptEntryMock).not.toHaveBeenCalled();
  });

  it('echoes the folder and leaves match patterns, favorite and entry keys out of the request', async () => {
    await edit({ name: 'New name', url: 'https://new.example', fields: { password: 'new' } });

    expect(client.updateEntry).toHaveBeenCalledTimes(1);
    expect(client.updateEntry).toHaveBeenCalledWith('e1', {
      encrypted_data: 'ed',
      data_nonce: 'dn',
      name: 'New name',
      url: 'https://new.example',
      folder_id: 'f-work',
    });
    expect(client.setFavorite).not.toHaveBeenCalled();
  });

  it('sends no folder for an entry at the root', async () => {
    client.getEntry.mockResolvedValue({ ...API_ENTRY, folder_id: undefined });
    await edit({ fields: { password: 'new' } });
    expect(client.updateEntry.mock.calls[0][1].folder_id).toBeUndefined();
  });

  it('removes a key whose value was removed instead of storing an empty string', async () => {
    await edit({ fields: { notes: '', period: null } });
    expect(written()).not.toHaveProperty('notes');
    expect(written()).not.toHaveProperty('period');
    expect(written().username).toBe('alice');
  });

  it('replaces custom fields and tags only when they are sent', async () => {
    await edit({ fields: { password: 'new' } });
    expect(written().custom_fields).toEqual(STORED.custom_fields);
    expect(written().tags).toEqual(['work']);

    reencryptEntryMock.mockClear();
    const list = [{ label: 'New', value: 'v', type: 'text' }];
    await edit({ customFields: list, tags: ['#Home ', 'home', 'b'] });
    expect(written().custom_fields).toEqual(list);
    expect(written().tags).toEqual(['Home', 'b']);
  });

  it('removes the custom_fields and tags keys when emptied', async () => {
    await edit({ customFields: [], tags: [] });
    expect(written()).not.toHaveProperty('custom_fields');
    expect(written()).not.toHaveProperty('tags');
  });

  describe('url and name in the request', () => {
    const request = () => client.updateEntry.mock.calls[0][1] as Record<string, unknown>;

    it('leaves out what the form did not send, which keeps it on the server', async () => {
      await handlers[MessageType.EDIT_ENTRY]({ entryId: 'e1', fields: { password: 'new' } }, {});
      expect(request()).not.toHaveProperty('url');
      expect(request()).not.toHaveProperty('name');
    });

    it('sends an empty url, which clears it', async () => {
      await handlers[MessageType.EDIT_ENTRY]({ entryId: 'e1', url: '' }, {});
      expect(request().url).toBe('');
    });

    it('sends a changed url and name', async () => {
      await handlers[MessageType.EDIT_ENTRY]({ entryId: 'e1', name: ' New ', url: 'https://new.example' }, {});
      expect(request()).toMatchObject({ name: 'New', url: 'https://new.example' });
    });

    it('UPDATE_ENTRY (match patterns) does not touch the url', async () => {
      await handlers[MessageType.UPDATE_ENTRY]({ entryId: 'e1', matchPatterns: ['a.example'], folderId: 'f-work' }, {});
      expect(request()).toEqual({ folder_id: 'f-work', match_patterns: ['a.example'] });
    });

    it('the SSO write path does not touch the url', async () => {
      client.listEntries.mockResolvedValue([{ ...API_ENTRY, match_patterns: ['old.example'] }]);
      await persistSsoToEntries('old.example', 'google');
      expect(request()).not.toHaveProperty('url');
      expect(request()).not.toHaveProperty('name');
    });
  });

  describe('a stray totp_secret', () => {
    it('is not written back by an edit that does not touch it', async () => {
      decryptEntryMock.mockImplementation(async () => ({ username: 'a', password: 'p', totp_secret: 'p' }));
      await edit({ fields: { notes: 'n' } });
      expect(written()).toEqual({ username: 'a', password: 'p', notes: 'n' });
    });

    it('does not turn into a secret when the password changes', async () => {
      decryptEntryMock.mockImplementation(async () => ({ password: 'p', totp_secret: 'p' }));
      await edit({ fields: { password: 'new' } });
      expect(written()).toEqual({ password: 'new' });
    });

    it('gives way to a secret the user enters', async () => {
      decryptEntryMock.mockImplementation(async () => ({ password: 'p', totp_secret: 'p' }));
      await edit({ fields: { totp_secret: 'JBSWY3DP' } });
      expect(written()).toEqual({ password: 'p', totp_secret: 'JBSWY3DP' });
    });

    it('a real secret stays untouched, also when the password changes', async () => {
      decryptEntryMock.mockImplementation(async () => ({ password: 'p', totp_secret: 'JBSWY3DP', period: 60 }));
      await edit({ fields: { password: 'new' } });
      expect(written()).toEqual({ password: 'new', totp_secret: 'JBSWY3DP', period: 60 });
    });
  });

  it('keeps the name and url copies the app stores inside the data in step', async () => {
    decryptEntryMock.mockImplementation(async () => ({ name: 'Old name', url: 'https://old.example', username: 'a' }));
    await edit({ name: 'New name', url: '' });
    expect(written()).toEqual({ name: 'New name', username: 'a' });
  });

  it('leaves those copies alone when name and url did not change', async () => {
    decryptEntryMock.mockImplementation(async () => ({ name: 'Copy', url: 'https://copy.example', username: 'a' }));
    await handlers[MessageType.EDIT_ENTRY]({ entryId: 'e1', fields: { username: 'b' } }, {});
    expect(written()).toEqual({ name: 'Copy', url: 'https://copy.example', username: 'b' });
  });

  it('does not add name or url to data that has none', async () => {
    await edit({ name: 'New name', url: 'https://new.example' });
    expect(written()).not.toHaveProperty('name');
    expect(written()).not.toHaveProperty('url');
  });

  it('refreshes the entries cache and drops the tags cache', async () => {
    setEntryTagsCache({ stamp: 'x', tags: { e1: ['work'] } });
    await edit({ tags: ['home'] });

    expect(getEntryTagsCache()).toBeNull();
    expect(client.listEntries).toHaveBeenCalled();
    expect(getEntriesCache()?.[0].name).toBe('refreshed');
  });

  it('still reports success when only the cache refresh fails', async () => {
    client.listEntries.mockRejectedValue(new TypeError('offline'));
    expect(await edit({ fields: { password: 'new' } })).toEqual({ ok: true });
  });

  it('overwrites nothing when the entry cannot be read', async () => {
    decryptEntryMock.mockRejectedValue(new Error('OperationError'));
    const res = await edit({ fields: { password: 'new' } });
    expect(res.ok).toBe(false);
    expect(res.error).toMatch(/nothing was changed/);
    expect(client.updateEntry).not.toHaveBeenCalled();
  });

  it('overwrites nothing when the data key cannot be unwrapped', async () => {
    reencryptEntryMock.mockRejectedValue(new Error('OperationError'));
    const res = await edit({ fields: { password: 'new' } });
    expect(res.ok).toBe(false);
    expect(res.error).toMatch(/nothing was changed/);
    expect(client.updateEntry).not.toHaveBeenCalled();
  });

  it('explains a read-only share, and reports other failures', async () => {
    client.updateEntry.mockRejectedValue(Object.assign(new Error('insufficient permissions'), { status: 403 }));
    expect((await edit({ fields: { password: 'new' } })).error).toMatch(/read access/);

    client.updateEntry.mockRejectedValue(new TypeError('Failed to fetch'));
    const res = await edit({ fields: { password: 'new' } });
    expect(res.ok).toBe(false);
    expect(res.error).toMatch(/Failed to fetch/);
  });

  it('refuses a blank name or a missing entry id', async () => {
    expect((await edit({ name: '   ' })).ok).toBe(false);
    expect(((await handlers[MessageType.EDIT_ENTRY]({ name: 'x', url: '' }, {})) as { ok?: boolean }).ok).toBe(false);
    expect(client.updateEntry).not.toHaveBeenCalled();
  });
});

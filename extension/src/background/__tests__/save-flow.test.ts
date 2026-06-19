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

// In-memory session storage so OFFER_SAVE → GET_PENDING_SAVE → CONFIRM_SAVE work.
const sessionStore: Record<string, unknown> = {};

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
        set: vi.fn(async (obj: Record<string, unknown>) => {
          Object.assign(sessionStore, obj);
        }),
        remove: vi.fn(async (k: string) => {
          delete sessionStore[k];
        }),
      },
    },
    alarms: { create: vi.fn(), clear: vi.fn() },
  },
}));

const createEntryMock = vi.hoisted(() => vi.fn().mockResolvedValue({ id: 'e1' }));

vi.mock('../session-store.js', () => ({
  getSession: vi.fn(() => ({ serverUrl: 'https://srv', accessToken: 'at' })),
  setSession: vi.fn(),
  clearSession: vi.fn(),
  getEntriesCache: vi.fn(),
  setEntriesCache: vi.fn(),
}));

vi.mock('@passbubble/shared-ts', () => ({
  PassbubbleClient: vi.fn().mockImplementation(() => ({
    setTokens: vi.fn(),
    listEntries: vi.fn().mockResolvedValue([]),
  })),
  unlock: vi.fn(),
  decryptEntry: vi.fn(),
  encryptEntry: vi.fn(),
  createEntry: createEntryMock,
}));

import { buildHandlers } from '../message-handler.js';
import { MessageType, STORAGE_KEYS } from '../../shared/constants.js';

describe('credential save flow', () => {
  beforeEach(() => {
    for (const k of Object.keys(sessionStore)) delete sessionStore[k];
    createEntryMock.mockClear();
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

  it('DISMISS_SAVE clears the pending save and remembers the host', async () => {
    const handlers = buildHandlers();
    await handlers[MessageType.OFFER_SAVE]({
      name: 'Example', url: 'https://example.com/login', username: 'alice', password: 'pw',
    });

    await handlers[MessageType.DISMISS_SAVE]({ host: 'example.com' });
    expect(sessionStore[STORAGE_KEYS.PENDING_SAVE]).toBeUndefined();
    expect(sessionStore[STORAGE_KEYS.DISMISSED_SAVE_HOSTS]).toContain('example.com');
  });
});

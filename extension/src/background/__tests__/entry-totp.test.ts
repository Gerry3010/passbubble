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

// TOTP codes in the background: FILL_ENTRY and GET_TOTP_FOR_URL honour the stored
// parameters and ignore a stray totp_secret. The code generation itself is real;
// the browser API, the HTTP client and the entry decryption are mocked.

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

const client = vi.hoisted(() => ({ setTokens: vi.fn(), listEntries: vi.fn(), getEntry: vi.fn() }));
const decryptEntryMock = vi.hoisted(() => vi.fn());

vi.mock('@passbubble/shared-ts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@passbubble/shared-ts')>()),
  PassbubbleClient: vi.fn().mockImplementation(() => client),
  decryptEntry: decryptEntryMock,
}));

import { generateTotp } from '@passbubble/shared-ts';
import { buildHandlers } from '../message-handler.js';
import { clearSession, setEntriesCache, setSession } from '../session-store.js';
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

const SECRET = 'JBSWY3DPEHPK3PXP';
// An hour-long period, so the expected code stays valid during the test.
const STORED = { period: 3600, digits: 8, algorithm: 'SHA256' };

function entry(id: string): EntryResponse {
  return {
    id,
    owner_id: 'u1',
    type: 'password',
    name: `Entry ${id}`,
    url: 'https://example.com',
    match_patterns: ['example.com'],
    created_at: '',
    updated_at: '',
  };
}

describe('TOTP in the background', () => {
  let handlers: ReturnType<typeof buildHandlers>;
  let vault: Record<string, EntryData>;

  const fill = (entryId: string) =>
    handlers[MessageType.FILL_ENTRY]({ entryId }, {}) as Promise<{ username: string; password: string; totpCode: string }>;
  const forUrl = () =>
    handlers[MessageType.GET_TOTP_FOR_URL]({ url: 'https://example.com/2fa' }, {}) as Promise<{
      code: string;
      remainingSeconds?: number;
      entryName?: string;
    }>;

  function serve(data: Record<string, EntryData>) {
    vault = data;
    setEntriesCache(Object.keys(data).map(entry));
  }

  beforeEach(async () => {
    await clearSession();
    await setSession(makeSession());
    client.getEntry.mockReset().mockImplementation(async (id: string) => entry(id));
    decryptEntryMock.mockReset().mockImplementation(async (e: EntryResponse) => structuredClone(vault[e.id]));
    handlers = buildHandlers();
  });

  describe('FILL_ENTRY', () => {
    it("uses the entry's stored period, digits and algorithm", async () => {
      serve({ a: { username: 'alice', password: 'pw', totp_secret: SECRET, ...STORED } });
      const res = await fill('a');
      expect(res.totpCode).toBe((await generateTotp(SECRET, STORED)).code);
      expect(res.totpCode).toHaveLength(8);
    });

    it('hands no code along for a copy of the password', async () => {
      serve({ a: { username: 'alice', password: SECRET, totp_secret: SECRET } });
      expect(await fill('a')).toEqual({ username: 'alice', password: SECRET, totpCode: '' });
    });

    it('still fills when the algorithm is unknown, without a code', async () => {
      serve({ a: { username: 'alice', password: 'pw', totp_secret: SECRET, algorithm: 'MD5' } });
      expect(await fill('a')).toEqual({ username: 'alice', password: 'pw', totpCode: '' });
    });

    it('a real secret keeps working', async () => {
      serve({ a: { username: 'alice', password: 'pw', totp_secret: SECRET } });
      expect((await fill('a')).totpCode).toBe((await generateTotp(SECRET)).code);
    });
  });

  describe('GET_TOTP_FOR_URL', () => {
    it("uses the entry's stored period, digits and algorithm", async () => {
      serve({ a: { password: 'pw', totp_secret: SECRET, ...STORED } });
      const res = await forUrl();
      const expected = await generateTotp(SECRET, STORED);
      expect(res.code).toBe(expected.code);
      expect(res.remainingSeconds).toBeGreaterThan(30);
    });

    it('skips a stray secret and moves on to the next match', async () => {
      serve({
        stray: { password: SECRET, totp_secret: SECRET },
        real: { password: 'pw', totp_secret: 'MFRGGZDFMZTWQ2LK' },
      });
      const res = await forUrl();
      expect(res.entryName).toBe('Entry real');
      expect(res.code).toBe((await generateTotp('MFRGGZDFMZTWQ2LK')).code);
    });

    it('offers nothing when the only secret is stray', async () => {
      serve({ stray: { password: SECRET, totp_secret: SECRET } });
      expect(await forUrl()).toEqual({ code: '' });
    });
  });
});

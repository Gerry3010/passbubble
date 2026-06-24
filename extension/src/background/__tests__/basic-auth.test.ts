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

vi.mock('webextension-polyfill', () => ({
  default: { storage: { sync: { get: vi.fn().mockResolvedValue({}) } }, alarms: { create: vi.fn(), clear: vi.fn() } },
}));

let sessionPresent = true;
let cache: unknown[] | null = [{ id: 'e1' }];
const matchEntriesForUrlMock = vi.hoisted(() => vi.fn());
const decryptEntryMock = vi.hoisted(() => vi.fn());
const getEntryMock = vi.hoisted(() => vi.fn(async (id: string) => ({ id })));

vi.mock('../session-store.js', () => ({
  getSession: vi.fn(() => (sessionPresent ? { serverUrl: 'https://srv', accessToken: 'at' } : null)),
  setSession: vi.fn(),
  clearSession: vi.fn(),
  getEntriesCache: vi.fn(() => cache),
  setEntriesCache: vi.fn(),
  getLoginFrameHost: vi.fn(() => ''),
  setLoginFrameHost: vi.fn(),
}));

vi.mock('@passbubble/shared-ts', () => ({
  PassbubbleClient: vi.fn().mockImplementation(() => ({ setTokens: vi.fn(), getEntry: getEntryMock })),
  decryptEntry: decryptEntryMock,
}));
vi.mock('../autofill-service.js', () => ({ matchEntriesForUrl: matchEntriesForUrlMock }));
vi.mock('../pin-store.js', () => ({}));

import { resolveBasicAuthCredentials } from '../message-handler.js';

beforeEach(() => {
  sessionPresent = true;
  cache = [{ id: 'e1' }];
  matchEntriesForUrlMock.mockReset();
  decryptEntryMock.mockReset().mockResolvedValue({ username: 'admin', password: 's3cret' });
  getEntryMock.mockClear();
});

describe('resolveBasicAuthCredentials', () => {
  it('returns the decrypted credentials for a single unambiguous match', async () => {
    matchEntriesForUrlMock.mockReturnValue([{ id: 'e1' }]);
    const creds = await resolveBasicAuthCredentials('https://intranet.example.com/');
    expect(creds).toEqual({ username: 'admin', password: 's3cret' });
  });

  it('returns null when nothing matches (browser shows its native dialog)', async () => {
    matchEntriesForUrlMock.mockReturnValue([]);
    expect(await resolveBasicAuthCredentials('https://intranet.example.com/')).toBeNull();
  });

  it('returns null when the match is ambiguous (more than one entry)', async () => {
    matchEntriesForUrlMock.mockReturnValue([{ id: 'e1' }, { id: 'e2' }]);
    expect(await resolveBasicAuthCredentials('https://intranet.example.com/')).toBeNull();
  });

  it('returns null when the vault is locked', async () => {
    sessionPresent = false;
    matchEntriesForUrlMock.mockReturnValue([{ id: 'e1' }]);
    expect(await resolveBasicAuthCredentials('https://intranet.example.com/')).toBeNull();
  });

  it('returns null when the matched entry has no password', async () => {
    matchEntriesForUrlMock.mockReturnValue([{ id: 'e1' }]);
    decryptEntryMock.mockResolvedValue({ username: 'admin', password: '' });
    expect(await resolveBasicAuthCredentials('https://intranet.example.com/')).toBeNull();
  });
});

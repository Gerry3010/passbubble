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

const sessionStore: Record<string, unknown> = {};
const syncStore: Record<string, unknown> = {};
const localStore: Record<string, unknown> = {};
const alarmsClear = vi.hoisted(() => vi.fn());

vi.mock('webextension-polyfill', () => ({
  default: {
    storage: {
      sync: {
        get: vi.fn(async (keys: string | string[]) => {
          const arr = Array.isArray(keys) ? keys : [keys];
          const out: Record<string, unknown> = {};
          for (const k of arr) if (k in syncStore) out[k] = syncStore[k];
          return out;
        }),
        set: vi.fn(async (obj: Record<string, unknown>) => Object.assign(syncStore, obj)),
      },
      local: {
        get: vi.fn(async (key: string) => (key in localStore ? { [key]: localStore[key] } : {})),
        set: vi.fn(async (obj: Record<string, unknown>) => Object.assign(localStore, obj)),
        remove: vi.fn(async (key: string) => {
          delete localStore[key];
        }),
      },
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
    },
    alarms: { create: vi.fn(), clear: alarmsClear },
  },
}));

// A togglable fake session so we can simulate locked vs unlocked. `storedPresent`
// models the copy in storage.session that survives a service-worker restart.
let sessionPresent = true;
let storedPresent = false;
const clearSessionMock = vi.hoisted(() => vi.fn());

vi.mock('../session-store.js', () => ({
  getSession: vi.fn(() => (sessionPresent ? { serverUrl: 'https://srv' } : null)),
  setSession: vi.fn(),
  clearSession: clearSessionMock,
  readStoredSession: vi.fn(async () => (storedPresent ? { serverUrl: 'https://srv' } : null)),
  adoptSession: vi.fn(),
  getEntriesCache: vi.fn(),
  setEntriesCache: vi.fn(),
  getLoginFrameHost: vi.fn(() => ''),
  setLoginFrameHost: vi.fn(),
}));

vi.mock('@passbubble/shared-ts', () => ({ PassbubbleClient: vi.fn() }));
vi.mock('../autofill-service.js', () => ({ matchEntriesForUrl: vi.fn() }));
vi.mock('../pin-store.js', () => ({}));

import browser from 'webextension-polyfill';
import { maybeAutoLock, scheduleAutoLock } from '../message-handler.js';
import { readLastLock } from '../lock-log.js';
import { STORAGE_KEYS } from '../../shared/constants.js';

beforeEach(() => {
  for (const k of Object.keys(sessionStore)) delete sessionStore[k];
  for (const k of Object.keys(syncStore)) delete syncStore[k];
  for (const k of Object.keys(localStore)) delete localStore[k];
  sessionPresent = true;
  storedPresent = false;
  clearSessionMock.mockClear();
  alarmsClear.mockClear();
  clearSessionMock.mockImplementation(async () => {
    sessionPresent = false;
    storedPresent = false;
  });
});

describe('maybeAutoLock', () => {
  it('locks when idle past the configured timeout', async () => {
    syncStore[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 5;
    sessionStore[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 6 * 60_000;

    const locked = await maybeAutoLock();

    expect(locked).toBe(true);
    expect(clearSessionMock).toHaveBeenCalledOnce();
    expect(alarmsClear).toHaveBeenCalled();
  });

  it('stays unlocked within the timeout window', async () => {
    syncStore[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 15;
    sessionStore[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 2 * 60_000;

    const locked = await maybeAutoLock();

    expect(locked).toBe(false);
    expect(clearSessionMock).not.toHaveBeenCalled();
  });

  it('never locks and retires the alarm when timeout is 0 ("never")', async () => {
    syncStore[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 0;
    sessionStore[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 999 * 60_000;

    const locked = await maybeAutoLock();

    expect(locked).toBe(true);
    expect(clearSessionMock).not.toHaveBeenCalled();
    expect(alarmsClear).toHaveBeenCalled();
  });

  it('retires the alarm when there is neither a live nor a stored session', async () => {
    sessionPresent = false;
    syncStore[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 5;

    const locked = await maybeAutoLock();

    expect(locked).toBe(true);
    expect(clearSessionMock).not.toHaveBeenCalled();
    expect(alarmsClear).toHaveBeenCalled();
  });

  it('keeps the alarm for a session that only survives in storage.session (SW restarted)', async () => {
    sessionPresent = false;
    storedPresent = true;
    syncStore[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 5;
    sessionStore[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 60_000;

    const locked = await maybeAutoLock();

    expect(locked).toBe(false);
    expect(alarmsClear).not.toHaveBeenCalled();
  });

  it('locks a stored session once it is idle past the timeout', async () => {
    sessionPresent = false;
    storedPresent = true;
    syncStore[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 5;
    sessionStore[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 6 * 60_000;

    const locked = await maybeAutoLock();

    expect(locked).toBe(true);
    expect(clearSessionMock).toHaveBeenCalledOnce();
    expect(sessionStore[STORAGE_KEYS.LAST_ACTIVITY]).toBeUndefined();
  });

  it('records why it locked', async () => {
    syncStore[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 5;
    sessionStore[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 6 * 60_000;

    await maybeAutoLock();

    expect(await readLastLock()).toMatchObject({ reason: 'idle', idleMinutes: 5, settingSource: 'sync' });
  });

  it('keeps "never" from the mirror when storage.sync has lost the value', async () => {
    localStore[STORAGE_KEYS.AUTO_LOCK_MIRROR] = 0;
    sessionStore[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 999 * 60_000;

    await maybeAutoLock();

    expect(clearSessionMock).not.toHaveBeenCalled();
  });

  it('applies the default timeout when no value was ever saved', async () => {
    sessionStore[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 999 * 60_000;

    expect(await maybeAutoLock()).toBe(true);

    expect(clearSessionMock).toHaveBeenCalledOnce();
    expect((await readLastLock())?.settingSource).toBe('default');
  });

  it('applies the default timeout when only storage.sync fails and nothing was mirrored', async () => {
    vi.mocked(browser.storage.sync.get).mockRejectedValueOnce(new Error('sync down'));
    sessionStore[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 999 * 60_000;

    expect(await maybeAutoLock()).toBe(true);

    expect(clearSessionMock).toHaveBeenCalledOnce();
  });

  it('does not lock while the setting cannot be read', async () => {
    vi.mocked(browser.storage.sync.get).mockRejectedValueOnce(new Error('sync down'));
    vi.mocked(browser.storage.local.get).mockRejectedValueOnce(new Error('local down'));
    sessionStore[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 999 * 60_000;

    expect(await maybeAutoLock()).toBe(false);

    expect(clearSessionMock).not.toHaveBeenCalled();
    expect(alarmsClear).not.toHaveBeenCalled();
  });
});

describe('scheduleAutoLock', () => {
  it('mirrors the synced setting into storage.local', async () => {
    syncStore[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 0;

    await scheduleAutoLock();

    expect(localStore[STORAGE_KEYS.AUTO_LOCK_MIRROR]).toBe(0);
  });
});

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

const localStore: Record<string, unknown> = {};

vi.mock('webextension-polyfill', () => ({
  default: {
    storage: {
      local: {
        get: vi.fn(async (key: string) => (key in localStore ? { [key]: localStore[key] } : {})),
        set: vi.fn(async (obj: Record<string, unknown>) => Object.assign(localStore, obj)),
        remove: vi.fn(async (key: string) => {
          delete localStore[key];
        }),
      },
    },
  },
}));

import browser from 'webextension-polyfill';
import { noteUnlocked, readLastLock, recordLock, recordPersistFailure, recordSessionGone } from '../lock-log.js';
import { STORAGE_KEYS } from '../../shared/constants.js';
import { describeLock } from '../../shared/lock-info.js';

beforeEach(() => {
  for (const k of Object.keys(localStore)) delete localStore[k];
});

describe('lock log', () => {
  it('has no last lock on a fresh install', async () => {
    expect(await readLastLock()).toBeNull();
  });

  it('records a lock with its details', async () => {
    await recordLock('idle', { idleMinutes: 15, settingSource: 'default' });
    expect(await readLastLock()).toMatchObject({ reason: 'idle', idleMinutes: 15, settingSource: 'default' });
  });

  it('records a vanished session only if the vault was unlocked', async () => {
    await recordSessionGone('browser-start');
    expect(await readLastLock()).toBeNull();

    await noteUnlocked();
    await recordSessionGone('browser-start');
    expect((await readLastLock())?.reason).toBe('browser-start');
  });

  it('records one vanished session once, keeping the first explanation', async () => {
    await noteUnlocked();
    await recordSessionGone('extension-reload');
    await recordSessionGone('session-lost');
    expect((await readLastLock())?.reason).toBe('extension-reload');
  });

  it('does not blame a vanished session after a recorded lock', async () => {
    await noteUnlocked();
    await recordLock('manual');
    await recordSessionGone('session-lost');
    expect((await readLastLock())?.reason).toBe('manual');
  });

  it('keeps a failed persist in the history without calling it a lock', async () => {
    await recordLock('manual');
    await recordPersistFailure();
    expect((await readLastLock())?.reason).toBe('manual');
    expect(localStore[STORAGE_KEYS.LOCK_LOG]).toHaveLength(2);
  });

  it('keeps only the most recent events', async () => {
    for (let i = 0; i < 25; i++) await recordLock('manual');
    expect(localStore[STORAGE_KEYS.LOCK_LOG]).toHaveLength(20);
  });

  it('never throws when storage fails', async () => {
    vi.mocked(browser.storage.local.get).mockRejectedValue(new Error('storage down'));
    vi.mocked(browser.storage.local.set).mockRejectedValue(new Error('storage down'));
    await expect(noteUnlocked()).resolves.toBeUndefined();
    await expect(recordLock('manual')).resolves.toBeUndefined();
    await expect(recordSessionGone('session-lost')).resolves.toBeUndefined();
    await expect(recordPersistFailure()).resolves.toBeUndefined();
    await expect(readLastLock()).resolves.toBeNull();
    vi.mocked(browser.storage.local.get).mockReset();
    vi.mocked(browser.storage.local.set).mockReset();
  });
});

describe('describeLock', () => {
  it('points out when the timeout was only the default', () => {
    expect(describeLock({ reason: 'idle', at: 0, idleMinutes: 15, settingSource: 'sync' })).toBe('idle for 15 min');
    expect(describeLock({ reason: 'idle', at: 0, idleMinutes: 15, settingSource: 'default' })).toContain('no saved setting');
  });
});

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
        get: vi.fn(async (key: string) => ({ [key]: localStore[key] })),
        set: vi.fn(async (obj: Record<string, unknown>) => {
          Object.assign(localStore, obj);
        }),
      },
    },
    tabs: { onRemoved: { addListener: vi.fn() } },
  },
}));

import {
  getSsoRecord,
  recordSsoUse,
  deleteSsoRecord,
  noteSsoCandidate,
  handleNavigation,
  setSsoRecordedHook,
} from '../sso-memory.js';

beforeEach(() => {
  for (const k of Object.keys(localStore)) delete localStore[k];
});

describe('sso record store', () => {
  it('records, reads back and deletes per-host provider use', async () => {
    await recordSsoUse('example.com', 'google');
    const rec = await getSsoRecord('example.com');
    expect(rec?.provider).toBe('google');
    expect(rec?.hits).toBe(1);

    await recordSsoUse('example.com', 'google');
    expect((await getSsoRecord('example.com'))?.hits).toBe(2);

    // switching provider resets the counter
    await recordSsoUse('example.com', 'apple');
    const switched = await getSsoRecord('example.com');
    expect(switched?.provider).toBe('apple');
    expect(switched?.hits).toBe(1);

    await deleteSsoRecord('example.com');
    expect(await getSsoRecord('example.com')).toBeNull();
  });

  it('invokes the recorded hook so uses can be persisted into entries', async () => {
    const hook = vi.fn();
    setSsoRecordedHook(hook);
    try {
      await recordSsoUse('example.com', 'microsoft');
      expect(hook).toHaveBeenCalledWith('example.com', 'microsoft');
    } finally {
      setSsoRecordedHook(() => {});
    }
  });
});

describe('handleNavigation', () => {
  it('attributes a same-tab OAuth redirect to the previous host', async () => {
    await handleNavigation(1, 'https://www.myapp.com/login');
    await handleNavigation(1, 'https://accounts.google.com/o/oauth2/v2/auth?client_id=x');
    expect((await getSsoRecord('myapp.com'))?.provider).toBe('google');
  });

  it('does not record ordinary navigations', async () => {
    await handleNavigation(1, 'https://myapp.com/login');
    await handleNavigation(1, 'https://myapp.com/dashboard');
    expect(await getSsoRecord('myapp.com')).toBeNull();
  });

  it('falls back to a fresh click candidate when the source tab is unknown', async () => {
    noteSsoCandidate('shop.example', 'github');
    await handleNavigation(99, 'https://github.com/login/oauth/authorize?client_id=x');
    expect((await getSsoRecord('shop.example'))?.provider).toBe('github');
  });

  it('never records the provider itself as the source site', async () => {
    await handleNavigation(2, 'https://accounts.google.com/ServiceLogin');
    await handleNavigation(2, 'https://accounts.google.com/o/oauth2/v2/auth');
    expect(await getSsoRecord('accounts.google.com')).toBeNull();
  });
});

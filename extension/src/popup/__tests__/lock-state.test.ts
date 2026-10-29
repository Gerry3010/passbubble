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

// The popup must follow the background's lock state: a {locked} / {loggedOut}
// answer switches it to the unlock / login screen instead of an empty vault.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { waitFor } from '@testing-library/react';

const responses = vi.hoisted(() => ({} as Record<string, unknown>));
const sendMessage = vi.hoisted(() =>
  vi.fn(async (msg: { type: string }) => {
    const r = responses[msg.type];
    if (r instanceof Error) throw r;
    return r;
  }),
);

vi.mock('webextension-polyfill', () => ({
  default: {
    runtime: { sendMessage },
    storage: { session: { get: vi.fn().mockResolvedValue({}), set: vi.fn(), remove: vi.fn() } },
    tabs: { query: vi.fn().mockResolvedValue([]) },
  },
}));

import { useSessionStore } from '../store/session.js';
import { useEntriesStore } from '../store/entries.js';

beforeEach(() => {
  for (const k of Object.keys(responses)) delete responses[k];
  useSessionStore.setState({ isLoggedIn: true, isUnlocked: true, error: null, isLoading: false });
  useEntriesStore.setState({ entries: [], folders: [], isLoading: false, error: null });
});

describe('popup lock-state handling', () => {
  it('switches to the unlock screen when the vault turns out to be locked', async () => {
    responses.SEARCH_ENTRIES = { locked: true };
    responses.LIST_FOLDERS = { locked: true };
    responses.GET_SESSION = { isLoggedIn: true, isUnlocked: false, serverUrl: 'https://srv' };

    await useEntriesStore.getState().load();

    await waitFor(() => expect(useSessionStore.getState().isUnlocked).toBe(false));
    expect(useSessionStore.getState().isLoggedIn).toBe(true);
    expect(useEntriesStore.getState().isLoading).toBe(false);
  });

  it('switches to the login form with a message when the device was signed out', async () => {
    responses.GET_ENTRY = { locked: true, loggedOut: true };
    responses.GET_SESSION = { isLoggedIn: false, isUnlocked: false, serverUrl: 'https://srv' };

    await useEntriesStore.getState().copyField('e1', 'password');

    await waitFor(() => expect(useSessionStore.getState().isLoggedIn).toBe(false));
    await waitFor(() => expect(useSessionStore.getState().error).toMatch(/log in again/i));
  });

  it('unlock shows the background error and follows a sign-out', async () => {
    useSessionStore.setState({ isUnlocked: false });
    responses.UNLOCK = { ok: false, loggedOut: true, error: 'Your session has expired — please log in again.' };
    responses.GET_SESSION = { isLoggedIn: false, isUnlocked: false, serverUrl: 'https://srv' };

    await useSessionStore.getState().unlock('master');

    expect(useSessionStore.getState()).toMatchObject({
      isLoggedIn: false,
      error: 'Your session has expired — please log in again.',
    });
  });

  it('unlock reports a wrong master password as such', async () => {
    useSessionStore.setState({ isUnlocked: false });
    responses.UNLOCK = { ok: false, wrongPassword: true, error: 'Wrong master password' };
    responses.GET_SESSION = { isLoggedIn: true, isUnlocked: false, serverUrl: 'https://srv' };

    await useSessionStore.getState().unlock('nope');

    expect(useSessionStore.getState()).toMatchObject({ isLoggedIn: true, isUnlocked: false, error: 'Wrong master password' });
  });
});

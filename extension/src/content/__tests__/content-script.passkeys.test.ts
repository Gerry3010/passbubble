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

// The content script with an unlocked vault: passkeys in the fill box for
// fields marked autocomplete="… webauthn", and a passkey-only entry (empty
// password) that must not wipe a typed password.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { FillHandlers, FillPayload } from '../fill-ui.js';

const state = vi.hoisted(() => ({
  unlocked: true,
  suggestions: null as unknown,
}));
const sendMessage = vi.hoisted(() =>
  vi.fn(async (msg: { type: string }): Promise<unknown> => {
    if (msg.type === 'GET_SESSION') return { isLoggedIn: true, isUnlocked: state.unlocked };
    if (msg.type === 'GET_MATCHES_FOR_URL') return [{ id: 'e1', name: 'Example', type: 'password', url: 'https://example.com' }];
    if (msg.type === 'SSO_GET') return { record: null };
    if (msg.type === 'PASSKEY_SUGGESTIONS') return state.suggestions;
    return null;
  }),
);

vi.mock('webextension-polyfill', () => ({
  default: { runtime: { id: 'test', getURL: (p: string) => `chrome-extension://test/${p}`, sendMessage } },
}));

const inject = vi.hoisted(() => vi.fn());
vi.mock('../fill-ui.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../fill-ui.js')>();
  return {
    ...actual,
    injectFillIframe: (...args: Parameters<typeof actual.injectFillIframe>) => {
      inject(args[1], args[2]);
      actual.injectFillIframe(...args);
    },
  };
});

import { removeFillIframe } from '../fill-ui.js';
import { MessageType } from '../../shared/constants.js';
import '../content-script.js';

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

async function focusField(field: HTMLInputElement) {
  (document.activeElement as HTMLElement | null)?.blur?.();
  field.focus();
  for (let i = 0; i < 4; i++) await flush();
}

const SUGGESTIONS = { token: 'suggest-token-0123456789', rows: [{ credentialId: 'c1', userName: 'alice', entryName: 'Example' }] };
const lastPayload = () => inject.mock.calls.at(-1)?.[0] as FillPayload | undefined;
const lastHandlers = () => inject.mock.calls.at(-1)?.[1] as FillHandlers;
const suggestionRequests = () => sendMessage.mock.calls.filter(([m]) => m.type === MessageType.PASSKEY_SUGGESTIONS);

beforeEach(() => {
  state.unlocked = true;
  state.suggestions = null;
  inject.mockClear();
  sendMessage.mockClear();
});

afterEach(() => {
  (document.activeElement as HTMLElement | null)?.blur?.();
  removeFillIframe();
});

describe('passkey-only entries', () => {
  function loginForm() {
    document.body.innerHTML = `
      <form>
        <input type="email" id="user" />
        <input type="password" id="pw" autocomplete="current-password" />
      </form>`;
    return {
      user: document.getElementById('user') as HTMLInputElement,
      password: document.getElementById('pw') as HTMLInputElement,
    };
  }

  it('fill the username and leave a typed password alone', async () => {
    const { user, password } = loginForm();
    password.value = 'typed-by-the-user';
    await focusField(user);
    expect(lastPayload()?.matches).toHaveLength(1);

    lastHandlers().onFillMatch('alice@example.com', '');
    expect(user.value).toBe('alice@example.com');
    expect(password.value).toBe('typed-by-the-user');

    lastHandlers().onFillMatch('alice@example.com', 'stored');
    expect(password.value).toBe('stored');
  });

  it('do not ask for passkeys on fields without the webauthn token', async () => {
    const { user } = loginForm();
    await focusField(user);
    expect(suggestionRequests()).toEqual([]);
  });
});

describe('conditional passkey requests', () => {
  it('offer passkeys on a username-only field marked for webauthn', async () => {
    state.suggestions = SUGGESTIONS;
    document.body.innerHTML = `<input type="text" id="user" autocomplete="username webauthn" />`;
    const user = document.getElementById('user') as HTMLInputElement;
    await focusField(user);
    expect(suggestionRequests()).toHaveLength(1);
    expect(lastPayload()).toEqual({ passkeys: SUGGESTIONS });
  });

  it('add passkeys to the matches of a login form', async () => {
    state.suggestions = SUGGESTIONS;
    document.body.innerHTML = `
      <form>
        <input type="email" id="user" autocomplete="username webauthn" />
        <input type="password" id="pw" />
      </form>`;
    await focusField(document.getElementById('user') as HTMLInputElement);
    expect(lastPayload()).toMatchObject({ passkeys: SUGGESTIONS, matches: [expect.objectContaining({ id: 'e1' })] });
  });

  it('show nothing when there is no open request', async () => {
    state.suggestions = { rows: [] };
    document.body.innerHTML = `<input type="text" id="user" autocomplete="webauthn" />`;
    await focusField(document.getElementById('user') as HTMLInputElement);
    expect(suggestionRequests()).toHaveLength(1);
    expect(inject).not.toHaveBeenCalled();
  });

  it('show the unlock box while the vault is locked', async () => {
    state.suggestions = { rows: [], locked: true };
    document.body.innerHTML = `<input type="text" id="user" autocomplete="webauthn" />`;
    await focusField(document.getElementById('user') as HTMLInputElement);
    expect(lastPayload()).toEqual({ locked: true, loggedIn: true });
  });
});

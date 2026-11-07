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

// The in-page box on login and signup fields while the vault is locked. Runs the
// content script itself on a jsdom page, with the real fill UI.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const session = vi.hoisted(() => ({ isLoggedIn: true, isUnlocked: false }));
const sendMessage = vi.hoisted(() =>
  vi.fn(async (msg: { type: string }): Promise<unknown> => {
    if (msg.type === 'GET_SESSION') return { ...session };
    if (msg.type === 'GET_MATCHES_FOR_URL') return [];
    if (msg.type === 'SSO_GET') return { record: null };
    return null;
  }),
);

vi.mock('webextension-polyfill', () => ({
  default: {
    runtime: {
      id: 'test',
      getURL: (p: string) => `chrome-extension://test/${p}`,
      sendMessage,
    },
  },
}));

const inject = vi.hoisted(() => vi.fn());

vi.mock('../fill-ui.js', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../fill-ui.js')>();
  return {
    ...actual,
    injectFillIframe: (...args: Parameters<typeof actual.injectFillIframe>) => {
      inject(args[1]);
      actual.injectFillIframe(...args);
    },
  };
});

import { removeFillIframe, isFillIframeShown, isLockedBoxShown } from '../fill-ui.js';
import '../content-script.js';

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

function signupForm(): { password: HTMLInputElement } {
  document.body.innerHTML = `
    <form>
      <input type="email" name="email" />
      <input type="password" id="pw" autocomplete="new-password" />
      <input type="password" id="pw2" placeholder="Confirm password" />
    </form>`;
  return { password: document.getElementById('pw') as HTMLInputElement };
}

function loginForm(): { password: HTMLInputElement } {
  document.body.innerHTML = `
    <form>
      <input type="email" name="email" />
      <input type="password" id="pw" autocomplete="current-password" />
    </form>`;
  return { password: document.getElementById('pw') as HTMLInputElement };
}

async function focusField(field: HTMLInputElement) {
  (document.activeElement as HTMLElement | null)?.blur?.();
  field.focus();
  await flush();
  await flush();
}

const UNLOCK_BOX = (loggedIn: boolean) => expect.objectContaining({ locked: true, loggedIn });

describe('fill box while the vault is locked', () => {
  beforeEach(() => {
    session.isLoggedIn = true;
    session.isUnlocked = false;
    inject.mockClear();
  });

  afterEach(() => {
    (document.activeElement as HTMLElement | null)?.blur?.();
    removeFillIframe();
  });

  it('offers the unlock box on a signup field', async () => {
    const { password } = signupForm();
    await focusField(password);

    expect(inject).toHaveBeenCalledTimes(1);
    expect(inject).toHaveBeenCalledWith(UNLOCK_BOX(true));
    expect(isLockedBoxShown()).toBe(true);
  });

  it('offers the sign-in variant on a signup field when logged out', async () => {
    session.isLoggedIn = false;
    const { password } = signupForm();
    await focusField(password);

    expect(inject).toHaveBeenCalledWith(UNLOCK_BOX(false));
  });

  it('still offers the unlock box on a login field', async () => {
    const { password } = loginForm();
    await focusField(password);

    expect(inject).toHaveBeenCalledWith(UNLOCK_BOX(true));
  });

  it('shows nothing when the field lost the focus in the meantime', async () => {
    const { password } = signupForm();
    password.focus();
    password.blur();
    await flush();
    await flush();

    expect(inject).not.toHaveBeenCalled();
  });

  it('does not rebuild the box on another focus while still locked', async () => {
    const { password } = signupForm();
    await focusField(password);
    await focusField(password);
    window.dispatchEvent(new Event('focus'));
    await flush();

    expect(inject).toHaveBeenCalledTimes(1);
  });

  it('shows the generator box when the field is focused after unlocking', async () => {
    const { password } = signupForm();
    await focusField(password);

    // The box's button removes the box and opens the popup.
    removeFillIframe();
    session.isUnlocked = true;
    await focusField(password);

    expect(inject).toHaveBeenLastCalledWith(expect.objectContaining({ generate: true }));
    expect(isLockedBoxShown()).toBe(false);
  });

  it('replaces an unlock box left on screen when the tab regains the focus after unlocking', async () => {
    const { password } = signupForm();
    await focusField(password);
    expect(isLockedBoxShown()).toBe(true);

    // Unlocked from the toolbar, without using the box; the field kept the focus.
    session.isUnlocked = true;
    window.dispatchEvent(new Event('focus'));
    await flush();
    await flush();

    expect(inject).toHaveBeenLastCalledWith(expect.objectContaining({ generate: true }));
    expect(isLockedBoxShown()).toBe(false);
    expect(isFillIframeShown(password)).toBe(true);
  });

  it('removes a stale unlock box on a login field with nothing to offer', async () => {
    const { password } = loginForm();
    await focusField(password);
    session.isUnlocked = true;
    window.dispatchEvent(new Event('focus'));
    await flush();
    await flush();

    expect(isFillIframeShown()).toBe(false);
  });

  it('leaves a regular box alone on a duplicate focus event', async () => {
    session.isUnlocked = true;
    const { password } = signupForm();
    await focusField(password);
    expect(inject).toHaveBeenCalledTimes(1);

    window.dispatchEvent(new Event('focus'));
    await flush();

    expect(inject).toHaveBeenCalledTimes(1);
  });
});

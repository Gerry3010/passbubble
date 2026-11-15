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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, renderHook, screen, fireEvent, act, waitFor } from '@testing-library/react';

const api = vi.hoisted(() => ({
  sendMessage: vi.fn(),
  openPopup: vi.fn() as ReturnType<typeof vi.fn> | undefined,
  createWindow: vi.fn(),
  storageListeners: [] as Array<(changes: Record<string, unknown>, area: string) => void>,
}));

vi.mock('webextension-polyfill', () => ({
  default: {
    runtime: { sendMessage: api.sendMessage, getURL: (p: string) => `chrome-extension://ext/${p}` },
    storage: {
      onChanged: {
        addListener: (fn: (c: Record<string, unknown>, a: string) => void) => api.storageListeners.push(fn),
        removeListener: (fn: (c: Record<string, unknown>, a: string) => void) => {
          api.storageListeners = api.storageListeners.filter((l) => l !== fn);
        },
      },
    },
    get action() {
      return { openPopup: api.openPopup };
    },
    windows: { create: api.createWindow },
  },
}));

import { ARM_DELAY_MS, IO_REMEASURE_MS, IO_SILENCE_MS, PasskeyDialog, displayText, useArmed } from '../PasskeyDialog.js';
import { MessageType, STORAGE_KEYS } from '../../shared/constants.js';
import type { DialogState } from '../../background/passkey-service.js';

const TOKEN = 'tok_0123456789abcdefABCD';

let states: Record<string, DialogState | undefined>;
let completeReply: Record<string, unknown>;

beforeEach(() => {
  document.body.innerHTML = '';
  api.storageListeners = [];
  api.openPopup = vi.fn().mockResolvedValue(undefined);
  api.createWindow.mockReset().mockResolvedValue({});
  states = {};
  completeReply = { ok: true };
  api.sendMessage.mockReset().mockImplementation(async (m: { type: string; payload: { token: string } }) => {
    if (m.type === MessageType.PASSKEY_UI_STATE) {
      const state = states[m.payload.token];
      return state ? { state } : { error: 'This passkey request is no longer open.' };
    }
    if (m.type === MessageType.PASSKEY_COMPLETE) return completeReply;
    return undefined;
  });
});

function sendToken(token: string, source: MessageEventSource | null = window) {
  window.dispatchEvent(new MessageEvent('message', { data: token, source }));
}

const completes = () =>
  api.sendMessage.mock.calls.filter(([m]) => m.type === MessageType.PASSKEY_COMPLETE).map(([m]) => m.payload);

const GET: DialogState = {
  mode: 'get',
  type: 'get',
  host: 'login.example.com',
  rpId: 'example.com',
  accounts: [{ credentialId: 'cred-1', userName: 'alice@example.com', entryName: 'Example' }],
};

async function open(state: DialogState) {
  states[TOKEN] = state;
  render(<PasskeyDialog />);
  await act(async () => sendToken(TOKEN));
}

const armed = (name: RegExp) => waitFor(() => expect((screen.getByRole('button', { name }) as HTMLButtonElement).disabled).toBe(false), { timeout: 2000 });

describe('token', () => {
  it('takes the token from its parent only, and only once the background accepts it', async () => {
    states[TOKEN] = GET;
    render(<PasskeyDialog />);
    const other = document.createElement('iframe');
    document.body.appendChild(other);
    await act(async () => sendToken(TOKEN, other.contentWindow));
    expect(api.sendMessage).not.toHaveBeenCalled();

    // A page script can post to the dialog too; a wrong token changes nothing.
    await act(async () => sendToken('fake_0123456789abcdefABC'));
    expect(screen.queryByRole('alert')).toBeNull();
    await act(async () => sendToken(TOKEN));
    expect(await screen.findByText('alice@example.com')).toBeDefined();
    expect(api.sendMessage).toHaveBeenCalledWith({ type: MessageType.PASSKEY_UI_STATE, payload: { token: TOKEN } });
  });

  it('ignores strings that cannot be a token', async () => {
    render(<PasskeyDialog />);
    await act(async () => sendToken('short'));
    await act(async () => sendToken('has spaces in it, not a token'));
    expect(api.sendMessage).not.toHaveBeenCalled();
  });
});

describe('sign in', () => {
  it('shows the requesting host and the accounts, and picks one once armed', async () => {
    await open(GET);
    expect(screen.getByText('login.example.com')).toBeDefined();
    expect(screen.getByText(/^passkey for/i).textContent).toBe('Passkey for example.com');
    const account = screen.getByRole('button', { name: /alice@example\.com/ }) as HTMLButtonElement;
    expect(account.disabled).toBe(true);
    await armed(/alice@example\.com/);
    await act(async () => fireEvent.click(account));
    expect(completes()).toEqual([{ token: TOKEN, choice: { action: 'select', credentialId: 'cred-1' } }]);
    expect(await screen.findByText(/signing in/i)).toBeDefined();
  });

  it('swallows Enter and Space before it is armed', async () => {
    await open(GET);
    const enter = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true });
    document.dispatchEvent(enter);
    expect(enter.defaultPrevented).toBe(true);
    await armed(/alice@example\.com/);
    const later = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true });
    document.dispatchEvent(later);
    expect(later.defaultPrevented).toBe(false);
  });

  it('cancels and hands over to another device at any time', async () => {
    await open(GET);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Cancel' })));
    expect(completes()).toEqual([{ token: TOKEN, choice: { action: 'cancel' } }]);
  });

  it('offers another device when nothing is stored for the site', async () => {
    await open({ ...GET, accounts: [] });
    expect(screen.getByText(/no passkey for this site/i)).toBeDefined();
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /use another device/i })));
    expect(completes()).toEqual([{ token: TOKEN, choice: { action: 'native' } }]);
  });

  it('strips bidi controls and shortens names from the site', async () => {
    await open({ ...GET, accounts: [{ credentialId: 'c', userName: `evil‮moc.elgoog${'x'.repeat(100)}`, entryName: 'E' }] });
    const label = screen.getByRole('button', { name: /evil/ }).textContent ?? '';
    expect(label).not.toMatch(/‮/);
    expect(label.length).toBeLessThan(80);
    expect(displayText('a‏b⁦c')).toBe('abc');
  });
});

describe('save', () => {
  const CREATE: DialogState = {
    mode: 'create',
    type: 'create',
    host: 'example.com',
    rpId: 'example.com',
    rpName: 'Example',
    userName: 'alice',
    targets: [{ entryId: 'e1', entryName: 'Example login', userName: 'alice' }],
    readOnlyTargets: [],
  };

  it('saves into the matching entry by default, or a new one', async () => {
    await open(CREATE);
    expect(screen.queryByText(/^passkey for/i)).toBeNull(); // rp id = host: no note
    const select = screen.getByRole('combobox') as HTMLSelectElement;
    expect(select.value).toBe('e1');
    await armed(/^save$/i);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /^save$/i })));
    expect(completes()).toEqual([{ token: TOKEN, choice: { action: 'create', target: 'e1' } }]);
  });

  it('switches to a new entry when the chosen one is read-only', async () => {
    await open(CREATE);
    completeReply = { ok: false, readOnly: true, error: 'You only have read access to this entry.' };
    states[TOKEN] = { ...CREATE, readOnlyTargets: ['e1'] };
    await armed(/^save$/i);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /^save$/i })));
    expect(await screen.findByText(/read access/i)).toBeDefined();
    await waitFor(() => expect((screen.getByRole('combobox') as HTMLSelectElement).value).toBe('new'));
    expect(screen.queryByRole('option', { name: /Example login/ })).toBeNull();
  });

  it('shows why saving failed and closes on request', async () => {
    await open(CREATE);
    completeReply = { ok: false, error: 'Could not save the passkey: offline' };
    await armed(/^save$/i);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /^save$/i })));
    expect((await screen.findByRole('alert')).textContent).toContain('offline');
    const post = vi.spyOn(window.parent, 'postMessage');
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(post).toHaveBeenCalledWith({ type: 'passkey-dialog', action: 'close' }, '*');
  });

  it('asks before reporting an existing passkey', async () => {
    await open({ mode: 'exists', type: 'create', host: 'example.com', rpId: 'example.com' });
    expect(screen.getByText(/already saved in passbubble/i)).toBeDefined();
    await armed(/^ok$/i);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /^ok$/i })));
    expect(completes()).toEqual([{ token: TOKEN, choice: { action: 'exists-ok' } }]);
  });
});

describe('trouble reading the vault', () => {
  it('shows the error and still offers another device', async () => {
    api.sendMessage.mockImplementation(async (m: { type: string }) =>
      m.type === MessageType.PASSKEY_UI_STATE ? { tokenValid: true, error: 'Could not read your passkeys: offline' } : { ok: true },
    );
    render(<PasskeyDialog />);
    await act(async () => sendToken(TOKEN));
    expect((await screen.findByRole('alert')).textContent).toContain('offline');
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /use another device/i })));
    expect(completes()).toEqual([{ token: TOKEN, choice: { action: 'native' } }]);
  });
});

describe('locked', () => {
  const LOCKED: DialogState = { mode: 'locked', type: 'get', host: 'example.com', rpId: 'example.com', loggedIn: true };

  it('opens the toolbar popup from the click and continues after unlocking', async () => {
    await open(LOCKED);
    await armed(/^unlock$/i);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /^unlock$/i })));
    expect(api.openPopup).toHaveBeenCalledTimes(1);

    states[TOKEN] = GET;
    await act(async () => {
      for (const l of api.storageListeners) l({ [STORAGE_KEYS.UNLOCKED_SESSION]: { newValue: {} } }, 'session');
    });
    expect(await screen.findByText('alice@example.com')).toBeDefined();
  });

  it('opens the popup page in a window where openPopup is missing', async () => {
    api.openPopup = undefined;
    await open(LOCKED);
    await armed(/^unlock$/i);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /^unlock$/i })));
    expect(api.createWindow).toHaveBeenCalledWith(expect.objectContaining({ url: 'chrome-extension://ext/popup/index.html', type: 'popup' }));
  });

  it('ignores storage changes of other areas and keys', async () => {
    await open(LOCKED);
    const calls = api.sendMessage.mock.calls.length;
    await act(async () => {
      for (const l of api.storageListeners) {
        l({ [STORAGE_KEYS.UNLOCKED_SESSION]: {} }, 'local');
        l({ other: {} }, 'session');
      }
    });
    expect(api.sendMessage.mock.calls.length).toBe(calls);
  });
});

describe('arming (clickjacking guard)', () => {
  type Callback = (entries: Array<{ isVisible?: boolean }>) => void;
  let callbacks: Callback[];
  // What a fresh observation reports; undefined = Chrome stays quiet.
  let visibleNow: boolean | undefined;

  beforeEach(() => {
    vi.useFakeTimers();
    callbacks = [];
    visibleNow = undefined;
    // Chrome's IntersectionObserver v2: reports isVisible when trackVisibility is set.
    (globalThis as unknown as { IntersectionObserver: unknown }).IntersectionObserver = class {
      trackVisibility = true;
      private cb: Callback;
      constructor(cb: Callback) {
        this.cb = cb;
        callbacks.push(cb);
      }
      observe() {
        if (visibleNow !== undefined) this.cb([{ isVisible: visibleNow }]);
      }
      unobserve() {}
      disconnect() {}
    };
  });

  afterEach(() => {
    vi.useRealTimers();
    delete (globalThis as unknown as { IntersectionObserver?: unknown }).IntersectionObserver;
  });

  const report = (isVisible: boolean) => act(() => callbacks.forEach((cb) => cb([{ isVisible }])));
  const advance = (ms: number) => act(() => vi.advanceTimersByTime(ms));

  it('waits for the observer to see the frame, then for the delay', () => {
    const { result } = renderHook(() => useArmed());
    advance(ARM_DELAY_MS + 100);
    expect(result.current).toBe(false);
    report(true);
    advance(ARM_DELAY_MS - 50);
    expect(result.current).toBe(false);
    advance(100);
    expect(result.current).toBe(true);
  });

  it('disarms when the frame is covered and starts over when it is visible again', () => {
    const { result } = renderHook(() => useArmed());
    report(true);
    advance(ARM_DELAY_MS + 10);
    expect(result.current).toBe(true);
    // Covered: every fresh measurement says so.
    visibleNow = false;
    report(false);
    expect(result.current).toBe(false);
    advance(IO_SILENCE_MS * 2);
    expect(result.current).toBe(false);
    visibleNow = true;
    report(true);
    advance(ARM_DELAY_MS + 10);
    expect(result.current).toBe(true);
  });

  it('restarts the delay on a resize', () => {
    const { result } = renderHook(() => useArmed());
    report(true);
    advance(ARM_DELAY_MS - 100);
    act(() => window.dispatchEvent(new Event('resize')));
    advance(200);
    expect(result.current).toBe(false);
    advance(ARM_DELAY_MS);
    expect(result.current).toBe(true);
  });

  it('measures again every second, so a stale "not visible" clears', () => {
    const { result } = renderHook(() => useArmed());
    report(false);
    advance(IO_REMEASURE_MS / 2);
    expect(result.current).toBe(false);
    // Nothing covers the frame: the next measurement says so.
    visibleNow = true;
    advance(IO_REMEASURE_MS / 2 + ARM_DELAY_MS + 10);
    expect(result.current).toBe(true);
  });

  it('arms when Chrome stops answering after one "not visible"', () => {
    const { result } = renderHook(() => useArmed());
    report(false);
    advance(IO_SILENCE_MS - 10);
    expect(result.current).toBe(false);
    advance(IO_REMEASURE_MS + ARM_DELAY_MS + 10);
    expect(result.current).toBe(true);
  });

  it('stays disabled while every new measurement says covered', () => {
    const { result } = renderHook(() => useArmed());
    visibleNow = false;
    report(false);
    advance(IO_REMEASURE_MS * 5);
    expect(result.current).toBe(false);
  });

  it('does not stay disabled when the observer never reports', () => {
    const { result } = renderHook(() => useArmed());
    advance(IO_SILENCE_MS - 10);
    expect(result.current).toBe(false);
    advance(10 + ARM_DELAY_MS + 10);
    expect(result.current).toBe(true);
  });
});

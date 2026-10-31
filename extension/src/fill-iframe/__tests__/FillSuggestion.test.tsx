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
import { render, screen, fireEvent, act, waitFor } from '@testing-library/react';

// The generate mode reads the generator defaults from storage.sync.
const syncGet = vi.hoisted(() => vi.fn(async () => ({}) as Record<string, unknown>));
vi.mock('webextension-polyfill', () => ({ default: { storage: { sync: { get: syncGet } } } }));

import { FillSuggestion } from '../FillSuggestion.js';
import { STORAGE_KEYS } from '../../shared/constants.js';

// In jsdom window.parent === window, so the component accepts a message whose
// source is window. Drive FILL_INIT through a real message event.
function postInit(payload: Record<string, unknown>) {
  window.dispatchEvent(new MessageEvent('message', { source: window, data: { type: 'FILL_INIT', ...payload } }));
}

describe('FillSuggestion locked mode', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    vi.restoreAllMocks();
  });

  it('shows an Unlock button when locked and signed in', async () => {
    render(<FillSuggestion />);
    await act(async () => postInit({ locked: true, loggedIn: true }));
    await waitFor(() => expect(screen.getByRole('button', { name: /unlock passbubble/i })).toBeDefined());
  });

  it('shows a Sign-in button when locked and not signed in', async () => {
    render(<FillSuggestion />);
    await act(async () => postInit({ locked: true, loggedIn: false }));
    await waitFor(() => expect(screen.getByRole('button', { name: /sign in to passbubble/i })).toBeDefined());
  });

  it('posts FILL_UNLOCK to the parent when the unlock button is clicked', async () => {
    const postSpy = vi.spyOn(window.parent, 'postMessage');
    render(<FillSuggestion />);
    await act(async () => postInit({ locked: true, loggedIn: true }));
    const btn = await screen.findByRole('button', { name: /unlock passbubble/i });

    await act(async () => fireEvent.click(btn));

    expect(postSpy).toHaveBeenCalledWith({ type: 'FILL_UNLOCK' }, '*');
  });

  it('does not show the unlock button for a normal (unlocked) fill', async () => {
    render(<FillSuggestion />);
    await act(async () => postInit({ matches: [], locked: false }));
    await waitFor(() => expect(screen.getByText(/no matching entries/i)).toBeDefined());
    expect(screen.queryByRole('button', { name: /unlock passbubble/i })).toBeNull();
  });
});

describe('FillSuggestion keepFocus', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  function pressDefaultPrevented(): boolean {
    const ev = new MouseEvent('mousedown', { bubbles: true, cancelable: true });
    document.body.dispatchEvent(ev);
    return ev.defaultPrevented;
  }

  it('cancels the focus change of a press when asked to keep the focus (Flutter web)', async () => {
    render(<FillSuggestion />);
    await act(async () => postInit({ matches: [], keepFocus: true }));
    expect(pressDefaultPrevented()).toBe(true);
  });

  it('leaves presses alone otherwise', async () => {
    render(<FillSuggestion />);
    await act(async () => postInit({ matches: [] }));
    expect(pressDefaultPrevented()).toBe(false);
  });
});

describe('FillSuggestion generate mode', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    syncGet.mockImplementation(async () => ({}));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  const shownPassword = () => screen.getByRole('status', { name: 'Generated password' }).textContent ?? '';

  async function showGenerator(payload: Record<string, unknown> = {}) {
    render(<FillSuggestion />);
    await act(async () => postInit({ generate: true, ...payload }));
    await screen.findByRole('status', { name: 'Generated password' });
  }

  it('generates a password itself from the stored defaults', async () => {
    syncGet.mockImplementation(async () => ({
      [STORAGE_KEYS.GENERATOR_DEFAULTS]: { length: 28, symbols: false },
    }));
    await showGenerator();
    expect(shownPassword()).toMatch(/^[a-zA-Z0-9]{28}$/);
    expect(screen.getByText('generate')).toBeDefined();
  });

  it('offers "Use password", which only fills — no claim about saving an entry', async () => {
    await showGenerator();
    expect(screen.getByRole('button', { name: 'Use password' })).toBeDefined();
    expect(screen.queryByText(/use & save/i)).toBeNull();
    expect(screen.queryByText(/saves a new entry/i)).toBeNull();
  });

  it('shows the password already used on the page instead of a fresh one', async () => {
    await showGenerator({ generatePassword: 'Already-Used-1' });
    expect(shownPassword()).toBe('Already-Used-1');
  });

  it('regenerate makes a new password', async () => {
    await showGenerator({ generatePassword: 'Already-Used-1' });
    fireEvent.click(screen.getByRole('button', { name: /regenerate/i }));
    expect(shownPassword()).not.toBe('Already-Used-1');
    expect(Array.from(shownPassword())).toHaveLength(20);
  });

  it('options are collapsed at first and regenerate on change', async () => {
    await showGenerator();
    expect(screen.queryByRole('slider', { name: 'Length' })).toBeNull();

    const toggle = screen.getByRole('button', { name: /options/i });
    fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-expanded')).toBe('true');

    fireEvent.change(screen.getByRole('slider', { name: 'Length' }), { target: { value: '16' } });
    fireEvent.change(screen.getByRole('textbox', { name: 'Symbol set' }), { target: { value: '-_' } });
    const pw = shownPassword();
    expect(Array.from(pw)).toHaveLength(16);
    expect(pw).toMatch(/^[a-zA-Z0-9_-]+$/);
    expect(pw).toMatch(/[-_]/);
  });

  it('Use password sends the password shown at that moment to the page', async () => {
    const postSpy = vi.spyOn(window.parent, 'postMessage');
    await showGenerator();
    fireEvent.click(screen.getByRole('button', { name: /regenerate/i }));
    const chosen = shownPassword();

    fireEvent.click(screen.getByRole('button', { name: 'Use password' }));

    expect(postSpy).toHaveBeenCalledWith({ type: 'FILL_USE_GENERATED', password: chosen }, '*');
  });

  it('Use password is disabled while the options cannot produce a password', async () => {
    const postSpy = vi.spyOn(window.parent, 'postMessage');
    await showGenerator();
    fireEvent.click(screen.getByRole('button', { name: /options/i }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Symbol set' }), { target: { value: '' } });

    expect(shownPassword()).toMatch(/symbol set is empty/i);
    const use = screen.getByRole('button', { name: 'Use password' }) as HTMLButtonElement;
    expect(use.disabled).toBe(true);
    fireEvent.click(use);
    expect(postSpy).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'FILL_USE_GENERATED' }), '*');
  });

  it('hides the options on Flutter pages, where the box never takes the focus', async () => {
    await showGenerator({ keepFocus: true });
    expect(screen.queryByRole('button', { name: /options/i })).toBeNull();
    expect(screen.getByRole('button', { name: /regenerate/i })).toBeDefined();
  });

  it('reports a new height when the card changes size (options opened)', async () => {
    let onResize: () => void = () => {};
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(cb: () => void) {
          onResize = cb;
        }
        observe() {}
        disconnect() {}
      },
    );
    const postSpy = vi.spyOn(window.parent, 'postMessage');
    await showGenerator();
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({ height: 240 } as DOMRect);

    act(() => onResize());

    expect(postSpy).toHaveBeenCalledWith({ type: 'FILL_RESIZE', height: 240 }, '*');
  });
});

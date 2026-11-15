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

// Passkey rows in the fill box (conditional mediation).

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';

const sendMessage = vi.hoisted(() => vi.fn());
vi.mock('webextension-polyfill', () => ({
  default: { storage: { sync: { get: vi.fn(async () => ({})) } }, runtime: { sendMessage } },
}));

import { FillSuggestion } from '../FillSuggestion.js';
import { MessageType } from '../../shared/constants.js';

function postInit(payload: Record<string, unknown>) {
  window.dispatchEvent(new MessageEvent('message', { source: window, data: { type: 'FILL_INIT', ...payload } }));
}

const PASSKEYS = {
  token: 'suggest-token-0123456789',
  rows: [{ credentialId: 'cred-1', userName: 'alice@example.com', entryName: 'Example' }],
};

describe('FillSuggestion passkey rows', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    sendMessage.mockReset();
  });

  it('lists passkeys next to the matches', async () => {
    render(<FillSuggestion />);
    await act(async () =>
      postInit({ matches: [{ id: 'e1', name: 'Example login', url: 'https://example.com', type: 'password' }], passkeys: PASSKEYS }),
    );
    expect(screen.getByRole('list', { name: 'Passkeys' })).toBeDefined();
    expect(screen.getByText(/alice@example\.com/)).toBeDefined();
    expect(screen.getByText('Example login')).toBeDefined();
  });

  it('shows passkeys alone without "No matching entries"', async () => {
    render(<FillSuggestion />);
    await act(async () => postInit({ matches: [], passkeys: PASSKEYS }));
    expect(screen.queryByText(/no matching entries/i)).toBeNull();
  });

  it('sends the pick straight to the background and only tells the page it is done', async () => {
    sendMessage.mockResolvedValue({ ok: true });
    const post = vi.spyOn(window.parent, 'postMessage');
    render(<FillSuggestion />);
    await act(async () => postInit({ matches: [], passkeys: PASSKEYS }));
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /alice@example\.com/ })));
    expect(sendMessage).toHaveBeenCalledWith({
      type: MessageType.PASSKEY_COMPLETE,
      payload: { token: PASSKEYS.token, choice: { action: 'select', credentialId: 'cred-1' } },
    });
    const toPage = post.mock.calls.map(([m]) => m as { type: string });
    expect(toPage.map((m) => m.type)).toContain('FILL_DISMISS');
    expect(JSON.stringify(toPage)).not.toContain(PASSKEYS.token);
  });

  it('shows an error and stays open when the background refuses', async () => {
    sendMessage.mockResolvedValue({ ok: false, error: 'This passkey request is no longer open.' });
    render(<FillSuggestion />);
    await act(async () => postInit({ matches: [], passkeys: PASSKEYS }));
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /alice@example\.com/ })));
    expect(screen.getByRole('alert').textContent).toMatch(/no longer open/);
  });

  it('ignores malformed passkey data', async () => {
    render(<FillSuggestion />);
    await act(async () => postInit({ matches: [], passkeys: { token: 5, rows: 'x' } }));
    expect(screen.queryByRole('list', { name: 'Passkeys' })).toBeNull();
    expect(screen.getByText(/no matching entries/i)).toBeDefined();
  });
});

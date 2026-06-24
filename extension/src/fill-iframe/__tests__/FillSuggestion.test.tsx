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
import { render, screen, fireEvent, act, waitFor } from '@testing-library/react';
import { FillSuggestion } from '../FillSuggestion.js';

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

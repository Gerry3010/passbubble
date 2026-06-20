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
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import type { EntryResponse } from '@passbubble/shared-ts';

const sendMessage = vi.hoisted(() => vi.fn());
vi.mock('webextension-polyfill', () => ({ default: { runtime: { sendMessage } } }));

import { EntryDetail } from '../components/EntryDetail.js';

const entry = {
  id: 'e1',
  name: 'GitHub',
  url: 'https://github.com',
  type: 'password',
  owner_id: 'u1',
  created_at: '',
  updated_at: '',
} as EntryResponse;

describe('EntryDetail', () => {
  beforeEach(() => sendMessage.mockReset());

  it('loads and shows the decrypted fields with a masked password', async () => {
    sendMessage.mockResolvedValue({ data: { username: 'octocat', password: 's3cret', notes: 'hi' } });
    render(<EntryDetail entry={entry} onBack={() => {}} />);

    await waitFor(() => expect(screen.getByText('octocat')).toBeDefined());
    expect(screen.getByText('••••••••')).toBeDefined();
    expect(screen.queryByText('s3cret')).toBeNull();
  });

  it('reveals the password on Show', async () => {
    sendMessage.mockResolvedValue({ data: { username: 'octocat', password: 's3cret' } });
    render(<EntryDetail entry={entry} onBack={() => {}} />);

    await waitFor(() => expect(screen.getByText('octocat')).toBeDefined());
    fireEvent.click(screen.getByRole('button', { name: /show/i }));
    expect(screen.getByText('s3cret')).toBeDefined();
  });

  it('shows a lock message when the vault is locked', async () => {
    sendMessage.mockResolvedValue({ locked: true });
    render(<EntryDetail entry={entry} onBack={() => {}} />);
    await waitFor(() => expect(screen.getByText(/vault is locked/i)).toBeDefined());
  });

  it('shows Copied! feedback immediately after clicking copy', async () => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } });
    sendMessage.mockResolvedValue({ data: { username: 'octocat', password: 's3cret' } });
    render(<EntryDetail entry={entry} onBack={() => {}} />);

    await waitFor(() => expect(screen.getByText('octocat')).toBeDefined());
    const copyBtns = screen.getAllByRole('button', { name: /copy/i });
    fireEvent.click(copyBtns[0]);

    expect(screen.getByText('Copied!')).toBeDefined();
  });
});

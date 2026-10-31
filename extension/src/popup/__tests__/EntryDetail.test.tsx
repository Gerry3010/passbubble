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

  it('masks secret custom fields and reveals each field on its own', async () => {
    sendMessage.mockResolvedValue({
      data: {
        username: 'octocat',
        password: 's3cret',
        custom_fields: [
          { label: 'Recovery', value: 'ask the cat', type: 'text' },
          { label: 'PIN', value: '4711', type: 'password' },
          { label: 'Backup', value: 'backup-code', type: 'password' },
          { label: 'Deploy key', value: 'ssh-ed25519 AAAA', type: 'ssh' },
        ],
      },
    });
    render(<EntryDetail entry={entry} onBack={() => {}} />);

    await waitFor(() => expect(screen.getByText('octocat')).toBeDefined());
    expect(screen.getByText('ask the cat')).toBeDefined();
    for (const secret of ['s3cret', '4711', 'backup-code', 'ssh-ed25519 AAAA']) {
      expect(screen.queryByText(secret)).toBeNull();
    }

    // Show buttons in field order: password, PIN, Backup, Deploy key.
    fireEvent.click(screen.getAllByRole('button', { name: 'Show' })[1]);
    expect(screen.getByText('4711')).toBeDefined();
    expect(screen.queryByText('s3cret')).toBeNull();
    expect(screen.queryByText('backup-code')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Hide' }));
    expect(screen.queryByText('4711')).toBeNull();
  });

  it("shows the entry's tags, normalised", async () => {
    sendMessage.mockResolvedValue({ data: { username: 'octocat', tags: ['#Work ', 'work', 'client', 42] } });
    render(<EntryDetail entry={entry} onBack={() => {}} />);

    const list = await screen.findByRole('list', { name: 'Tags' });
    expect(Array.from(list.querySelectorAll('li')).map((li) => li.textContent)).toEqual(['#Work', '#client']);
  });

  it('shows no tag list for an entry without tags', async () => {
    sendMessage.mockResolvedValue({ data: { username: 'octocat' } });
    render(<EntryDetail entry={entry} onBack={() => {}} />);
    await waitFor(() => expect(screen.getByText('octocat')).toBeDefined());
    expect(screen.queryByRole('list', { name: 'Tags' })).toBeNull();
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

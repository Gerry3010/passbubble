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
import { EntryList } from '../components/EntryList.js';
import type { EntryResponse } from '@passbubble/shared-ts';

vi.mock('../store/entries.js', () => ({
  useEntriesStore: vi.fn(),
}));

import { useEntriesStore } from '../store/entries.js';

const mockUseStore = useEntriesStore as ReturnType<typeof vi.fn>;

function makeEntry(id: string, name: string, url?: string): EntryResponse {
  return { id, owner_id: 'u1', type: 'password', name, url, created_at: '', updated_at: '' };
}

function defaultStore(overrides = {}) {
  return {
    entries: [],
    folders: [],
    currentHost: '',
    isLoading: false,
    error: null,
    load: vi.fn().mockResolvedValue(undefined),
    copyField: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

describe('EntryList', () => {
  beforeEach(() => {
    mockUseStore.mockReturnValue(defaultStore());
  });

  it('shows empty-state message when there are no entries', () => {
    render(<EntryList />);
    expect(screen.getByText(/no entries found/i)).toBeDefined();
  });

  it('shows loading indicator while loading', () => {
    mockUseStore.mockReturnValue(defaultStore({ isLoading: true }));
    render(<EntryList />);
    expect(screen.getByText(/loading/i)).toBeDefined();
  });

  it('renders entry name and URL for each entry', () => {
    const entries = [makeEntry('1', 'GitHub', 'https://github.com')];
    mockUseStore.mockReturnValue(defaultStore({ entries }));
    render(<EntryList />);
    expect(screen.getByText('GitHub')).toBeDefined();
    expect(screen.getByText('https://github.com')).toBeDefined();
  });

  it('renders copy buttons for each entry', () => {
    const entries = [makeEntry('1', 'GitHub', 'https://github.com')];
    mockUseStore.mockReturnValue(defaultStore({ entries }));
    render(<EntryList />);
    expect(screen.getByRole('button', { name: /username/i })).toBeDefined();
    expect(screen.getByRole('button', { name: /password/i })).toBeDefined();
  });

  it('calls copyField with entryId and "username" when Username button is clicked', async () => {
    const copyMock = vi.fn().mockResolvedValue(undefined);
    const entries = [makeEntry('42', 'GitHub')];
    mockUseStore.mockReturnValue(defaultStore({ entries, copyField: copyMock }));
    render(<EntryList />);

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /username/i }));
    });

    expect(copyMock).toHaveBeenCalledWith('42', 'username');
  });

  it('calls copyField with entryId and "password" when Password button is clicked', async () => {
    const copyMock = vi.fn().mockResolvedValue(undefined);
    const entries = [makeEntry('42', 'GitHub')];
    mockUseStore.mockReturnValue(defaultStore({ entries, copyField: copyMock }));
    render(<EntryList />);

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /^password$/i }));
    });

    expect(copyMock).toHaveBeenCalledWith('42', 'password');
  });

  it('filters the visible entries as the search query changes', async () => {
    const entries = [makeEntry('1', 'GitHub', 'https://github.com'), makeEntry('2', 'Google')];
    mockUseStore.mockReturnValue(defaultStore({ entries }));
    render(<EntryList />);

    fireEvent.change(screen.getByPlaceholderText(/grep entries/i), {
      target: { value: 'git' },
    });

    await waitFor(() => {
      expect(screen.getByText('GitHub')).toBeDefined();
      expect(screen.queryByText('Google')).toBeNull();
    });
  });

  it('groups entries under their folder and opens it on click', () => {
    const entries = [
      { ...makeEntry('1', 'WorkPass'), folder_id: 'f1' },
      makeEntry('2', 'RootPass'),
    ];
    const folders = [{ id: 'f1', name: 'Work', created_at: '' }];
    mockUseStore.mockReturnValue(defaultStore({ entries, folders }));
    render(<EntryList />);

    // Root view: folder row + root entry visible, foldered entry hidden
    expect(screen.getByText(/Work/)).toBeDefined();
    expect(screen.getByText('RootPass')).toBeDefined();
    expect(screen.queryByText('WorkPass')).toBeNull();

    // Open the folder → its entry becomes visible
    fireEvent.click(screen.getByText(/Work/));
    expect(screen.getByText('WorkPass')).toBeDefined();
  });

  it('renders multiple entries', () => {
    const entries = [makeEntry('1', 'GitHub'), makeEntry('2', 'Google')];
    mockUseStore.mockReturnValue(defaultStore({ entries }));
    render(<EntryList />);
    expect(screen.getByText('GitHub')).toBeDefined();
    expect(screen.getByText('Google')).toBeDefined();
  });
});

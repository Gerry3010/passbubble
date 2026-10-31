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
    usernames: {},
    currentHost: '',
    isLoading: false,
    error: null,
    load: vi.fn().mockResolvedValue(undefined),
    copyField: vi.fn().mockResolvedValue(undefined),
    toggleSite: vi.fn().mockResolvedValue(undefined),
    removeMatch: vi.fn().mockResolvedValue(undefined),
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

  it('shows "+ Site" and calls toggleSite when the host is not yet a match', async () => {
    const toggleMock = vi.fn().mockResolvedValue(undefined);
    const entries = [makeEntry('42', 'Apple', 'https://apple.com')];
    mockUseStore.mockReturnValue(defaultStore({ entries, currentHost: 'apple.com', toggleSite: toggleMock }));
    render(<EntryList />);

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /\+ Site/ }));
    });
    expect(toggleMock).toHaveBeenCalledWith('42');
  });

  it('shows the active host (✓) when it is already a match pattern', () => {
    const entries = [{ ...makeEntry('42', 'Apple'), match_patterns: ['apple.com'] }];
    mockUseStore.mockReturnValue(defaultStore({ entries, currentHost: 'apple.com' }));
    render(<EntryList />);
    expect(screen.getByRole('button', { name: /✓ apple\.com/ })).toBeDefined();
    expect(screen.queryByRole('button', { name: /\+ Site/ })).toBeNull();
  });

  it('search matches an entry via a wildcard match pattern', async () => {
    const entries = [{ ...makeEntry('7', 'GitHub'), match_patterns: ['*.github.com'] }];
    mockUseStore.mockReturnValue(defaultStore({ entries }));
    render(<EntryList />);

    fireEvent.change(screen.getByPlaceholderText(/grep entries/i), {
      target: { value: 'gist.github.com' },
    });
    await waitFor(() => expect(screen.getByText('GitHub')).toBeDefined());
  });

  it('search matches an entry by its (decrypted) username', async () => {
    const entries = [makeEntry('1', 'GitHub', 'https://github.com'), makeEntry('2', 'Google')];
    mockUseStore.mockReturnValue(defaultStore({ entries, usernames: { '1': 'octocat@example.com' } }));
    render(<EntryList />);

    fireEvent.change(screen.getByPlaceholderText(/grep entries/i), {
      target: { value: 'octocat' },
    });
    await waitFor(() => {
      expect(screen.getByText('GitHub')).toBeDefined();
      expect(screen.queryByText('Google')).toBeNull();
    });
  });

  it('toggles the match-patterns list and removes a pattern', async () => {
    const removeMock = vi.fn().mockResolvedValue(undefined);
    const entries = [{ ...makeEntry('42', 'Apple'), match_patterns: ['apple.com', 'id.apple.com'] }];
    mockUseStore.mockReturnValue(defaultStore({ entries, removeMatch: removeMock }));
    render(<EntryList />);

    // Collapsed by default; expand it.
    fireEvent.click(screen.getByRole('button', { name: /2 match sites/ }));
    expect(screen.getByText('id.apple.com')).toBeDefined();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Remove id\.apple\.com/ }));
    });
    expect(removeMock).toHaveBeenCalledWith('42', 'id.apple.com');
  });

  describe('current-site placeholder', () => {
    const entries = [
      { ...makeEntry('1', 'GitHub'), match_patterns: ['github.com'] },
      makeEntry('2', 'GitLab', 'https://gitlab.com'),
      makeEntry('3', 'Google'),
    ];

    it('shows the host as placeholder, keeps the field empty and lists its matches', () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, currentHost: 'github.com' }));
      render(<EntryList />);

      const search = screen.getByRole('searchbox', { name: 'Search entries' }) as HTMLInputElement;
      expect(search.value).toBe('');
      expect(search.placeholder).toBe('github.com');
      expect(screen.getByText('GitHub')).toBeDefined();
      expect(screen.queryByText('GitLab')).toBeNull();
      expect(screen.getByText(/matches for github\.com/)).toBeDefined();
    });

    it('replaces the host filter with whatever the user types', async () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, currentHost: 'github.com' }));
      render(<EntryList />);

      fireEvent.change(screen.getByRole('searchbox', { name: 'Search entries' }), {
        target: { value: 'gitlab' },
      });
      await waitFor(() => {
        expect(screen.getByText('GitLab')).toBeDefined();
        expect(screen.queryByText('GitHub')).toBeNull();
      });
      expect(screen.queryByText(/matches for/)).toBeNull();
    });

    it('"show all" drops the host filter and opens the folder browser', () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, currentHost: 'github.com' }));
      render(<EntryList />);

      fireEvent.click(screen.getByRole('button', { name: 'show all' }));
      expect(screen.getByText('GitHub')).toBeDefined();
      expect(screen.getByText('GitLab')).toBeDefined();
      expect(screen.getByText('Google')).toBeDefined();
      expect(screen.getByPlaceholderText(/grep entries/i)).toBeDefined();
    });

    it('falls back to the folder browser when nothing matches the host', () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, currentHost: 'example.org' }));
      render(<EntryList />);

      expect(screen.getByText('GitHub')).toBeDefined();
      expect(screen.getByText('Google')).toBeDefined();
      expect(screen.queryByText(/matches for/)).toBeNull();
      expect(screen.queryByText('No entries found')).toBeNull();
    });
  });

  describe('tags', () => {
    const entries = [
      { ...makeEntry('1', 'GitHub', 'https://github.com'), match_patterns: ['github.com'] },
      makeEntry('2', 'GitLab', 'https://gitlab.com'),
      makeEntry('3', 'Bank'),
      makeEntry('4', 'Untagged'),
    ];
    const entryTags = { '1': ['Work', 'client'], '2': ['work'], '3': ['private'] };

    function search(value: string) {
      fireEvent.change(screen.getByRole('searchbox', { name: 'Search entries' }), { target: { value } });
    }

    function chip(tag: string) {
      return screen.getByRole('button', { name: `#${tag}` });
    }

    it('lists the vault tags as chips, first spelling, sorted case-insensitively', () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, entryTags }));
      render(<EntryList />);
      const group = screen.getByRole('group', { name: 'Filter by tag' });
      const names = Array.from(group.querySelectorAll('button')).map((b) => b.textContent);
      expect(names).toEqual(['#client', '#private', '#Work']);
    });

    it("shows an entry's tags in its row", () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, entryTags }));
      render(<EntryList />);
      expect(screen.getByText('#Work #client')).toBeDefined();
    });

    it('shows no tag chips when the vault has no tags', () => {
      mockUseStore.mockReturnValue(defaultStore({ entries }));
      render(<EntryList />);
      expect(screen.queryByRole('group', { name: 'Filter by tag' })).toBeNull();
    });

    it('filters by a #tag token in the search box, case-insensitively', async () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, entryTags }));
      render(<EntryList />);
      search('#WORK');
      await waitFor(() => {
        expect(screen.getByText('GitHub')).toBeDefined();
        expect(screen.getByText('GitLab')).toBeDefined();
        expect(screen.queryByText('Bank')).toBeNull();
        expect(screen.queryByText('Untagged')).toBeNull();
      });
      expect(chip('Work').getAttribute('aria-pressed')).toBe('true');
    });

    it('combines several #tags (AND) with the text search', async () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, entryTags }));
      render(<EntryList />);

      search('#work #client');
      await waitFor(() => {
        expect(screen.getByText('GitHub')).toBeDefined();
        expect(screen.queryByText('GitLab')).toBeNull();
      });

      search('#work lab');
      await waitFor(() => {
        expect(screen.getByText('GitLab')).toBeDefined();
        expect(screen.queryByText('GitHub')).toBeNull();
      });
    });

    it('plain text search also finds tag text', async () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, entryTags }));
      render(<EntryList />);
      search('priv');
      await waitFor(() => {
        expect(screen.getByText('Bank')).toBeDefined();
        expect(screen.queryByText('GitHub')).toBeNull();
      });
    });

    it('finds nothing for a tag no entry has', async () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, entryTags }));
      render(<EntryList />);
      search('#nope');
      await waitFor(() => expect(screen.getByText('No entries found')).toBeDefined());
    });

    it('a chip toggles its tag filter on and off', () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, entryTags }));
      render(<EntryList />);

      fireEvent.click(chip('private'));
      expect(chip('private').getAttribute('aria-pressed')).toBe('true');
      expect(screen.getByText('Bank')).toBeDefined();
      expect(screen.queryByText('GitHub')).toBeNull();

      fireEvent.click(chip('private'));
      expect(chip('private').getAttribute('aria-pressed')).toBe('false');
      expect(screen.getByText('GitHub')).toBeDefined();
      expect(screen.getByText('Untagged')).toBeDefined();
    });

    it('chips and #tokens add up (AND)', async () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, entryTags }));
      render(<EntryList />);
      fireEvent.click(chip('client'));
      search('#work');
      await waitFor(() => {
        expect(screen.getByText('GitHub')).toBeDefined();
        expect(screen.queryByText('GitLab')).toBeNull();
      });
    });

    it('switching off a typed tag via its chip removes it from the search box', () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, entryTags }));
      render(<EntryList />);
      search('git #work');

      fireEvent.click(chip('Work'));
      const box = screen.getByRole('searchbox', { name: 'Search entries' }) as HTMLInputElement;
      expect(box.value).toBe('git');
      expect(chip('Work').getAttribute('aria-pressed')).toBe('false');
    });

    it('"clear" drops every tag filter but keeps the text', () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, entryTags }));
      render(<EntryList />);
      fireEvent.click(chip('client'));
      search('hub #work');

      fireEvent.click(screen.getByRole('button', { name: 'clear' }));
      const box = screen.getByRole('searchbox', { name: 'Search entries' }) as HTMLInputElement;
      expect(box.value).toBe('hub');
      expect(chip('client').getAttribute('aria-pressed')).toBe('false');
      expect(screen.queryByRole('button', { name: 'clear' })).toBeNull();
    });

    it('a chip replaces the current-site placeholder filter', () => {
      mockUseStore.mockReturnValue(defaultStore({ entries, entryTags, currentHost: 'github.com' }));
      render(<EntryList />);
      expect(screen.getByText(/matches for github\.com/)).toBeDefined();

      fireEvent.click(chip('Work'));
      expect(screen.queryByText(/matches for/)).toBeNull();
      expect(screen.getByText('GitHub')).toBeDefined();
      expect(screen.getByText('GitLab')).toBeDefined();
      expect(screen.getByPlaceholderText(/grep entries/i)).toBeDefined();
    });

    it('collapses a long tag list behind "+N more", keeping active chips visible', () => {
      const many = Array.from({ length: 11 }, (_, i) => `t${String(i).padStart(2, '0')}`);
      mockUseStore.mockReturnValue(defaultStore({ entries: [makeEntry('1', 'A')], entryTags: { '1': many } }));
      render(<EntryList />);

      expect(screen.queryByRole('button', { name: '#t10' })).toBeNull();
      fireEvent.click(screen.getByRole('button', { name: '+3 more' }));
      fireEvent.click(chip('t10'));
      fireEvent.click(screen.getByRole('button', { name: 'less' }));

      // Collapsed again: the first eight plus the active one.
      expect(chip('t10').getAttribute('aria-pressed')).toBe('true');
      expect(screen.queryByRole('button', { name: '#t09' })).toBeNull();
      expect(screen.getByRole('button', { name: '+2 more' })).toBeDefined();
    });
  });
});

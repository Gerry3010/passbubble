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

// The entries store loads the decrypted tags along with the vault listing.

import { describe, it, expect, vi, beforeEach } from 'vitest';

const responses = vi.hoisted(() => ({} as Record<string, unknown>));
const sent = vi.hoisted(() => [] as string[]);
const sendMessage = vi.hoisted(() =>
  vi.fn(async (msg: { type: string }) => {
    sent.push(msg.type);
    const r = responses[msg.type];
    if (r instanceof Error) throw r;
    return r;
  }),
);

vi.mock('webextension-polyfill', () => ({
  default: {
    runtime: { sendMessage },
    storage: { session: { get: vi.fn().mockResolvedValue({}), set: vi.fn(), remove: vi.fn() } },
    tabs: { query: vi.fn().mockResolvedValue([]) },
  },
}));

import { useEntriesStore } from '../store/entries.js';

const listing = [{ id: 'e1', owner_id: 'u1', type: 'password', name: 'GitHub', created_at: '', updated_at: '' }];

beforeEach(() => {
  for (const k of Object.keys(responses)) delete responses[k];
  sent.length = 0;
  useEntriesStore.setState({ entries: [], folders: [], entryTags: {}, isLoading: false, error: null });
  responses.SEARCH_ENTRIES = listing;
  responses.LIST_FOLDERS = [];
  responses.GET_USERNAMES = { usernames: {} };
});

describe('entries store: tags', () => {
  it('loads the tags per entry', async () => {
    responses.LIST_ENTRY_TAGS = { tags: { e1: ['work'] } };
    await useEntriesStore.getState().load();
    expect(useEntriesStore.getState().entryTags).toEqual({ e1: ['work'] });
    expect(useEntriesStore.getState().entries).toHaveLength(1);
  });

  it('asks for the tags only after the entry listing came back', async () => {
    let releaseListing: (v: unknown) => void = () => {};
    sendMessage.mockImplementationOnce(async (msg: { type: string }) => {
      sent.push(msg.type);
      return new Promise((resolve) => {
        releaseListing = resolve;
      });
    });
    responses.LIST_ENTRY_TAGS = { tags: {} };

    const loading = useEntriesStore.getState().load();
    await new Promise((r) => setTimeout(r, 0));
    expect(sent[0]).toBe('SEARCH_ENTRIES');
    expect(sent).not.toContain('LIST_ENTRY_TAGS');

    releaseListing(listing);
    await loading;
    expect(sent).toContain('LIST_ENTRY_TAGS');
  });

  it('renders the list without tags when the tag request fails', async () => {
    responses.LIST_ENTRY_TAGS = new Error('Background error');
    await useEntriesStore.getState().load();
    expect(useEntriesStore.getState().entryTags).toEqual({});
    expect(useEntriesStore.getState().entries).toHaveLength(1);
    expect(useEntriesStore.getState().error).toBeNull();
  });

  it('ignores a locked answer to the tag request', async () => {
    responses.LIST_ENTRY_TAGS = { locked: true };
    await useEntriesStore.getState().load();
    expect(useEntriesStore.getState().entryTags).toEqual({});
  });
});

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

import { create } from 'zustand';
import browser from 'webextension-polyfill';
import { MessageType } from '../../shared/constants.js';
import type { EntryResponse, FolderResponse } from '@passbubble/shared-ts';

interface EntriesState {
  entries: EntryResponse[];
  folders: FolderResponse[];
  /** Host of the active tab (sans leading "www."), used to pre-filter on open. */
  currentHost: string;
  isLoading: boolean;
  error: string | null;
  /** Fetch the full vault (entries + folders) and the active-tab host. */
  load: () => Promise<void>;
  copyField: (entryId: string, field: 'username' | 'password') => Promise<void>;
}

/** The folders endpoint returns a tree (roots with nested `children`); the
 * popup browses level-by-level via parent_id, so flatten it to a single list. */
function flattenFolders(tree: FolderResponse[]): FolderResponse[] {
  const out: FolderResponse[] = [];
  const walk = (nodes: FolderResponse[]) => {
    for (const n of nodes) {
      out.push(n);
      if (n.children?.length) walk(n.children);
    }
  };
  walk(tree);
  return out;
}

/** Best-effort host of the currently active tab. Empty string if unavailable. */
async function activeTabHost(): Promise<string> {
  try {
    const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
    if (!tab?.url) return '';
    return new URL(tab.url).hostname.replace(/^www\./, '');
  } catch {
    return '';
  }
}

export const useEntriesStore = create<EntriesState>((set) => ({
  entries: [],
  folders: [],
  currentHost: '',
  isLoading: false,
  error: null,

  load: async () => {
    set({ isLoading: true, error: null });
    try {
      const [entriesResp, foldersResp, host] = await Promise.all([
        browser.runtime.sendMessage({ type: MessageType.SEARCH_ENTRIES, payload: { query: '' } }),
        browser.runtime.sendMessage({ type: MessageType.LIST_FOLDERS, payload: {} }),
        activeTabHost(),
      ]);
      set({
        entries: Array.isArray(entriesResp) ? (entriesResp as EntryResponse[]) : [],
        folders: Array.isArray(foldersResp) ? flattenFolders(foldersResp as FolderResponse[]) : [],
        currentHost: host,
        isLoading: false,
      });
    } catch (e) {
      set({ isLoading: false, error: String(e) });
    }
  },

  copyField: async (entryId, field) => {
    const resp = await browser.runtime.sendMessage({
      type: MessageType.GET_ENTRY,
      payload: { id: entryId },
    }) as { data?: { username?: string; password?: string }; locked?: boolean };
    if (resp.locked || !resp.data) return;
    const value = field === 'username' ? (resp.data.username ?? '') : (resp.data.password ?? '');
    await navigator.clipboard.writeText(value);
  },
}));

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
  /** Decrypted usernames keyed by entry id, so search can match usernames
   * (which are E2E-encrypted and not part of the metadata entry list). */
  usernames: Record<string, string>;
  /** Host of the active tab (sans leading "www."), used to pre-filter on open. */
  currentHost: string;
  isLoading: boolean;
  error: string | null;
  /** Fetch the full vault (entries + folders) and the active-tab host. */
  load: () => Promise<void>;
  copyField: (entryId: string, field: 'username' | 'password') => Promise<void>;
  /** Add the active-tab host to the entry's match patterns, or remove it if the
   * exact host is already present (no wildcard handling — kept deliberately simple). */
  toggleSite: (entryId: string) => Promise<void>;
  /** Remove a single match pattern from an entry. */
  removeMatch: (entryId: string, pattern: string) => Promise<void>;
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

/** Best-effort host for pre-filling search + the "+ Site" toggle. Prefers the
 * host of the frame that actually has the login form (reported by the content
 * script — often an SSO iframe), falling back to the active tab's top URL. */
async function activeTabHost(): Promise<string> {
  try {
    const resp = (await browser.runtime.sendMessage({
      type: MessageType.GET_FILL_HOST,
      payload: {},
    })) as { host?: string };
    if (resp?.host) return resp.host;
  } catch {
    /* fall through to the top tab URL */
  }
  try {
    const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
    // Only web pages have a meaningful host to pre-fill; skip chrome://, about:,
    // moz-extension://, file://, etc. so the search field is not seeded with junk.
    if (!tab?.url || !/^https?:\/\//.test(tab.url)) return '';
    return new URL(tab.url).hostname.replace(/^www\./, '');
  } catch {
    return '';
  }
}

export const useEntriesStore = create<EntriesState>((set, get) => ({
  entries: [],
  folders: [],
  usernames: {},
  currentHost: '',
  isLoading: false,
  error: null,

  load: async () => {
    set({ isLoading: true, error: null });
    try {
      const [entriesResp, foldersResp, host, usernamesResp] = await Promise.all([
        browser.runtime.sendMessage({ type: MessageType.SEARCH_ENTRIES, payload: { query: '' } }),
        browser.runtime.sendMessage({ type: MessageType.LIST_FOLDERS, payload: {} }),
        activeTabHost(),
        // Best-effort: search-by-username works once these resolve; a failure
        // here must not block the entry list from rendering.
        browser.runtime
          .sendMessage({ type: MessageType.GET_USERNAMES, payload: {} })
          .catch(() => ({ usernames: {} })),
      ]);
      const usernames =
        usernamesResp && typeof usernamesResp === 'object' && 'usernames' in usernamesResp
          ? ((usernamesResp as { usernames?: Record<string, string> }).usernames ?? {})
          : {};
      set({
        entries: Array.isArray(entriesResp) ? (entriesResp as EntryResponse[]) : [],
        folders: Array.isArray(foldersResp) ? flattenFolders(foldersResp as FolderResponse[]) : [],
        currentHost: host,
        usernames,
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

  toggleSite: async (entryId) => {
    const { entries, currentHost } = get();
    if (!currentHost) return;
    const entry = entries.find((e) => e.id === entryId);
    if (!entry) return;
    const current = entry.match_patterns ?? [];
    const next = current.includes(currentHost)
      ? current.filter((p) => p !== currentHost)
      : [...current, currentHost];
    await persistMatchPatterns(set, get, entry, next);
  },

  removeMatch: async (entryId, pattern) => {
    const { entries } = get();
    const entry = entries.find((e) => e.id === entryId);
    if (!entry) return;
    const next = (entry.match_patterns ?? []).filter((p) => p !== pattern);
    await persistMatchPatterns(set, get, entry, next);
  },
}));

/** Persist a new match-pattern list for an entry and reflect it locally. The
 * entry's folder is echoed back because the backend always overwrites folder_id. */
async function persistMatchPatterns(
  set: (partial: Partial<EntriesState>) => void,
  get: () => EntriesState,
  entry: EntryResponse,
  next: string[],
): Promise<void> {
  await browser.runtime.sendMessage({
    type: MessageType.UPDATE_ENTRY,
    payload: { entryId: entry.id, matchPatterns: next, folderId: entry.folder_id },
  });
  set({
    entries: get().entries.map((e) => (e.id === entry.id ? { ...e, match_patterns: next } : e)),
  });
}

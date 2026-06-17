import { create } from 'zustand';
import browser from 'webextension-polyfill';
import { MessageType } from '../../shared/constants.js';
import type { EntryResponse } from '@passbubble/shared-ts';

interface EntriesState {
  entries: EntryResponse[];
  isLoading: boolean;
  error: string | null;
  search: (query: string) => Promise<void>;
  copyField: (entryId: string, field: 'username' | 'password') => Promise<void>;
}

export const useEntriesStore = create<EntriesState>((set) => ({
  entries: [],
  isLoading: false,
  error: null,

  search: async (query) => {
    set({ isLoading: true, error: null });
    try {
      const resp = await browser.runtime.sendMessage({
        type: MessageType.SEARCH_ENTRIES,
        payload: { query },
      });
      if (Array.isArray(resp)) {
        set({ entries: resp as EntryResponse[], isLoading: false });
      } else {
        set({ isLoading: false, entries: [] });
      }
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

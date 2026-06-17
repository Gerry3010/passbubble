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
import type { SessionInfo } from '@passbubble/shared-ts';

interface SessionState extends SessionInfo {
  serverUrl: string;
  isLoading: boolean;
  error: string | null;

  checkSession: () => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  unlock: (masterPassword: string) => Promise<void>;
  lock: () => Promise<void>;
  clearError: () => void;
}

export const useSessionStore = create<SessionState>((set) => ({
  isLoggedIn: false,
  isUnlocked: false,
  userEmail: undefined,
  userName: undefined,
  serverUrl: '',
  isLoading: false,
  error: null,

  checkSession: async () => {
    set({ isLoading: true, error: null });
    try {
      const resp = await browser.runtime.sendMessage({
        type: MessageType.GET_SESSION,
        payload: {},
      }) as SessionInfo & { serverUrl: string };
      set({ ...resp, isLoading: false });
    } catch {
      set({ isLoading: false, error: 'Failed to connect to extension background' });
    }
  },

  login: async (email, password) => {
    set({ isLoading: true, error: null });
    try {
      await browser.runtime.sendMessage({
        type: MessageType.LOGIN,
        payload: { email, password },
      });
      set({ isLoggedIn: true, isLoading: false });
    } catch (e) {
      set({ isLoading: false, error: String(e) });
    }
  },

  unlock: async (masterPassword) => {
    set({ isLoading: true, error: null });
    try {
      await browser.runtime.sendMessage({
        type: MessageType.UNLOCK,
        payload: { masterPassword },
      });
      // Re-fetch session state
      const resp = await browser.runtime.sendMessage({
        type: MessageType.GET_SESSION,
        payload: {},
      }) as SessionInfo & { serverUrl: string };
      set({ ...resp, isLoading: false });
    } catch (e) {
      set({ isLoading: false, error: String(e) });
    }
  },

  lock: async () => {
    await browser.runtime.sendMessage({ type: MessageType.LOCK, payload: {} });
    set({ isUnlocked: false, error: null });
  },

  clearError: () => set({ error: null }),
}));

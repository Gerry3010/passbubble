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
import { MessageType, STORAGE_KEYS } from '../../shared/constants.js';
import type { SessionInfo } from '@passbubble/shared-ts';

/** Drop the in-progress login draft + 2FA state from session storage. */
async function clearAuthDraft(): Promise<void> {
  try {
    await browser.storage.session.remove([STORAGE_KEYS.AUTH_DRAFT, STORAGE_KEYS.PENDING_2FA]);
  } catch {
    // session storage may be unavailable in some contexts — best effort only
  }
}

interface SessionState extends SessionInfo {
  serverUrl: string;
  isLoading: boolean;
  error: string | null;
  /** True after the password step when the account requires a 2FA code. */
  totpRequired: boolean;
  pendingToken: string | null;

  checkSession: () => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  verifyTotp: (code: string) => Promise<void>;
  cancelTotp: () => void;
  unlock: (masterPassword: string) => Promise<void>;
  lock: () => Promise<void>;
  logout: () => Promise<void>;
  clearError: () => void;
}

export const useSessionStore = create<SessionState>((set, get) => ({
  isLoggedIn: false,
  isUnlocked: false,
  userEmail: undefined,
  userName: undefined,
  serverUrl: '',
  isLoading: false,
  error: null,
  totpRequired: false,
  pendingToken: null,

  checkSession: async () => {
    set({ isLoading: true, error: null });
    try {
      const resp = await browser.runtime.sendMessage({
        type: MessageType.GET_SESSION,
        payload: {},
      }) as SessionInfo & { serverUrl: string };
      // Restore a pending 2FA step if the popup was closed mid-login, so the
      // user lands back on the code prompt instead of starting over.
      let totp: { totpRequired: boolean; pendingToken: string | null } = {
        totpRequired: false,
        pendingToken: null,
      };
      if (!resp.isLoggedIn) {
        try {
          const stored = await browser.storage.session.get(STORAGE_KEYS.PENDING_2FA);
          const token = stored[STORAGE_KEYS.PENDING_2FA] as string | undefined;
          if (token) totp = { totpRequired: true, pendingToken: token };
        } catch {
          // ignore — fall back to a fresh login
        }
      }
      set({ ...resp, ...totp, isLoading: false });
    } catch {
      set({ isLoading: false, error: 'Failed to connect to extension background' });
    }
  },

  login: async (email, password) => {
    set({ isLoading: true, error: null });
    try {
      const resp = (await browser.runtime.sendMessage({
        type: MessageType.LOGIN,
        payload: { email, password },
      })) as { requiresTotp?: boolean; pendingToken?: string };
      if (resp?.requiresTotp) {
        // Password step done — keep only the pending-2FA token (not the
        // password) so the code prompt survives the popup closing.
        try {
          await browser.storage.session.set({ [STORAGE_KEYS.PENDING_2FA]: resp.pendingToken ?? '' });
          await browser.storage.session.remove(STORAGE_KEYS.AUTH_DRAFT);
        } catch {
          // best effort
        }
        set({
          isLoading: false,
          totpRequired: true,
          pendingToken: resp.pendingToken ?? null,
        });
        return;
      }
      await clearAuthDraft();
      set({ isLoggedIn: true, isLoading: false });
    } catch (e) {
      set({ isLoading: false, error: String(e) });
    }
  },

  verifyTotp: async (code) => {
    const pendingToken = get().pendingToken;
    if (!pendingToken) return;
    set({ isLoading: true, error: null });
    try {
      await browser.runtime.sendMessage({
        type: MessageType.VERIFY_TOTP,
        payload: { pendingToken, code },
      });
      await clearAuthDraft();
      set({
        isLoggedIn: true,
        isLoading: false,
        totpRequired: false,
        pendingToken: null,
      });
    } catch (e) {
      set({ isLoading: false, error: String(e) });
    }
  },

  cancelTotp: () => {
    void clearAuthDraft();
    set({ totpRequired: false, pendingToken: null, error: null });
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

  logout: async () => {
    await browser.runtime.sendMessage({ type: MessageType.LOGOUT, payload: {} });
    await clearAuthDraft();
    set({
      isLoggedIn: false,
      isUnlocked: false,
      totpRequired: false,
      pendingToken: null,
      userEmail: undefined,
      userName: undefined,
      error: null,
    });
  },

  clearError: () => set({ error: null }),
}));

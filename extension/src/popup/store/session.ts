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

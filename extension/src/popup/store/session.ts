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
import type { LockEvent } from '../../shared/lock-info.js';
import type { SessionInfo } from '@passbubble/shared-ts';

/** Drop the in-progress login draft + 2FA state from session storage. */
async function clearAuthDraft(): Promise<void> {
  try {
    await browser.storage.session.remove([STORAGE_KEYS.AUTH_DRAFT, STORAGE_KEYS.PENDING_2FA]);
  } catch {
    // session storage may be unavailable in some contexts — best effort only
  }
}

type SessionResponse = SessionInfo & {
  serverUrl: string;
  pinEnabled?: boolean;
  pinAvailable?: boolean;
  lastLock?: LockEvent | null;
};

/** UNLOCK / UNLOCK_WITH_PIN failure details from the background. */
interface UnlockResponse {
  ok?: boolean;
  error?: string;
  /** The server rejected the refresh token — the device was signed out. */
  loggedOut?: boolean;
}

/** GET_SESSION plus a pending 2FA step (restored if the popup was closed
 * mid-login, so the user lands back on the code prompt instead of starting over). */
async function fetchSessionState(): Promise<
  SessionResponse & { totpRequired: boolean; pendingToken: string | null }
> {
  const resp = (await browser.runtime.sendMessage({
    type: MessageType.GET_SESSION,
    payload: {},
  })) as SessionResponse;
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
  return { ...resp, ...totp };
}

interface SessionState extends SessionInfo {
  serverUrl: string;
  isLoading: boolean;
  error: string | null;
  /** True after the password step when the account requires a 2FA code. */
  totpRequired: boolean;
  pendingToken: string | null;
  /** A PIN is configured on this device. */
  pinEnabled: boolean;
  /** The PIN may be used to unlock right now (configured and within its interval). */
  pinAvailable: boolean;
  /** Why the vault is locked, shown on the unlock screen. */
  lastLock: LockEvent | null;

  checkSession: () => Promise<void>;
  /** Re-read the session state without the loading screen — used when the
   * background reports (or storage shows) that the vault was locked or the
   * device signed out, so the popup switches to the unlock / login screen. */
  syncSession: () => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  verifyTotp: (code: string) => Promise<void>;
  cancelTotp: () => void;
  unlock: (masterPassword: string) => Promise<void>;
  unlockWithPin: (pin: string) => Promise<void>;
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
  pinEnabled: false,
  pinAvailable: false,
  lastLock: null,

  checkSession: async () => {
    set({ isLoading: true, error: null });
    try {
      set({ ...(await fetchSessionState()), isLoading: false });
    } catch {
      set({ isLoading: false, error: 'Failed to connect to extension background' });
    }
  },

  syncSession: async () => {
    try {
      set(await fetchSessionState());
    } catch {
      // background unreachable — keep the current state
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
      const resp = (await browser.runtime.sendMessage({
        type: MessageType.UNLOCK,
        payload: { masterPassword },
      })) as UnlockResponse | undefined;
      // Re-fetch session state — after a rejected refresh token the device is
      // signed out and App switches to the login form (showing the error).
      const s = (await browser.runtime.sendMessage({
        type: MessageType.GET_SESSION,
        payload: {},
      })) as SessionResponse;
      set({ ...s, isLoading: false, error: resp?.ok === false ? (resp.error ?? 'Unlock failed') : null });
    } catch (e) {
      set({ isLoading: false, error: e instanceof Error ? e.message : String(e) });
    }
  },

  unlockWithPin: async (pin) => {
    set({ isLoading: true, error: null });
    try {
      const resp = (await browser.runtime.sendMessage({
        type: MessageType.UNLOCK_WITH_PIN,
        payload: { pin },
      })) as UnlockResponse & {
        expired?: boolean;
        wrongPin?: boolean;
        lockedOut?: boolean;
        triesRemaining?: number;
      };
      if (!resp.ok) {
        if (resp.expired) {
          set({ isLoading: false, pinAvailable: false, error: 'PIN expired — enter your master password' });
        } else if (resp.lockedOut) {
          set({
            isLoading: false,
            pinAvailable: false,
            pinEnabled: false,
            error: 'Too many incorrect attempts — PIN removed. Enter your master password.',
          });
        } else if (resp.wrongPin) {
          set({ isLoading: false, error: `Incorrect PIN — ${resp.triesRemaining ?? 0} attempt(s) left` });
        } else if (resp.loggedOut) {
          set({ ...(await fetchSessionState()), isLoading: false, error: resp.error ?? 'Please log in again.' });
        } else {
          set({ isLoading: false, error: resp.error ?? 'PIN unlock failed' });
        }
        return;
      }
      const s = (await browser.runtime.sendMessage({
        type: MessageType.GET_SESSION,
        payload: {},
      })) as SessionResponse;
      set({ ...s, isLoading: false });
    } catch (e) {
      set({ isLoading: false, error: e instanceof Error ? e.message : String(e) });
    }
  },

  lock: async () => {
    await browser.runtime.sendMessage({ type: MessageType.LOCK, payload: {} });
    set({ isUnlocked: false, error: null, lastLock: { reason: 'manual', at: Date.now() } });
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

/**
 * Call with any background response: if it says the vault is locked (or the
 * device was signed out), refresh the session state so App switches to the
 * unlock / login screen. Returns true when the response was such a "locked" one.
 */
export function handleLockedResponse(resp: unknown): boolean {
  if (!resp || typeof resp !== 'object') return false;
  const r = resp as { locked?: unknown; loggedOut?: unknown };
  if (!r.locked && !r.loggedOut) return false;
  void useSessionStore
    .getState()
    .syncSession()
    .then(() => {
      if (r.loggedOut) useSessionStore.setState({ error: 'Your session has expired — please log in again.' });
    });
  return true;
}

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

// Handles all chrome.runtime.onMessage messages from popup and content script.
// All crypto operations live here — never in content scripts or popup.

import browser from 'webextension-polyfill';
import { PassbubbleClient, unlock, decryptEntry, encryptEntry, createEntry } from '@passbubble/shared-ts';
import { b64Dec, b64Enc } from '../shared/utils.js';
import { MessageType, STORAGE_KEYS } from '../shared/constants.js';
import {
  clearSession,
  getEntriesCache,
  getSession,
  setEntriesCache,
  setSession,
} from './session-store.js';
import { matchEntriesForUrl } from './autofill-service.js';
import type { EntryData, LoginResponse, SessionInfo, UnlockedSession } from '@passbubble/shared-ts';

function makeClient(serverUrl: string, accessToken?: string): PassbubbleClient {
  const c = new PassbubbleClient(serverUrl);
  if (accessToken) c.setTokens(accessToken, '', 900);
  return c;
}

async function getServerUrl(): Promise<string> {
  const data = await browser.storage.sync.get(STORAGE_KEYS.SERVER_URL);
  return (data[STORAGE_KEYS.SERVER_URL] as string | undefined) ?? '';
}

async function loadSessionData() {
  return browser.storage.session.get([
    STORAGE_KEYS.REFRESH_TOKEN,
    STORAGE_KEYS.ENC_PRIV_X25519,
    STORAGE_KEYS.ENC_PRIV_MLKEM,
    STORAGE_KEYS.KDF_SALT,
    STORAGE_KEYS.KDF_TIME,
    STORAGE_KEYS.KDF_MEMORY,
    STORAGE_KEYS.USER_ID,
    STORAGE_KEYS.USER_EMAIL,
    STORAGE_KEYS.USER_NAME,
    STORAGE_KEYS.ROLE,
    STORAGE_KEYS.PUB_X25519,
    STORAGE_KEYS.PUB_MLKEM,
  ]);
}

/**
 * Persists the non-secret session material from a successful login (the master
 * password and plaintext private keys are never stored). Shared by the LOGIN
 * and VERIFY_TOTP handlers.
 */
async function persistLoginResponse(resp: LoginResponse): Promise<void> {
  await browser.storage.session.set({
    [STORAGE_KEYS.REFRESH_TOKEN]: resp.refresh_token,
    [STORAGE_KEYS.ENC_PRIV_X25519]: resp.enc_priv_x25519,
    [STORAGE_KEYS.ENC_PRIV_MLKEM]: resp.enc_priv_mlkem768,
    [STORAGE_KEYS.KDF_SALT]: resp.kdf_salt,
    [STORAGE_KEYS.KDF_TIME]: resp.kdf_time,
    [STORAGE_KEYS.KDF_MEMORY]: resp.kdf_memory,
    [STORAGE_KEYS.USER_ID]: resp.user_id,
    [STORAGE_KEYS.USER_EMAIL]: resp.email,
    [STORAGE_KEYS.USER_NAME]: resp.name,
    [STORAGE_KEYS.ROLE]: resp.role,
    [STORAGE_KEYS.PUB_X25519]: resp.pub_x25519,
    [STORAGE_KEYS.PUB_MLKEM]: resp.pub_mlkem768,
  });
}

type Handler = (payload: Record<string, unknown>) => Promise<unknown>;

export function buildHandlers(): Record<string, Handler> {
  return {
    [MessageType.GET_SESSION]: async () => {
      const stored = await loadSessionData();
      const isLoggedIn = !!stored[STORAGE_KEYS.REFRESH_TOKEN];
      const session = getSession();
      const serverUrl = await getServerUrl();
      return {
        isLoggedIn,
        isUnlocked: !!session,
        userEmail: (stored[STORAGE_KEYS.USER_EMAIL] as string) ?? session?.userEmail,
        userName: (stored[STORAGE_KEYS.USER_NAME] as string) ?? session?.userName,
        serverUrl,
      } satisfies SessionInfo & { serverUrl: string };
    },

    [MessageType.LOGIN]: async (payload) => {
      const { email, password } = payload as { email: string; password: string };
      const serverUrl = await getServerUrl();
      if (!serverUrl) throw new Error('Server URL not configured');
      const client = makeClient(serverUrl);
      const resp = await client.login(email, password);
      // Account-level 2FA: the password step succeeded but a TOTP code is
      // required. Do NOT persist anything yet (the response carries no keys).
      if (resp.status === '2fa_required') {
        return { ok: true, requiresTotp: true, pendingToken: resp.pending_token };
      }
      await persistLoginResponse(resp);
      return { ok: true, needsUnlock: true };
    },

    [MessageType.VERIFY_TOTP]: async (payload) => {
      const { pendingToken, code } = payload as { pendingToken: string; code: string };
      const serverUrl = await getServerUrl();
      if (!serverUrl) throw new Error('Server URL not configured');
      const client = makeClient(serverUrl);
      const resp = await client.verifyTotp(pendingToken, code);
      await persistLoginResponse(resp);
      return { ok: true, needsUnlock: true };
    },

    [MessageType.UNLOCK]: async (payload) => {
      const { masterPassword } = payload as { masterPassword: string };
      const stored = await loadSessionData();
      const serverUrl = await getServerUrl();
      if (!stored[STORAGE_KEYS.REFRESH_TOKEN]) throw new Error('Not logged in');

      const kdfParams = {
        salt: b64Dec(stored[STORAGE_KEYS.KDF_SALT] as string),
        time: stored[STORAGE_KEYS.KDF_TIME] as number,
        memory: stored[STORAGE_KEYS.KDF_MEMORY] as number,
      };
      const { privX25519, privMLKEM } = await unlock(
        masterPassword,
        kdfParams,
        stored[STORAGE_KEYS.ENC_PRIV_X25519] as string,
        stored[STORAGE_KEYS.ENC_PRIV_MLKEM] as string,
      );

      // Refresh access token
      const client = makeClient(serverUrl);
      const refreshResp = await client.refresh(stored[STORAGE_KEYS.REFRESH_TOKEN] as string);
      await browser.storage.session.set({ [STORAGE_KEYS.REFRESH_TOKEN]: refreshResp.refresh_token });

      const session: UnlockedSession = {
        privX25519,
        privMLKEM,
        pubX25519: b64Dec(stored[STORAGE_KEYS.PUB_X25519] as string),
        pubMLKEM: b64Dec(stored[STORAGE_KEYS.PUB_MLKEM] as string),
        userId: stored[STORAGE_KEYS.USER_ID] as string,
        userEmail: stored[STORAGE_KEYS.USER_EMAIL] as string,
        userName: stored[STORAGE_KEYS.USER_NAME] as string,
        role: stored[STORAGE_KEYS.ROLE] as string,
        accessToken: refreshResp.access_token,
        refreshToken: refreshResp.refresh_token,
        accessTokenExpiresAt: Date.now() + refreshResp.expires_in * 1000,
        serverUrl,
        kdfSalt: kdfParams.salt,
        kdfTime: kdfParams.time,
        kdfMemory: kdfParams.memory,
        encPrivX25519: b64Dec(stored[STORAGE_KEYS.ENC_PRIV_X25519] as string),
        encPrivMLKEM: b64Dec(stored[STORAGE_KEYS.ENC_PRIV_MLKEM] as string),
      };
      setSession(session);

      // Pre-load entry list into cache
      const apiClient = makeClient(serverUrl, session.accessToken);
      apiClient.setTokens(session.accessToken, session.refreshToken, refreshResp.expires_in);
      const entries = await apiClient.listEntries();
      setEntriesCache(entries);

      // Schedule token refresh 60s before expiry
      await browser.alarms.create('token-refresh', {
        delayInMinutes: (refreshResp.expires_in - 60) / 60,
      });

      return { ok: true };
    },

    [MessageType.LOCK]: async () => {
      clearSession();
      await browser.alarms.clear('token-refresh');
      return { ok: true };
    },

    // Full sign-out: unlike LOCK (which only drops the in-memory keys but keeps
    // you logged in), this wipes the refresh token and all session storage so
    // the next popup starts at the login screen.
    [MessageType.LOGOUT]: async () => {
      clearSession();
      await browser.alarms.clear('token-refresh');
      await browser.storage.session.clear();
      return { ok: true };
    },

    [MessageType.SEARCH_ENTRIES]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { query } = payload as { query: string };
      const client = makeClient(session.serverUrl, session.accessToken);
      // The popup loads the full vault with an empty query; the backend search
      // endpoint deliberately returns [] for a blank q, so list everything
      // instead (and refresh the cache used for URL matching while we're here).
      if (!query.trim()) {
        const entries = await client.listEntries();
        setEntriesCache(entries);
        return entries;
      }
      return client.searchEntries(query);
    },

    [MessageType.LIST_FOLDERS]: async () => {
      const session = getSession();
      if (!session) return { locked: true };
      const client = makeClient(session.serverUrl, session.accessToken);
      return client.listFolders();
    },

    [MessageType.GET_ENTRY]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { id } = payload as { id: string };
      const client = makeClient(session.serverUrl, session.accessToken);
      const apiEntry = await client.getEntry(id);
      const data = await decryptEntry(apiEntry, session);
      return { entry: apiEntry, data };
    },

    [MessageType.FILL_ENTRY]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { entryId } = payload as { entryId: string };
      const client = makeClient(session.serverUrl, session.accessToken);
      const apiEntry = await client.getEntry(entryId);
      const data = await decryptEntry(apiEntry, session);
      return { username: data.username ?? '', password: data.password ?? '' };
    },

    [MessageType.GET_MATCHES_FOR_URL]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { url } = payload as { url: string };
      const cache = getEntriesCache();
      if (!cache) return [];
      return matchEntriesForUrl(url, cache);
    },

    [MessageType.CREATE_ENTRY]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { name, type, url, data, folderId, matchPatterns } = payload as {
        name: string;
        type: string;
        url?: string;
        data: EntryData;
        folderId?: string;
        matchPatterns?: string[];
      };
      const client = makeClient(session.serverUrl, session.accessToken);
      const result = await createEntry(client, name, type, url, data, session, folderId, matchPatterns);
      // Invalidate cache
      const entries = await client.listEntries();
      setEntriesCache(entries);
      return result;
    },

    [MessageType.GENERATE]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const client = makeClient(session.serverUrl, session.accessToken);
      return client.generate(payload as Parameters<typeof client.generate>[0]);
    },

    // A form submission was detected on a page with no matching entry. Stash the
    // credentials so the popup can offer to save them (consent-based — we never
    // create an entry silently).
    [MessageType.OFFER_SAVE]: async (payload) => {
      const { name, url, username, password } = payload as {
        name: string;
        url: string;
        username: string;
        password: string;
      };
      await browser.storage.session.set({
        [STORAGE_KEYS.PENDING_SAVE]: { name, url, username, password },
      });
      return { ok: true };
    },

    [MessageType.GET_PENDING_SAVE]: async () => {
      const stored = await browser.storage.session.get(STORAGE_KEYS.PENDING_SAVE);
      return (stored[STORAGE_KEYS.PENDING_SAVE] as unknown) ?? null;
    },

    // The user confirmed the save in the popup: create the entry and clear the
    // pending offer.
    [MessageType.CONFIRM_SAVE]: async () => {
      const session = getSession();
      if (!session) return { locked: true };
      const stored = await browser.storage.session.get(STORAGE_KEYS.PENDING_SAVE);
      const pending = stored[STORAGE_KEYS.PENDING_SAVE] as
        | { name: string; url: string; username: string; password: string }
        | undefined;
      if (!pending) return { ok: false };

      const client = makeClient(session.serverUrl, session.accessToken);
      const data = { username: pending.username, password: pending.password } as EntryData;
      const result = await createEntry(client, pending.name || pending.url, 'password', pending.url, data, session);
      setEntriesCache(await client.listEntries());
      await browser.storage.session.remove(STORAGE_KEYS.PENDING_SAVE);
      return { ok: true, ...result };
    },

    [MessageType.DISMISS_SAVE]: async (payload) => {
      const { host } = payload as { host?: string };
      await browser.storage.session.remove(STORAGE_KEYS.PENDING_SAVE);
      if (host) {
        const stored = await browser.storage.session.get(STORAGE_KEYS.DISMISSED_SAVE_HOSTS);
        const list = (stored[STORAGE_KEYS.DISMISSED_SAVE_HOSTS] as string[] | undefined) ?? [];
        if (!list.includes(host)) {
          await browser.storage.session.set({ [STORAGE_KEYS.DISMISSED_SAVE_HOSTS]: [...list, host] });
        }
      }
      return { ok: true };
    },
  };
}

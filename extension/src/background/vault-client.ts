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

// Central token handling for the background. Every refresh — unlock, the
// token-refresh alarm, an API call that hit a 401 — goes through ONE
// single-flight refreshTokens(), which persists the rotated refresh token before
// anything else happens. API calls use vaultClient(), which refreshes an expiring
// access token up front and retries a 401 once after refreshing.

import browser from 'webextension-polyfill';
import { PassbubbleClient } from '@passbubble/shared-ts';
import type { EntryResponse, RefreshResponse, UnlockedSession } from '@passbubble/shared-ts';
import { STORAGE_KEYS, AUTO_LOCK_ALARM, TOKEN_REFRESH_ALARM } from '../shared/constants.js';
import { clearSession, getEntriesCache, getSession, setEntriesCache, setSession } from './session-store.js';
import { clearAccount, loadAccount, saveRefreshToken } from './account-store.js';
import { clearPin } from './pin-store.js';

/** The vault is locked — or, with `loggedOut`, the device was signed out because
 * the server rejected the refresh token. Handlers turn it into `{ locked: true }`. */
export class VaultLockedError extends Error {
  constructor(readonly loggedOut: boolean) {
    super(loggedOut ? 'Your session has expired — please log in again.' : 'Vault is locked');
    this.name = 'VaultLockedError';
  }
}

/** HTTP status of an ApiError (duck-typed so callers need no class import). */
export function httpStatus(err: unknown): number | undefined {
  const status = (err as { status?: unknown } | null)?.status;
  return typeof status === 'number' ? status : undefined;
}

export async function getServerUrl(): Promise<string> {
  const data = await browser.storage.sync.get(STORAGE_KEYS.SERVER_URL);
  return (data[STORAGE_KEYS.SERVER_URL] as string | undefined) ?? '';
}

/** Lock the vault: drop the unlocked session (memory + storage.session) and the
 * alarms that only make sense while unlocked. The login state is kept. */
export async function lockVault(): Promise<void> {
  await clearSession();
  await browser.alarms.clear(TOKEN_REFRESH_ALARM);
  await browser.alarms.clear(AUTO_LOCK_ALARM);
  try {
    await browser.storage.session.remove(STORAGE_KEYS.LAST_ACTIVITY);
  } catch {
    /* ignore */
  }
}

/** Sign this device out locally: lock, forget the login state and the PIN (a
 * signed-out device must never keep a PIN-unlockable master key around). */
export async function endLogin(): Promise<void> {
  await lockVault();
  await clearAccount();
  await clearPin();
}

export async function scheduleTokenRefresh(expiresInSeconds: number): Promise<void> {
  await browser.alarms.create(TOKEN_REFRESH_ALARM, {
    delayInMinutes: Math.max(1, (expiresInSeconds - 60) / 60),
  });
}

let inflightRefresh: Promise<RefreshResponse> | null = null;

/**
 * Exchange the stored refresh token for new tokens. Single-flight: concurrent
 * callers share one request, since the server invalidates the old token on use.
 *   - success: the rotated refresh token is persisted FIRST, then the live
 *     session (if unlocked) gets the new access token and the alarm is re-armed;
 *   - 401 (refresh token expired / revoked): the device is signed out and a
 *     VaultLockedError(loggedOut) is thrown;
 *   - anything else (offline, 429, 5xx): rethrown unchanged; nothing is cleared.
 */
export function refreshTokens(): Promise<RefreshResponse> {
  inflightRefresh ??= doRefresh().finally(() => {
    inflightRefresh = null;
  });
  return inflightRefresh;
}

async function doRefresh(): Promise<RefreshResponse> {
  const account = await loadAccount();
  if (!account) {
    await endLogin();
    throw new VaultLockedError(true);
  }
  const serverUrl = getSession()?.serverUrl || (await getServerUrl());
  let resp: RefreshResponse;
  try {
    resp = await new PassbubbleClient(serverUrl).refresh(account.refreshToken);
  } catch (err) {
    if (httpStatus(err) === 401) {
      await endLogin();
      throw new VaultLockedError(true);
    }
    throw err;
  }
  await saveRefreshToken(resp.refresh_token);
  const live = getSession();
  if (live) {
    live.accessToken = resp.access_token;
    live.accessTokenExpiresAt = Date.now() + resp.expires_in * 1000;
    await setSession(live);
  }
  await scheduleTokenRefresh(resp.expires_in);
  return resp;
}

/** token-refresh alarm: keep the access token of an unlocked vault fresh. A
 * transient failure (e.g. no network right after waking from sleep) keeps the
 * session and retries in a minute — it never locks the vault. */
export async function handleTokenRefreshAlarm(): Promise<void> {
  if (!getSession()) return;
  try {
    await refreshTokens();
  } catch (err) {
    if (err instanceof VaultLockedError) return; // signed out; already cleaned up
    await browser.alarms.create(TOKEN_REFRESH_ALARM, { delayInMinutes: 1 });
  }
}

// Methods that must not trigger a refresh/retry (auth endpoints, local helpers).
const NO_AUTH_RETRY = new Set<PropertyKey>([
  'setTokens',
  'clearTokens',
  'isTokenExpiringSoon',
  'login',
  'verifyTotp',
  'refresh',
  'logout',
  'healthCheck',
]);

/**
 * An API client for the unlocked vault. Each call uses the live session's
 * current access token, refreshes it first when it is about to expire, and on a
 * 401 refreshes once and retries once. The client gets an empty refresh token
 * so shared-ts never rotates tokens on its own (which would bypass persistence).
 */
export function vaultClient(session: UnlockedSession): PassbubbleClient {
  const client = new PassbubbleClient(session.serverUrl);
  const current = () => getSession() ?? session;
  const useCurrentToken = () => client.setTokens(current().accessToken, '', 900);

  return new Proxy(client, {
    get(target, prop, receiver) {
      const value = Reflect.get(target, prop, receiver) as unknown;
      if (typeof value !== 'function' || NO_AUTH_RETRY.has(prop)) return value;
      const method = value as (...args: unknown[]) => Promise<unknown>;
      return async (...args: unknown[]) => {
        if (Date.now() > current().accessTokenExpiresAt - 60_000) {
          try {
            await refreshTokens();
          } catch (err) {
            if (err instanceof VaultLockedError) throw err;
            // transient — try the request with the token we have
          }
        }
        useCurrentToken();
        try {
          return await method.apply(target, args);
        } catch (err) {
          if (httpStatus(err) !== 401) throw err;
          await refreshTokens();
          if (!getSession()) throw new VaultLockedError(false);
          useCurrentToken();
          return await method.apply(target, args);
        }
      };
    },
  });
}

/** The entries metadata cache used for URL matching; reloaded when missing
 * (e.g. after the session was restored into a fresh service worker). */
export async function loadEntriesCache(session: UnlockedSession): Promise<EntryResponse[]> {
  const cached = getEntriesCache();
  if (cached) return cached;
  const entries = await vaultClient(session).listEntries();
  setEntriesCache(entries);
  return entries;
}

const LOGOUT_TIMEOUT_MS = 5000;

/**
 * Best-effort server-side logout (POST /auth/logout needs a valid access token
 * plus the refresh token to revoke). While locked, the refresh token is traded
 * for a short-lived access token first; nothing is persisted, since the caller
 * wipes the login state right afterwards. Bounded by a timeout so signing out
 * never hangs on an unreachable server.
 */
export async function revokeServerSession(): Promise<void> {
  const attempt = (async () => {
    const account = await loadAccount();
    if (!account) return;
    const live = getSession();
    const client = new PassbubbleClient(live?.serverUrl || (await getServerUrl()));
    let accessToken = live?.accessToken ?? '';
    let refreshToken = account.refreshToken;
    if (!live || Date.now() > live.accessTokenExpiresAt - 30_000) {
      const r = await client.refresh(refreshToken);
      accessToken = r.access_token;
      refreshToken = r.refresh_token;
    }
    client.setTokens(accessToken, '', 60);
    await client.logout(refreshToken);
  })();
  let timer: ReturnType<typeof setTimeout> | undefined;
  await Promise.race([
    attempt.catch(() => undefined),
    new Promise<void>((resolve) => {
      timer = setTimeout(resolve, LOGOUT_TIMEOUT_MS);
    }),
  ]);
  clearTimeout(timer);
}

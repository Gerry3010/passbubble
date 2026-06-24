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
import {
  PassbubbleClient,
  unlock,
  decryptEntry,
  encryptEntry,
  createEntry,
  deriveKey,
  aesGcmDecrypt,
  wrapMasterKeyWithPin,
  unlockWithPin,
  generatePinSalt,
  clampPwIntervalDays,
  pwIntervalElapsed,
  PIN_KDF_TIME,
  PIN_KDF_MEMORY,
  DEFAULT_PIN_MAX_TRIES,
  DEFAULT_PIN_PW_INTERVAL_DAYS,
} from '@passbubble/shared-ts';
import { b64Dec, b64Enc, normaliseHost } from '../shared/utils.js';
import {
  MessageType,
  STORAGE_KEYS,
  AUTO_LOCK_DEFAULT_MINUTES,
  AUTO_LOCK_ALARM,
} from '../shared/constants.js';
import {
  clearSession,
  getEntriesCache,
  getLoginFrameHost,
  getSession,
  setEntriesCache,
  setLoginFrameHost,
  setSession,
} from './session-store.js';
import {
  clearPin,
  loadPin,
  savePin,
  savePinFailCount,
  updatePinAfterUnlock,
  type PinBootstrap,
  type PinRecord,
} from './pin-store.js';
import { matchEntriesForUrl } from './autofill-service.js';
import type { EntryData, LoginResponse, SessionInfo, UnlockedSession } from '@passbubble/shared-ts';

// Uniform (already-encrypted / non-secret) session material, sourced from either
// storage.session (normal) or the PIN bootstrap in storage.local (after a
// browser restart). Used to rebuild an UnlockedSession on either unlock path.
interface Bootstrap {
  refreshToken: string;
  encPrivX25519: string;
  encPrivMLKEM: string;
  kdfSalt: string;
  kdfTime: number;
  kdfMemory: number;
  userId: string;
  userEmail: string;
  userName: string;
  role: string;
  pubX25519: string;
  pubMLKEM: string;
}

function makeClient(serverUrl: string, accessToken?: string): PassbubbleClient {
  const c = new PassbubbleClient(serverUrl);
  if (accessToken) c.setTokens(accessToken, '', 900);
  return c;
}

// --- Idle auto-lock -------------------------------------------------------

/** The configured idle-lock timeout in minutes (0 = never). Falls back to the
 * default if unset or malformed. */
export async function readAutoLockMinutes(): Promise<number> {
  try {
    const data = await browser.storage.sync.get(STORAGE_KEYS.AUTO_LOCK_MINUTES);
    const raw = data[STORAGE_KEYS.AUTO_LOCK_MINUTES];
    const n = typeof raw === 'number' ? raw : Number(raw);
    if (Number.isFinite(n) && n >= 0) return n;
  } catch {
    /* fall through to default */
  }
  return AUTO_LOCK_DEFAULT_MINUTES;
}

/** Record "the vault was just used" so the auto-lock alarm restarts its countdown.
 * Best-effort — storage.session may be unavailable in some contexts. */
export async function touchActivity(): Promise<void> {
  try {
    await browser.storage.session.set({ [STORAGE_KEYS.LAST_ACTIVITY]: Date.now() });
  } catch {
    /* ignore */
  }
}

/** (Re)arm the recurring auto-lock alarm. With a 0-minute timeout ("never") the
 * alarm is cleared instead. Called on every unlock; the alarm itself wakes the
 * service worker so locking happens even with the popup closed. */
export async function scheduleAutoLock(): Promise<void> {
  const minutes = await readAutoLockMinutes();
  if (minutes <= 0) {
    await browser.alarms.clear(AUTO_LOCK_ALARM);
    return;
  }
  await touchActivity();
  await browser.alarms.create(AUTO_LOCK_ALARM, { delayInMinutes: 1, periodInMinutes: 1 });
}

/** Auto-lock tick: drop the in-memory session if the vault has been idle for at
 * least the configured timeout, and retire the alarm. Returns true if it locked
 * (or there was nothing to keep armed). Driven by the recurring AUTO_LOCK_ALARM. */
export async function maybeAutoLock(): Promise<boolean> {
  const minutes = await readAutoLockMinutes();
  if (minutes <= 0) {
    await browser.alarms.clear(AUTO_LOCK_ALARM);
    return true;
  }
  // No live session (e.g. the SW was evicted and the keys are already gone):
  // nothing to lock, so retire the alarm until the next unlock re-arms it.
  if (!getSession()) {
    await browser.alarms.clear(AUTO_LOCK_ALARM);
    return true;
  }
  const stored = await browser.storage.session.get(STORAGE_KEYS.LAST_ACTIVITY);
  const last = Number(stored[STORAGE_KEYS.LAST_ACTIVITY]) || 0;
  if (Date.now() - last < minutes * 60_000) return false;
  clearSession();
  await browser.alarms.clear(AUTO_LOCK_ALARM);
  await browser.storage.session.remove(STORAGE_KEYS.LAST_ACTIVITY);
  return true;
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

function bootstrapFromStored(stored: Record<string, unknown>): Bootstrap {
  return {
    refreshToken: stored[STORAGE_KEYS.REFRESH_TOKEN] as string,
    encPrivX25519: stored[STORAGE_KEYS.ENC_PRIV_X25519] as string,
    encPrivMLKEM: stored[STORAGE_KEYS.ENC_PRIV_MLKEM] as string,
    kdfSalt: stored[STORAGE_KEYS.KDF_SALT] as string,
    kdfTime: stored[STORAGE_KEYS.KDF_TIME] as number,
    kdfMemory: stored[STORAGE_KEYS.KDF_MEMORY] as number,
    userId: stored[STORAGE_KEYS.USER_ID] as string,
    userEmail: stored[STORAGE_KEYS.USER_EMAIL] as string,
    userName: stored[STORAGE_KEYS.USER_NAME] as string,
    role: stored[STORAGE_KEYS.ROLE] as string,
    pubX25519: stored[STORAGE_KEYS.PUB_X25519] as string,
    pubMLKEM: stored[STORAGE_KEYS.PUB_MLKEM] as string,
  };
}

function bootstrapFromPin(b: PinBootstrap): Bootstrap {
  return {
    refreshToken: b.refresh_token,
    encPrivX25519: b.enc_priv_x25519,
    encPrivMLKEM: b.enc_priv_mlkem,
    kdfSalt: b.kdf_salt,
    kdfTime: b.kdf_time,
    kdfMemory: b.kdf_memory,
    userId: b.user_id,
    userEmail: b.user_email,
    userName: b.user_name,
    role: b.role,
    pubX25519: b.pub_x25519,
    pubMLKEM: b.pub_mlkem,
  };
}

function pinBootstrapFrom(b: Bootstrap): PinBootstrap {
  return {
    refresh_token: b.refreshToken,
    enc_priv_x25519: b.encPrivX25519,
    enc_priv_mlkem: b.encPrivMLKEM,
    kdf_salt: b.kdfSalt,
    kdf_time: b.kdfTime,
    kdf_memory: b.kdfMemory,
    user_id: b.userId,
    user_email: b.userEmail,
    user_name: b.userName,
    role: b.role,
    pub_x25519: b.pubX25519,
    pub_mlkem: b.pubMLKEM,
  };
}

// Returns the session bootstrap from storage.session, falling back to the PIN
// bootstrap in storage.local (which survives a browser restart). null = not
// logged in on this device.
async function loadBootstrap(): Promise<Bootstrap | null> {
  const stored = await loadSessionData();
  if (stored[STORAGE_KEYS.REFRESH_TOKEN]) return bootstrapFromStored(stored);
  const pin = await loadPin();
  if (pin.enabled && pin.bootstrap) return bootstrapFromPin(pin.bootstrap);
  return null;
}

// Restores the full session bootstrap into storage.session (e.g. after a browser
// restart, where only the PIN-local copy survived) so the other handlers and
// GET_SESSION behave normally afterwards.
async function persistBootstrapToSession(b: Bootstrap, refreshToken: string): Promise<void> {
  await browser.storage.session.set({
    [STORAGE_KEYS.REFRESH_TOKEN]: refreshToken,
    [STORAGE_KEYS.ENC_PRIV_X25519]: b.encPrivX25519,
    [STORAGE_KEYS.ENC_PRIV_MLKEM]: b.encPrivMLKEM,
    [STORAGE_KEYS.KDF_SALT]: b.kdfSalt,
    [STORAGE_KEYS.KDF_TIME]: b.kdfTime,
    [STORAGE_KEYS.KDF_MEMORY]: b.kdfMemory,
    [STORAGE_KEYS.USER_ID]: b.userId,
    [STORAGE_KEYS.USER_EMAIL]: b.userEmail,
    [STORAGE_KEYS.USER_NAME]: b.userName,
    [STORAGE_KEYS.ROLE]: b.role,
    [STORAGE_KEYS.PUB_X25519]: b.pubX25519,
    [STORAGE_KEYS.PUB_MLKEM]: b.pubMLKEM,
  });
}

// Refreshes the access token, builds the in-memory UnlockedSession, pre-loads the
// entries cache, and schedules the refresh alarm. Returns the rotated refresh
// token. Shared by the master-password and PIN unlock paths.
async function buildSession(
  privX25519: Uint8Array,
  privMLKEM: Uint8Array,
  b: Bootstrap,
  serverUrl: string,
): Promise<string> {
  const client = makeClient(serverUrl);
  const refreshResp = await client.refresh(b.refreshToken);

  const session: UnlockedSession = {
    privX25519,
    privMLKEM,
    pubX25519: b64Dec(b.pubX25519),
    pubMLKEM: b64Dec(b.pubMLKEM),
    userId: b.userId,
    userEmail: b.userEmail,
    userName: b.userName,
    role: b.role,
    accessToken: refreshResp.access_token,
    refreshToken: refreshResp.refresh_token,
    accessTokenExpiresAt: Date.now() + refreshResp.expires_in * 1000,
    serverUrl,
    kdfSalt: b64Dec(b.kdfSalt),
    kdfTime: b.kdfTime,
    kdfMemory: b.kdfMemory,
    encPrivX25519: b64Dec(b.encPrivX25519),
    encPrivMLKEM: b64Dec(b.encPrivMLKEM),
  };
  setSession(session);

  const apiClient = makeClient(serverUrl, session.accessToken);
  apiClient.setTokens(session.accessToken, session.refreshToken, refreshResp.expires_in);
  setEntriesCache(await apiClient.listEntries());

  await browser.alarms.create('token-refresh', {
    delayInMinutes: (refreshResp.expires_in - 60) / 60,
  });
  await scheduleAutoLock();
  return refreshResp.refresh_token;
}

/** True if `host` is covered by a blocklist entry (exact host or a parent domain). */
function hostBlocked(host: string, blocklist: string[]): boolean {
  return blocklist.some((e) => {
    const b = e.trim().toLowerCase().replace(/^www\./, '');
    return !!b && (host === b || host.endsWith('.' + b));
  });
}

// Entry encryption needs a valid X25519 public key; the ML-KEM key is optional
// (encryptDataKey falls back to X25519-only for accounts without one). If even
// the X25519 key is missing the session is incomplete — surface a clear message
// instead of a raw crypto exception.
function assertCanEncrypt(session: UnlockedSession): void {
  if (session.pubX25519?.length !== 32) {
    throw new Error('Session keys incomplete — please log out and log in again.');
  }
}

/** Toolbar "?" badge that flags a pending "save this login?" offer. Best-effort. */
async function setSaveBadge(on: boolean): Promise<void> {
  try {
    await browser.action.setBadgeText({ text: on ? '?' : '' });
    if (on) await browser.action.setBadgeBackgroundColor({ color: '#ffb000' });
  } catch {
    /* action API may be unavailable */
  }
}

/** Minimal shape of the message sender we rely on (frame URL + tab id). */
interface MessageSender {
  tab?: { id?: number };
  url?: string;
}

type Handler = (payload: Record<string, unknown>, sender: MessageSender) => Promise<unknown>;

export function buildHandlers(): Record<string, Handler> {
  return {
    [MessageType.GET_SESSION]: async () => {
      const stored = await loadSessionData();
      const session = getSession();
      const serverUrl = await getServerUrl();
      const pin = await loadPin();
      // A configured PIN keeps a logged-in bootstrap in storage.local, so the
      // user stays "logged in" across a browser restart even though
      // storage.session was wiped — they just need to PIN-unlock again.
      const isLoggedIn = !!stored[STORAGE_KEYS.REFRESH_TOKEN] || (pin.enabled && !!pin.bootstrap);
      // PIN is offered for unlock only while it is still within its interval.
      const pinAvailable = pin.enabled && !!pin.bootstrap && !pwIntervalElapsed(pin.lastMasterUnlock, pin.intervalDays);
      return {
        isLoggedIn,
        isUnlocked: !!session,
        userEmail: (stored[STORAGE_KEYS.USER_EMAIL] as string) ?? session?.userEmail ?? pin.bootstrap?.user_email,
        userName: (stored[STORAGE_KEYS.USER_NAME] as string) ?? session?.userName ?? pin.bootstrap?.user_name,
        serverUrl,
        pinEnabled: pin.enabled,
        pinAvailable,
      } satisfies SessionInfo & { serverUrl: string; pinEnabled: boolean; pinAvailable: boolean };
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
      const serverUrl = await getServerUrl();
      // Fall back to the PIN-local bootstrap when storage.session was wiped (e.g.
      // after a browser restart with a configured PIN).
      const bs = await loadBootstrap();
      if (!bs) throw new Error('Not logged in');

      const kdfParams = {
        salt: b64Dec(bs.kdfSalt),
        time: bs.kdfTime,
        memory: bs.kdfMemory,
      };
      let privX25519: Uint8Array;
      let privMLKEM: Uint8Array;
      try {
        ({ privX25519, privMLKEM } = await unlock(masterPassword, kdfParams, bs.encPrivX25519, bs.encPrivMLKEM));
      } catch {
        // A failed key-unwrap throws a raw DOMException; report it cleanly.
        throw new Error('Wrong master password');
      }

      const newRefresh = await buildSession(privX25519, privMLKEM, bs, serverUrl);
      // Make sure storage.session is fully populated again (it may have been
      // empty when we unlocked from the PIN-local bootstrap).
      await persistBootstrapToSession(bs, newRefresh);

      // A successful master-password unlock restarts the PIN re-auth interval.
      const pin = await loadPin();
      if (pin.enabled) await updatePinAfterUnlock(pin, newRefresh, true);

      return { ok: true };
    },

    [MessageType.SET_PIN]: async (payload) => {
      const { masterPassword, pin, intervalDays } = payload as {
        masterPassword: string;
        pin: string;
        intervalDays?: number;
      };
      const bs = await loadBootstrap();
      if (!bs) throw new Error('Not logged in');

      // Re-derive and verify the master password before trusting the key.
      const masterKey = await deriveKey(masterPassword, {
        salt: b64Dec(bs.kdfSalt),
        time: bs.kdfTime,
        memory: bs.kdfMemory,
      });
      try {
        await aesGcmDecrypt(masterKey, b64Dec(bs.encPrivX25519));
      } catch {
        throw new Error('Wrong master password');
      }

      const pinSalt = generatePinSalt();
      const wrapped = await wrapMasterKeyWithPin(masterKey, pin, pinSalt);
      const rec: PinRecord = {
        enabled: true,
        salt: b64Enc(pinSalt),
        wrapped: b64Enc(wrapped),
        kdfTime: PIN_KDF_TIME,
        kdfMemory: PIN_KDF_MEMORY,
        maxTries: DEFAULT_PIN_MAX_TRIES,
        failCount: 0,
        intervalDays: clampPwIntervalDays(intervalDays ?? DEFAULT_PIN_PW_INTERVAL_DAYS),
        lastMasterUnlock: Date.now(),
        bootstrap: pinBootstrapFrom(bs),
      };
      await savePin(rec);
      return { ok: true };
    },

    [MessageType.UNLOCK_WITH_PIN]: async (payload) => {
      const { pin } = payload as { pin: string };
      const rec = await loadPin();
      if (!rec.enabled || !rec.bootstrap) return { ok: false, error: 'PIN not enabled' };

      if (pwIntervalElapsed(rec.lastMasterUnlock, rec.intervalDays)) {
        return { ok: false, expired: true };
      }

      const maxTries = rec.maxTries || DEFAULT_PIN_MAX_TRIES;
      // Persist the incremented counter BEFORE the attempt so killing the worker
      // mid-attempt cannot reset it and bypass the lockout.
      const newCount = rec.failCount + 1;
      await savePinFailCount(newCount);

      const serverUrl = await getServerUrl();
      // Only a failed crypto unwrap (wrong PIN → GCM auth-tag failure) counts as a
      // failed attempt. A later network / token-refresh error must NOT consume a
      // try or trigger the lockout.
      let privs: { privX25519: Uint8Array; privMLKEM: Uint8Array };
      try {
        privs = await unlockWithPin(
          pin,
          b64Dec(rec.salt),
          b64Dec(rec.wrapped),
          rec.bootstrap.enc_priv_x25519,
          rec.bootstrap.enc_priv_mlkem,
        );
      } catch {
        if (newCount >= maxTries) {
          await clearPin();
          return { ok: false, lockedOut: true };
        }
        return { ok: false, wrongPin: true, triesRemaining: maxTries - newCount };
      }

      // The PIN was correct. Building the session may still fail (offline, or a
      // stale/rotated refresh token); roll back the pre-incremented counter so a
      // server-side hiccup never burns attempts or wipes a valid PIN.
      const bs = bootstrapFromPin(rec.bootstrap);
      try {
        const newRefresh = await buildSession(privs.privX25519, privs.privMLKEM, bs, serverUrl);
        await persistBootstrapToSession(bs, newRefresh);
        // Reset the failure counter + rotate the stored refresh token. The interval
        // is NOT reset — only a real master-password unlock restarts it.
        await updatePinAfterUnlock({ ...rec, failCount: 0 }, newRefresh, false);
      } catch {
        await savePinFailCount(rec.failCount);
        return {
          ok: false,
          error: 'Could not refresh your session — please unlock with your master password.',
        };
      }
      return { ok: true };
    },

    [MessageType.DISABLE_PIN]: async () => {
      await clearPin();
      return { ok: true };
    },

    [MessageType.GET_PIN_STATUS]: async () => {
      const rec = await loadPin();
      if (!rec.enabled) return { enabled: false };
      const maxTries = rec.maxTries || DEFAULT_PIN_MAX_TRIES;
      return {
        enabled: true,
        expired: pwIntervalElapsed(rec.lastMasterUnlock, rec.intervalDays),
        intervalDays: rec.intervalDays,
        triesRemaining: maxTries - rec.failCount,
      };
    },

    [MessageType.LOCK]: async () => {
      clearSession();
      await browser.alarms.clear('token-refresh');
      await browser.alarms.clear(AUTO_LOCK_ALARM);
      return { ok: true };
    },

    // Full sign-out: unlike LOCK (which only drops the in-memory keys but keeps
    // you logged in), this wipes the refresh token and all session storage so
    // the next popup starts at the login screen.
    [MessageType.LOGOUT]: async () => {
      clearSession();
      await browser.alarms.clear('token-refresh');
      await browser.alarms.clear(AUTO_LOCK_ALARM);
      await browser.storage.session.clear();
      // The PIN wraps the master key and holds a logged-in bootstrap — a
      // logged-out device must never leave a PIN-unlockable copy behind.
      await clearPin();
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

    // The content script (running in the frame that has a login form) reports
    // its host so the popup can pre-fill search + the "+ Site" toggle with the
    // *form's* host, which for SSO logins is an iframe, not the top page.
    [MessageType.REPORT_LOGIN_FRAME]: async (payload, sender) => {
      const tabId = sender?.tab?.id;
      const { host } = payload as { host?: string };
      if (typeof tabId === 'number' && host) setLoginFrameHost(tabId, host);
      return { ok: true };
    },

    [MessageType.GET_FILL_HOST]: async () => {
      const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
      const tabId = tab?.id;
      return { host: typeof tabId === 'number' ? getLoginFrameHost(tabId) : '' };
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
      assertCanEncrypt(session);
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

    // Partial metadata update. Currently used by the "+ Site" toggle to add/
    // remove an autofill match pattern. folder_id must be echoed back because the
    // backend always overwrites it; the caller passes the entry's current folder.
    [MessageType.UPDATE_ENTRY]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { entryId, matchPatterns, folderId } = payload as {
        entryId: string;
        matchPatterns?: string[];
        folderId?: string;
      };
      const client = makeClient(session.serverUrl, session.accessToken);
      await client.updateEntry(entryId, { folder_id: folderId, match_patterns: matchPatterns });
      // Refresh the cache so autofill matching sees the new patterns immediately.
      setEntriesCache(await client.listEntries());
      return { ok: true };
    },

    [MessageType.GENERATE]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const client = makeClient(session.serverUrl, session.accessToken);
      return client.generate(payload as Parameters<typeof client.generate>[0]);
    },

    // A form submission (or generated-password fill) was detected. Stash the
    // credentials so the in-page bar can offer to save them — and, on a site we
    // already know, detect whether this is a *new* account or a credential
    // *update*. Consent-based: we never create/update an entry silently.
    [MessageType.OFFER_SAVE]: async (payload) => {
      const { name, url, username, password } = payload as {
        name: string;
        url: string;
        username: string;
        password: string;
      };
      const session = getSession();
      if (!session) return { ok: false }; // locked — can't save anyway
      const host = normaliseHost(url);
      // Never offer for blocklisted hosts/domains (autofill is unaffected).
      const block = await browser.storage.local.get(STORAGE_KEYS.SAVE_BLOCKLIST);
      const blocklist = (block[STORAGE_KEYS.SAVE_BLOCKLIST] as string[] | undefined) ?? [];
      if (host && hostBlocked(host, blocklist)) return { ok: false, blocked: true };
      // Skip hosts the user previously dismissed (the content script can't read
      // storage.session, so this check lives here).
      const dism = await browser.storage.session.get(STORAGE_KEYS.DISMISSED_SAVE_HOSTS);
      const dismissed = (dism[STORAGE_KEYS.DISMISSED_SAVE_HOSTS] as string[] | undefined) ?? [];
      if (host && dismissed.includes(host)) return { ok: false, dismissed: true };

      // Change detection against entries that already match this URL. listEntries
      // is metadata-only, so decrypt each candidate's full entry to read its
      // username/password. If the exact credentials are already stored there's
      // nothing to offer; otherwise the user can create a new entry or update an
      // existing one (same username + different password = the likely target).
      const client = makeClient(session.serverUrl, session.accessToken);
      const cache = getEntriesCache();
      const matches = cache ? matchEntriesForUrl(url, cache) : [];
      const candidates: { id: string; username: string }[] = [];
      let suggestUpdateId: string | undefined;
      for (const m of matches) {
        let creds: EntryData;
        try {
          creds = await decryptEntry(await client.getEntry(m.id), session);
        } catch {
          continue; // skip entries we can't read
        }
        const u = creds.username ?? '';
        const p = creds.password ?? '';
        if (u === username && p === password) return { ok: false, unchanged: true };
        candidates.push({ id: m.id, username: u });
        if (suggestUpdateId === undefined && u === username) suggestUpdateId = m.id;
      }

      // candidates + suggestUpdateId are persisted too so the in-page bar can be
      // fully restored (incl. the "update existing" option) after a navigation —
      // the content script re-renders it from GET_PENDING_SAVE on the next load.
      await browser.storage.session.set({
        [STORAGE_KEYS.PENDING_SAVE]: { name, url, username, password, candidates, suggestUpdateId },
      });
      await setSaveBadge(true);
      return { ok: true, candidates, suggestUpdateId };
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
      assertCanEncrypt(session);
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
      await setSaveBadge(false);
      return { ok: true, ...result };
    },

    // The user chose to update an existing entry (known site, changed password or
    // a new password for the same username). Re-encrypt the pending credentials
    // and overwrite that entry's data; the data key is freshly wrapped for the
    // current session keys (hybrid where available, X25519-only fallback).
    [MessageType.UPDATE_SAVE]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      assertCanEncrypt(session);
      const { entryId } = payload as { entryId?: string };
      if (!entryId) return { ok: false };
      const stored = await browser.storage.session.get(STORAGE_KEYS.PENDING_SAVE);
      const pending = stored[STORAGE_KEYS.PENDING_SAVE] as
        | { name: string; url: string; username: string; password: string }
        | undefined;
      if (!pending) return { ok: false };

      const client = makeClient(session.serverUrl, session.accessToken);
      const data = { username: pending.username, password: pending.password } as EntryData;
      const encrypted = await encryptEntry(data, session);
      await client.updateEntry(entryId, { ...encrypted });
      setEntriesCache(await client.listEntries());
      await browser.storage.session.remove(STORAGE_KEYS.PENDING_SAVE);
      await setSaveBadge(false);
      return { ok: true, id: entryId };
    },

    [MessageType.DISMISS_SAVE]: async (payload) => {
      const { host } = payload as { host?: string };
      await browser.storage.session.remove(STORAGE_KEYS.PENDING_SAVE);
      await setSaveBadge(false);
      if (host) {
        const stored = await browser.storage.session.get(STORAGE_KEYS.DISMISSED_SAVE_HOSTS);
        const list = (stored[STORAGE_KEYS.DISMISSED_SAVE_HOSTS] as string[] | undefined) ?? [];
        if (!list.includes(host)) {
          await browser.storage.session.set({ [STORAGE_KEYS.DISMISSED_SAVE_HOSTS]: [...list, host] });
        }
      }
      return { ok: true };
    },

    // ─── Save blocklist (device-local; never prompt "save?" for these) ─────────
    [MessageType.BLOCKLIST_GET]: async () => {
      const s = await browser.storage.local.get(STORAGE_KEYS.SAVE_BLOCKLIST);
      return { list: (s[STORAGE_KEYS.SAVE_BLOCKLIST] as string[] | undefined) ?? [] };
    },

    [MessageType.BLOCKLIST_ADD]: async (payload) => {
      const host = String((payload as { host?: string }).host ?? '')
        .trim()
        .toLowerCase()
        .replace(/^www\./, '');
      if (!host) return { ok: false };
      const s = await browser.storage.local.get(STORAGE_KEYS.SAVE_BLOCKLIST);
      const list = (s[STORAGE_KEYS.SAVE_BLOCKLIST] as string[] | undefined) ?? [];
      if (!list.includes(host)) {
        await browser.storage.local.set({ [STORAGE_KEYS.SAVE_BLOCKLIST]: [...list, host] });
      }
      // Also clear any pending offer for this host.
      await browser.storage.session.remove(STORAGE_KEYS.PENDING_SAVE);
      await setSaveBadge(false);
      return { ok: true };
    },

    [MessageType.BLOCKLIST_REMOVE]: async (payload) => {
      const { host } = payload as { host?: string };
      const s = await browser.storage.local.get(STORAGE_KEYS.SAVE_BLOCKLIST);
      const list = (s[STORAGE_KEYS.SAVE_BLOCKLIST] as string[] | undefined) ?? [];
      await browser.storage.local.set({
        [STORAGE_KEYS.SAVE_BLOCKLIST]: list.filter((h) => h !== host),
      });
      return { ok: true };
    },
  };
}

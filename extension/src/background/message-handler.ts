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
  computeHealthReport,
  generateTotp,
  deriveKey,
  aesGcmDecrypt,
  wrapMasterKeyWithPin,
  unlockWithPin,
  generatePinSalt,
  clampPwIntervalDays,
  pwIntervalElapsed,
  putTags,
  tagsOf,
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
  adoptSession,
  clearEntryTagsCache,
  getEntryTagsCache,
  getLastFilledEntry,
  getLoginFrameHost,
  getSession,
  readStoredSession,
  setEntriesCache,
  setEntryTagsCache,
  setLastFilledEntry,
  setLoginFrameHost,
  setSession,
} from './session-store.js';
import { clearPin, loadPin, resetPinAfterUnlock, savePin, savePinFailCount, type PinRecord } from './pin-store.js';
import { loadAccount, saveAccountFromLogin, type StoredAccount } from './account-store.js';
import {
  VaultLockedError,
  endLogin,
  getServerUrl,
  httpStatus,
  loadEntriesCache,
  lockVault,
  refreshTokens,
  revokeServerSession,
  vaultClient,
} from './vault-client.js';
import { matchEntriesForUrl } from './autofill-service.js';
import { deleteSsoRecord, getSsoRecord, noteSsoCandidate } from './sso-memory.js';
import { persistSsoToEntries, ssoFromEntries } from './sso-entry.js';
import type { SsoProvider } from '../shared/sso.js';
import type { EntryData, EntryResponse, SessionInfo, UnlockedSession } from '@passbubble/shared-ts';

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
  // Always record the unlock as activity, so switching from "never" to a timeout
  // later measures idle time from a real timestamp.
  await touchActivity();
  const minutes = await readAutoLockMinutes();
  if (minutes <= 0) {
    await browser.alarms.clear(AUTO_LOCK_ALARM);
    return;
  }
  await browser.alarms.create(AUTO_LOCK_ALARM, { delayInMinutes: 1, periodInMinutes: 1 });
}

/** True once the vault has been idle for at least `minutes` (0 = never). */
async function idleLimitReached(minutes: number): Promise<boolean> {
  if (minutes <= 0) return false;
  const stored = await browser.storage.session.get(STORAGE_KEYS.LAST_ACTIVITY);
  const last = Number(stored[STORAGE_KEYS.LAST_ACTIVITY]) || 0;
  return Date.now() - last >= minutes * 60_000;
}

/** Auto-lock tick: lock the vault if it has been idle for at least the
 * configured timeout, and retire the alarm. Returns true if it locked (or there
 * was nothing to keep armed). Driven by the recurring AUTO_LOCK_ALARM. */
export async function maybeAutoLock(): Promise<boolean> {
  const minutes = await readAutoLockMinutes();
  if (minutes <= 0) {
    await browser.alarms.clear(AUTO_LOCK_ALARM);
    return true;
  }
  // A restarted service worker starts with empty memory while the unlocked
  // session still sits in storage.session — that session must lock as well, so
  // only retire the alarm when neither exists.
  if (!getSession() && !(await readStoredSession())) {
    await browser.alarms.clear(AUTO_LOCK_ALARM);
    return true;
  }
  if (!(await idleLimitReached(minutes))) return false;
  await lockVault();
  return true;
}

let restoring: Promise<void> | null = null;

/**
 * Bring the unlocked session back after the service worker was restarted (its
 * memory is gone, the copy in storage.session is not). The auto-lock setting
 * stays authoritative: a session idle for longer than the timeout is locked
 * instead of restored. Must run before handling any message or alarm.
 */
export function ensureSessionRestored(): Promise<void> {
  if (getSession()) return Promise.resolve();
  restoring ??= (async () => {
    try {
      const stored = await readStoredSession();
      if (!stored) return;
      if (await idleLimitReached(await readAutoLockMinutes())) {
        await lockVault();
        return;
      }
      if (!getSession()) adoptSession(stored);
    } catch {
      /* stay locked */
    }
  })().finally(() => {
    restoring = null;
  });
  return restoring;
}

/** Resolve credentials to satisfy an HTTP Basic Auth challenge for `url`, reusing
 * the autofill URL→entry matching and decrypt-on-demand. Returns null when the
 * vault is locked, nothing matches, or the match is ambiguous (more than one
 * entry) — in which case the browser shows its native dialog instead of risking
 * the wrong credentials. Only usernames/passwords for a single unambiguous match
 * are returned; passwords are never cached. */
export async function resolveBasicAuthCredentials(
  url: string,
): Promise<{ username: string; password: string } | null> {
  await ensureSessionRestored();
  const session = getSession();
  if (!session) return null;
  try {
    const matches = matchEntriesForUrl(url, await loadEntriesCache(session));
    // Exactly one match → unambiguous. Zero or several → let the user decide.
    if (matches.length !== 1) return null;
    const data = await decryptEntry(await vaultClient(session).getEntry(matches[0].id), session);
    const password = data.password ?? '';
    if (!password) return null;
    return { username: data.username ?? '', password };
  } catch {
    return null;
  }
}

/** The entries cache for URL matching, loaded on demand; [] if it cannot be
 * loaded right now (a lock / sign-out still propagates). */
async function entriesForMatching(session: UnlockedSession): Promise<EntryResponse[]> {
  try {
    return await loadEntriesCache(session);
  } catch (err) {
    if (err instanceof VaultLockedError) throw err;
    return [];
  }
}

/** Fingerprint of an entry listing for the tags cache. It changes whenever an
 * entry is added, removed or has its data rewritten (the server bumps
 * updated_at), here or on another device. */
function listingStamp(entries: EntryResponse[]): string {
  return entries
    .map((e) => `${e.id}@${e.updated_at}`)
    .sort()
    .join('|');
}

// Gets fresh tokens (persisting the rotated refresh token first), builds the
// unlocked session (memory + storage.session) and arms the auto-lock. Shared by
// the master-password and PIN unlock paths.
async function buildSession(
  privX25519: Uint8Array,
  privMLKEM: Uint8Array,
  account: StoredAccount,
  serverUrl: string,
): Promise<void> {
  const refreshResp = await refreshTokens();

  const session: UnlockedSession = {
    privX25519,
    privMLKEM,
    pubX25519: b64Dec(account.pubX25519),
    pubMLKEM: b64Dec(account.pubMLKEM),
    userId: account.userId,
    userEmail: account.userEmail,
    userName: account.userName,
    role: account.role,
    accessToken: refreshResp.access_token,
    // The refresh token lives in storage.local only (account-store.ts).
    refreshToken: '',
    accessTokenExpiresAt: Date.now() + refreshResp.expires_in * 1000,
    serverUrl,
    kdfSalt: b64Dec(account.kdfSalt),
    kdfTime: account.kdfTime,
    kdfMemory: account.kdfMemory,
    encPrivX25519: b64Dec(account.encPrivX25519),
    encPrivMLKEM: b64Dec(account.encPrivMLKEM),
  };
  await setSession(session);
  await scheduleAutoLock();

  // Pre-load the entries cache. Not fatal: the popup and autofill load it on demand.
  try {
    setEntriesCache(await vaultClient(session).listEntries());
  } catch (err) {
    if (err instanceof VaultLockedError) throw err;
  }
}

// A failed AES-GCM key unwrap (wrong master password) surfaces as a WebCrypto
// OperationError; anything else is a different problem and must say so.
function isDecryptionFailure(err: unknown): boolean {
  return (err as { name?: unknown } | null)?.name === 'OperationError';
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  return typeof err === 'string' ? err : 'unknown error';
}

/** User-facing message for a failed server round-trip during unlock. */
function describeRequestError(err: unknown): string {
  const status = httpStatus(err);
  if (status === 429) return 'Too many requests — please wait a minute and try again.';
  if (status !== undefined && status >= 500) return `The server reported an error (${status}) — please try again later.`;
  if (status !== undefined) return `The server rejected the request (${status}: ${errorMessage(err)}).`;
  if (err instanceof TypeError) return 'Could not reach the server — check your connection and try again.';
  return `Unlock failed: ${errorMessage(err)}`;
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

// A handler that finds the vault locked — or the device signed out — mid-request
// (e.g. the refresh token was rejected) answers like a locked vault, so the popup
// and content scripts switch to their unlock / sign-in state.
function withLockHandling(handlers: Record<string, Handler>): Record<string, Handler> {
  const wrapped: Record<string, Handler> = {};
  for (const [type, handler] of Object.entries(handlers)) {
    wrapped[type] = async (payload, sender) => {
      try {
        return await handler(payload, sender);
      } catch (err) {
        if (err instanceof VaultLockedError) return { locked: true, loggedOut: err.loggedOut };
        throw err;
      }
    };
  }
  return wrapped;
}

const SESSION_EXPIRED_MESSAGE = 'Your session has expired — please log in again.';

export function buildHandlers(): Record<string, Handler> {
  return withLockHandling({
    [MessageType.GET_SESSION]: async () => {
      const account = await loadAccount();
      const session = getSession();
      const serverUrl = await getServerUrl();
      const pin = await loadPin();
      // The login state lives in storage.local, so the device stays logged in
      // across browser restarts — it just needs to be unlocked again.
      const isLoggedIn = !!account;
      // PIN is offered for unlock only while it is still within its interval.
      const pinAvailable = isLoggedIn && pin.enabled && !pwIntervalElapsed(pin.lastMasterUnlock, pin.intervalDays);
      return {
        isLoggedIn,
        isUnlocked: !!session,
        userEmail: account?.userEmail || session?.userEmail,
        userName: account?.userName || session?.userName,
        serverUrl,
        pinEnabled: pin.enabled,
        pinAvailable,
      } satisfies SessionInfo & { serverUrl: string; pinEnabled: boolean; pinAvailable: boolean };
    },

    [MessageType.LOGIN]: async (payload) => {
      const { email, password } = payload as { email: string; password: string };
      const serverUrl = await getServerUrl();
      if (!serverUrl) throw new Error('Server URL not configured');
      const client = new PassbubbleClient(serverUrl);
      const resp = await client.login(email, password);
      // Account-level 2FA: the password step succeeded but a TOTP code is
      // required. Do NOT persist anything yet (the response carries no keys).
      if (resp.status === '2fa_required') {
        return { ok: true, requiresTotp: true, pendingToken: resp.pending_token };
      }
      await saveAccountFromLogin(resp);
      return { ok: true, needsUnlock: true };
    },

    [MessageType.VERIFY_TOTP]: async (payload) => {
      const { pendingToken, code } = payload as { pendingToken: string; code: string };
      const serverUrl = await getServerUrl();
      if (!serverUrl) throw new Error('Server URL not configured');
      const client = new PassbubbleClient(serverUrl);
      const resp = await client.verifyTotp(pendingToken, code);
      await saveAccountFromLogin(resp);
      return { ok: true, needsUnlock: true };
    },

    [MessageType.UNLOCK]: async (payload) => {
      const { masterPassword } = payload as { masterPassword: string };
      const serverUrl = await getServerUrl();
      const account = await loadAccount();
      if (!account) return { ok: false, loggedOut: true, error: 'Not logged in — please log in again.' };

      let kdfSalt: Uint8Array;
      try {
        kdfSalt = b64Dec(account.kdfSalt);
      } catch {
        return { ok: false, error: 'Stored login data is damaged — please log out and log in again.' };
      }
      let privX25519: Uint8Array;
      let privMLKEM: Uint8Array;
      try {
        ({ privX25519, privMLKEM } = await unlock(
          masterPassword,
          { salt: kdfSalt, time: account.kdfTime, memory: account.kdfMemory },
          account.encPrivX25519,
          account.encPrivMLKEM,
        ));
      } catch (err) {
        if (isDecryptionFailure(err)) return { ok: false, wrongPassword: true, error: 'Wrong master password' };
        return { ok: false, error: `Unlock failed: ${errorMessage(err)}` };
      }

      try {
        await buildSession(privX25519, privMLKEM, account, serverUrl);
      } catch (err) {
        if (err instanceof VaultLockedError) return { ok: false, loggedOut: true, error: SESSION_EXPIRED_MESSAGE };
        return { ok: false, error: describeRequestError(err) };
      }

      // A successful master-password unlock restarts the PIN re-auth interval.
      await resetPinAfterUnlock(await loadPin(), true);
      return { ok: true };
    },

    [MessageType.SET_PIN]: async (payload) => {
      const { masterPassword, pin, intervalDays } = payload as {
        masterPassword: string;
        pin: string;
        intervalDays?: number;
      };
      const account = await loadAccount();
      if (!account) throw new Error('Not logged in');

      // Re-derive and verify the master password before trusting the key.
      const masterKey = await deriveKey(masterPassword, {
        salt: b64Dec(account.kdfSalt),
        time: account.kdfTime,
        memory: account.kdfMemory,
      });
      try {
        await aesGcmDecrypt(masterKey, b64Dec(account.encPrivX25519));
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
      };
      await savePin(rec);
      return { ok: true };
    },

    [MessageType.UNLOCK_WITH_PIN]: async (payload) => {
      const { pin } = payload as { pin: string };
      const rec = await loadPin();
      const account = await loadAccount();
      if (!rec.enabled || !account) return { ok: false, error: 'PIN not enabled' };

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
          account.encPrivX25519,
          account.encPrivMLKEM,
        );
      } catch {
        if (newCount >= maxTries) {
          await clearPin();
          await lockVault();
          return { ok: false, lockedOut: true };
        }
        return { ok: false, wrongPin: true, triesRemaining: maxTries - newCount };
      }

      // The PIN was correct. Building the session may still fail (offline, server
      // error); roll back the pre-incremented counter so a server-side hiccup
      // never burns attempts or wipes a valid PIN.
      try {
        await buildSession(privs.privX25519, privs.privMLKEM, account, serverUrl);
      } catch (err) {
        // A rejected refresh token signed the device out (PIN included).
        if (err instanceof VaultLockedError) return { ok: false, loggedOut: true, error: SESSION_EXPIRED_MESSAGE };
        await savePinFailCount(rec.failCount);
        return { ok: false, error: describeRequestError(err) };
      }
      // Reset the failure counter. The interval is NOT reset — only a real
      // master-password unlock restarts it.
      await resetPinAfterUnlock(rec, false);
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
      await lockVault();
      return { ok: true };
    },

    // Full sign-out: unlike LOCK (which only drops the unlocked session but keeps
    // you logged in), this revokes the server session (best-effort) and wipes the
    // refresh token and all session storage so the next popup starts at the
    // login screen.
    [MessageType.LOGOUT]: async () => {
      await revokeServerSession();
      // The PIN wraps the master key — a logged-out device must never leave a
      // PIN-unlockable copy behind (endLogin clears it with the login state).
      await endLogin();
      await browser.storage.session.clear();
      return { ok: true };
    },

    [MessageType.SEARCH_ENTRIES]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { query } = payload as { query: string };
      const client = vaultClient(session);
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
      const client = vaultClient(session);
      return client.listFolders();
    },

    [MessageType.GET_ENTRY]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { id } = payload as { id: string };
      const client = vaultClient(session);
      const apiEntry = await client.getEntry(id);
      const data = await decryptEntry(apiEntry, session);
      return { entry: apiEntry, data };
    },

    // Decrypt just the username of every entry so the popup can search by
    // username. Uses the bulk "full" listing (one request with encrypted_data +
    // entry keys) rather than N getEntry calls. Passwords are never bulk-decrypted
    // or cached — only usernames, which are the searchable field.
    [MessageType.GET_USERNAMES]: async () => {
      const session = getSession();
      if (!session) return { locked: true };
      const client = vaultClient(session);
      const full = await client.listEntriesFull();
      const usernames: Record<string, string> = {};
      for (const entry of full) {
        try {
          const data = await decryptEntry(entry, session);
          if (data.username) usernames[entry.id] = data.username;
        } catch {
          // skip entries we cannot read (e.g. shared with an incompatible key)
        }
      }
      return { usernames };
    },

    // Decrypt the tags of every entry for the popup's tag filter, in bulk like
    // GET_USERNAMES. Cached until the entry listing changes (the popup refreshes
    // it via SEARCH_ENTRIES right before asking), an entry is written here, or
    // the vault locks. Entries without tags are left out of the map.
    [MessageType.LIST_ENTRY_TAGS]: async () => {
      const session = getSession();
      if (!session) return { locked: true };
      const stamp = listingStamp(await loadEntriesCache(session));
      const cached = getEntryTagsCache();
      if (cached?.stamp === stamp) return { tags: cached.tags };
      const full = await vaultClient(session).listEntriesFull();
      const tags: Record<string, string[]> = {};
      for (const entry of full) {
        try {
          const entryTags = tagsOf(await decryptEntry(entry, session));
          if (entryTags.length) tags[entry.id] = entryTags;
        } catch {
          // skip entries we cannot read
        }
      }
      // A lock while decrypting already cleared the cache — don't refill it.
      if (getSession()) setEntryTagsCache({ stamp, tags });
      return { tags };
    },

    [MessageType.FILL_ENTRY]: async (payload, sender) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { entryId } = payload as { entryId: string };
      const client = vaultClient(session);
      const apiEntry = await client.getEntry(entryId);
      const data = await decryptEntry(apiEntry, session);
      // Remember the fill per tab so the 2FA step that usually follows can find
      // this entry's TOTP code even when the 2FA page's URL no longer matches.
      const tabId = sender?.tab?.id;
      if (typeof tabId === 'number') setLastFilledEntry(tabId, entryId);
      // When the entry has a TOTP secret, hand the current code along — the fill
      // UI copies it to the clipboard so it's ready for the 2FA prompt. Only the
      // short-lived code leaves the background, never the secret.
      let totpCode = '';
      if (data.totp_secret) {
        try {
          totpCode = (await generateTotp(data.totp_secret, { period: data.period, digits: data.digits })).code;
        } catch {
          // malformed secret — fill username/password anyway
        }
      }
      return { username: data.username ?? '', password: data.password ?? '', totpCode };
    },

    // The current TOTP code for the entry relevant to `url`: preferably the
    // entry just filled in this tab (the 2FA page often matches no URL pattern),
    // otherwise the first URL match that has a TOTP secret.
    [MessageType.GET_TOTP_FOR_URL]: async (payload, sender) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { url } = payload as { url: string };
      const client = vaultClient(session);
      const tabId = sender?.tab?.id;
      const lastFilled = typeof tabId === 'number' ? getLastFilledEntry(tabId) : '';
      const matchIds = matchEntriesForUrl(url, await entriesForMatching(session)).map((m) => m.id);
      const tryIds = [...new Set([...(lastFilled ? [lastFilled] : []), ...matchIds])];
      for (const id of tryIds) {
        try {
          const apiEntry = await client.getEntry(id);
          const data = await decryptEntry(apiEntry, session);
          if (!data.totp_secret) continue;
          const r = await generateTotp(data.totp_secret, { period: data.period, digits: data.digits });
          return { code: r.code, remainingSeconds: r.secondsRemaining, entryName: apiEntry.name };
        } catch {
          // skip entries we cannot read or with malformed secrets
        }
      }
      return { code: '' };
    },

    // Entries of a given type (credit-card / identity) with a short non-secret
    // display hint for the picker: cards show their last 4 digits, identities
    // the email or name. Full data only via FILL_TYPED_ENTRY after selection.
    [MessageType.GET_ENTRIES_BY_TYPE]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { type } = payload as { type: string };
      const client = vaultClient(session);
      const cache = await loadEntriesCache(session);
      const items: { id: string; name: string; hint: string }[] = [];
      for (const meta of cache.filter((e) => e.type === type)) {
        let hint = '';
        try {
          const data = await decryptEntry(await client.getEntry(meta.id), session);
          if (type === 'credit-card') {
            const digits = (data.card_number ?? '').replace(/\D/g, '');
            hint = digits ? `•••• ${digits.slice(-4)}` : (data.holder_name ?? '');
          } else {
            hint = data.email || [data.first_name, data.last_name].filter(Boolean).join(' ');
          }
        } catch {
          // undecryptable entry — still list it by name
        }
        items.push({ id: meta.id, name: meta.name, hint });
      }
      return { items };
    },

    // Decrypted field map of a card/identity entry for the content script to
    // fill into classified form fields. Same trust path as FILL_ENTRY.
    [MessageType.FILL_TYPED_ENTRY]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { entryId } = payload as { entryId: string };
      const client = vaultClient(session);
      const apiEntry = await client.getEntry(entryId);
      const data = await decryptEntry(apiEntry, session);
      return {
        type: apiEntry.type,
        data: {
          card_number: data.card_number ?? '',
          holder_name: data.holder_name ?? '',
          expiry_month: data.expiry_month ?? '',
          expiry_year: data.expiry_year ?? '',
          cvv: data.cvv ?? '',
          title: data.title ?? '',
          first_name: data.first_name ?? '',
          last_name: data.last_name ?? '',
          company: data.company ?? '',
          email: data.email ?? '',
          phone: data.phone ?? '',
          street: data.street ?? '',
          city: data.city ?? '',
          state: data.state ?? '',
          postal_code: data.postal_code ?? '',
          country: data.country ?? '',
        },
      };
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
      // The cache is empty after a service-worker restart — reload it on demand.
      return matchEntriesForUrl(url, await entriesForMatching(session));
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
      // Tags are normalised on save whoever sent them (popup form, import).
      if (data && 'tags' in data) putTags(data, data.tags);
      const client = vaultClient(session);
      const result = await createEntry(client, name, type, url, data, session, folderId, matchPatterns);
      // Invalidate cache
      clearEntryTagsCache();
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
      const client = vaultClient(session);
      await client.updateEntry(entryId, { folder_id: folderId, match_patterns: matchPatterns });
      // Refresh the cache so autofill matching sees the new patterns immediately.
      setEntriesCache(await client.listEntries());
      return { ok: true };
    },

    // "Sign in with …" memory. Candidates come from content-script clicks and
    // are confirmed by the OAuth navigation (sso-memory.ts); GET feeds the
    // in-page badge; DELETE is the badge's "forget" affordance. The canonical,
    // cross-device store is the entry's encrypted sign_in_with field
    // (sso-entry.ts) — the device-local record is the fallback, and is synced
    // into matching entries opportunistically when the vault is unlocked.
    [MessageType.SSO_CANDIDATE]: async (payload) => {
      const { host, provider } = payload as { host: string; provider: SsoProvider };
      noteSsoCandidate(host, provider);
      return { ok: true };
    },

    [MessageType.SSO_GET]: async (payload) => {
      const { host, url } = payload as { host: string; url?: string };
      const fromEntry = await ssoFromEntries(url || `https://${host}/`);
      if (fromEntry) return { record: { provider: fromEntry.provider, source: 'entry' } };
      const record = await getSsoRecord(host);
      // A local-only record with an unlocked vault: promote it into matching
      // entries so other devices pick it up (fire-and-forget; no-op if none).
      if (record) void persistSsoToEntries(host, record.provider);
      return { record };
    },

    [MessageType.SSO_DELETE]: async (payload) => {
      const { host } = payload as { host: string };
      await deleteSsoRecord(host);
      await persistSsoToEntries(host, null);
      return { ok: true };
    },

    // Vault-wide password health. Decrypts in bulk like GET_USERNAMES; the
    // response carries only ids/names/category data — never a password. The
    // HIBP breach check is opt-in (network; k-anonymity, 5 hash chars only).
    [MessageType.HEALTH_REPORT]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { checkBreaches } = payload as { checkBreaches?: boolean };
      const client = vaultClient(session);
      const full = await client.listEntriesFull();
      const items: { id: string; name: string; password: string; updatedAt?: string }[] = [];
      for (const entry of full) {
        try {
          const data = await decryptEntry(entry, session);
          if (data.password) {
            items.push({
              id: entry.id,
              name: entry.name,
              password: data.password,
              updatedAt: entry.updated_at,
            });
          }
        } catch {
          // skip entries we cannot read
        }
      }
      const report = await computeHealthReport(items, { checkBreaches: !!checkBreaches });
      return { report };
    },

    [MessageType.TOGGLE_FAVORITE]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const { entryId, favorite } = payload as { entryId: string; favorite: boolean };
      const client = vaultClient(session);
      await client.setFavorite(entryId, favorite);
      setEntriesCache(await client.listEntries());
      return { ok: true };
    },

    [MessageType.GENERATE]: async (payload) => {
      const session = getSession();
      if (!session) return { locked: true };
      const client = vaultClient(session);
      return client.generate(payload as Parameters<typeof client.generate>[0]);
    },

    // Open the toolbar popup so the user can unlock / sign in — triggered by the
    // in-page "unlock" prompt on a login field. Best-effort: action.openPopup()
    // works in Chrome 127+ from the worker, but some browsers (e.g. Firefox) only
    // allow it from a direct user-input handler, so a failure is non-fatal — the
    // user can always click the toolbar icon.
    [MessageType.OPEN_POPUP]: async () => {
      try {
        const action = browser.action as { openPopup?: () => Promise<void> };
        await action.openPopup?.();
        return { ok: true };
      } catch {
        return { ok: false };
      }
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
      const client = vaultClient(session);
      const matches = matchEntriesForUrl(url, await entriesForMatching(session));
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

      const client = vaultClient(session);
      const data = { username: pending.username, password: pending.password } as EntryData;
      const result = await createEntry(client, pending.name || pending.url, 'password', pending.url, data, session);
      clearEntryTagsCache();
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

      const client = vaultClient(session);
      // Merge into the entry's existing data — overwriting with just
      // username+password would silently drop totp_secret, notes, tags and any
      // key another client wrote. An entry that cannot be read is therefore
      // left alone; the pending save stays, so it can still be saved as new.
      let apiEntry: EntryResponse;
      let data: EntryData;
      try {
        apiEntry = await client.getEntry(entryId);
        data = (await decryptEntry(apiEntry, session)) ?? {};
      } catch (err) {
        if (err instanceof VaultLockedError) throw err;
        return { ok: false, error: 'Could not read the existing entry — nothing was changed.' };
      }
      data.username = pending.username;
      data.password = pending.password;
      const encrypted = await encryptEntry(data, session);
      // folder_id is echoed back because the backend always overwrites it.
      await client.updateEntry(entryId, { ...encrypted, folder_id: apiEntry.folder_id });
      clearEntryTagsCache();
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
  });
}

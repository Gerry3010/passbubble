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

// Passbubble as the passkey authenticator (docs/passkeys.md). The page script
// wraps navigator.credentials; the bridge content script forwards each request
// here (PASSKEY_BEGIN). This module decides whether Passbubble answers at all,
// keeps the open request in storage.session (no secrets in it), serves the
// dialog its state for a one-time token, and acts on the user's decision:
// signs an assertion with the freshly decrypted key, or creates a passkey and
// saves it into the vault BEFORE the site gets its credential. Results go back
// to the bridge with tabs.sendMessage, so no reply has to stay open while the
// dialog is up (the service worker may stop in between).
//
// Trust: the origin is taken from the browser's sender information for the
// top frame only, never from the page. Dialog and fill-box decisions are
// accepted only from our own extension frames in the request's tab, and only
// from the frame that first used the token.

import browser from 'webextension-polyfill';
import {
  base64urlDecode,
  base64urlEncode,
  buildClientDataJSON,
  createEntry,
  createPasskey,
  decryptEntry,
  reencryptEntry,
  sha256,
  signAssertion,
} from '@passbubble/shared-ts';
import type { EntryData, EntryResponse, PasskeyRecord, UnlockedSession } from '@passbubble/shared-ts';
import { MessageType, STORAGE_KEYS } from '../shared/constants.js';
import { isAndroid, readPasskeySettings, siteListed } from '../shared/passkey-settings.js';
import {
  HARDWARE_TRANSPORTS,
  clampTimeout,
  passthroughReason,
  requestWithinLimits,
  type RequestOutcome,
  type SerializedCreationOptions,
  type SerializedCredential,
  type SerializedDescriptor,
  type SerializedExtensions,
  type SerializedRequest,
  type SerializedRequestOptions,
} from '../webauthn/protocol.js';
import { checkRequestOrigin } from './rp-id.js';
import { credentialIdKey, getPasskeyIndex, invalidatePasskeyIndex, passkeysOf, rowsForRp } from './passkey-index.js';
import { clearEntryTagsCache, getSession, setEntriesCache } from './session-store.js';
import { VaultLockedError, httpStatus, loadEntriesCache, vaultClient } from './vault-client.js';
import { loadAccount } from './account-store.js';

/** What the browser tells us about a message's sender (Chrome adds origin,
 * documentId and documentLifecycle; Firefox does not). */
export interface PasskeySender {
  tab?: { id?: number };
  frameId?: number;
  url?: string;
  origin?: string;
  documentId?: string;
  documentLifecycle?: string;
}

export interface PendingPasskey {
  id: string;
  tabId: number;
  /** Chrome only: the requesting document, so a result never reaches the next one. */
  documentId?: string;
  origin: string;
  /** The requesting page's host, the dialog's headline. */
  host: string;
  rpId: string;
  type: 'create' | 'get';
  conditional: boolean;
  request: SerializedRequest;
  createdAt: number;
  deadline: number;
  /** One-time token of the dialog (modal requests). */
  uiToken?: string;
  dialogFrameId?: number;
  /** Token of the passkey rows in the fill box (conditional requests); a new
   * one for every list shown. */
  suggestToken?: string;
  suggestFrameId?: number;
  /** What the user was offered; a decision may only pick from these. */
  candidates?: Array<{ credentialId: string; entryId: string }>;
  createTargets?: string[];
  readOnlyTargets?: string[];
  excludeMatched?: boolean;
}

export type DialogState =
  | { mode: 'locked'; type: 'create' | 'get'; host: string; rpId: string; rpName?: string; loggedIn: boolean }
  | {
      mode: 'get';
      type: 'get';
      host: string;
      rpId: string;
      accounts: Array<{ credentialId: string; userName: string; entryName: string }>;
    }
  | {
      mode: 'create';
      type: 'create';
      host: string;
      rpId: string;
      rpName?: string;
      userName: string;
      userDisplayName?: string;
      targets: Array<{ entryId: string; entryName: string; userName: string }>;
      readOnlyTargets: string[];
    }
  | { mode: 'exists'; type: 'create'; host: string; rpId: string; rpName?: string };

export type PasskeyChoice =
  | { action: 'cancel' }
  | { action: 'native' }
  | { action: 'exists-ok' }
  | { action: 'select'; credentialId: string }
  | { action: 'create'; target: string };

type BeginReply =
  | { action: 'native' }
  | { action: 'ui'; id: string; uiToken: string }
  | { action: 'conditional'; id: string }
  | { action: 'error'; name: string; message: string };

const NATIVE: BeginReply = { action: 'native' };

/** Conditional requests run until the page aborts them or navigates; this
 * only bounds how long a forgotten one lingers. */
const CONDITIONAL_LIFETIME_MS = 60 * 60_000;

/** Upper bound of entries decrypted to find the "save into" choices. */
const MAX_TARGET_CANDIDATES = 25;

const TRANSPORTS = ['internal', 'hybrid'];

const GONE = 'This passkey request is no longer open.';

function notAllowed(message = 'The operation either timed out or was not allowed.'): RequestOutcome {
  return { kind: 'error', name: 'NotAllowedError', message };
}

function randomToken(): string {
  return base64urlEncode(crypto.getRandomValues(new Uint8Array(16)));
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

// ─── Pending requests in storage.session ────────────────────────────────────

let queue: Promise<unknown> = Promise.resolve();

/** Runs `fn` on the stored requests (expired ones dropped) under a lock and
 * writes them back. `fn` must not wait on the network. */
function update<T>(fn: (all: Record<string, PendingPasskey>) => T): Promise<T> {
  const run = queue.then(async () => {
    const all = await readAll();
    const result = fn(all);
    await browser.storage.session.set({ [STORAGE_KEYS.PASSKEY_PENDING]: all });
    return result;
  });
  queue = run.catch(() => undefined);
  return run;
}

async function readAll(): Promise<Record<string, PendingPasskey>> {
  let stored: unknown;
  try {
    stored = (await browser.storage.session.get(STORAGE_KEYS.PASSKEY_PENDING))[STORAGE_KEYS.PASSKEY_PENDING];
  } catch {
    stored = undefined;
  }
  const all: Record<string, PendingPasskey> = {};
  if (typeof stored !== 'object' || stored === null) return all;
  const now = Date.now();
  for (const [id, p] of Object.entries(stored as Record<string, PendingPasskey>)) {
    if (p && typeof p === 'object' && p.deadline > now) all[id] = p;
  }
  return all;
}

/** Removes a request and says whether it was still there: a decision is
 * carried out at most once. */
function take(id: string): Promise<boolean> {
  return update((all) => {
    if (!all[id]) return false;
    delete all[id];
    return true;
  });
}

// Requests taken for a create that is being saved right now, so a cancel in
// that moment is not lost if the request has to be put back (read-only entry).
const savingCancelled = new Map<string, boolean>();

/** Drops the open requests of a tab: it closed, or its top frame navigated to
 * another document (Chrome: `documentId` is the new one; Firefox: requests
 * made before `before` go). */
export function dropPendingForTab(tabId: number, opts: { documentId?: string; before?: number } = {}): Promise<void> {
  return update((all) => {
    for (const p of Object.values(all)) {
      if (p.tabId !== tabId) continue;
      if (opts.documentId && p.documentId) {
        if (p.documentId === opts.documentId) continue;
      } else if (opts.before !== undefined && p.createdAt >= opts.before) {
        continue;
      }
      delete all[p.id];
      if (savingCancelled.has(p.id)) savingCancelled.set(p.id, true);
    }
  });
}

// ─── Results to the page ────────────────────────────────────────────────────

async function push(p: PendingPasskey, outcome: RequestOutcome, keepDialog = false): Promise<void> {
  const options: { frameId: number; documentId?: string } = { frameId: 0 };
  if (p.documentId) options.documentId = p.documentId;
  try {
    await browser.tabs.sendMessage(
      p.tabId,
      { type: MessageType.PASSKEY_RESULT, payload: { id: p.id, outcome, keepDialog } },
      options as { frameId: number },
    );
  } catch {
    // the page is gone; nothing waits for the answer any more
  }
}

// ─── Sender checks ──────────────────────────────────────────────────────────

/** The top-frame page a bridge message came from, or null. */
function pageSender(sender: PasskeySender, selfOrigin: unknown): { tabId: number; documentId?: string; url: string } | null {
  const tabId = sender.tab?.id;
  if (typeof tabId !== 'number' || sender.frameId !== 0 || typeof sender.url !== 'string') return null;
  if (sender.url.startsWith(browser.runtime.getURL(''))) return null;
  let urlOrigin: string;
  try {
    urlOrigin = new URL(sender.url).origin;
  } catch {
    return null;
  }
  if (!urlOrigin || urlOrigin === 'null') return null;
  if (sender.documentLifecycle !== undefined && sender.documentLifecycle !== 'active') return null;
  if (sender.origin !== undefined) {
    if (sender.origin !== urlOrigin) return null;
  } else if (typeof selfOrigin !== 'string' || selfOrigin === 'null' || selfOrigin !== urlOrigin) {
    // Firefox reports no sender.origin; the bridge's own origin has to agree.
    return null;
  }
  if (typeof selfOrigin === 'string' && selfOrigin !== urlOrigin) return null;
  return { tabId, documentId: sender.documentId, url: sender.url };
}

/** True when `sender` is our extension frame `path` (passkey-dialog/ or
 * fill-iframe/) inside the request's tab and, once bound, the same frame. */
function extensionFrameOk(sender: PasskeySender, path: string, p: PendingPasskey, boundFrameId: number | undefined): boolean {
  if (!sender.url?.startsWith(browser.runtime.getURL(path))) return false;
  if (sender.tab?.id !== p.tabId) return false;
  if (typeof sender.frameId !== 'number' || sender.frameId === 0) return false;
  return boundFrameId === undefined || boundFrameId === sender.frameId;
}

// ─── Settings ───────────────────────────────────────────────────────────────

export async function passkeysAllowedOn(host: string): Promise<boolean> {
  const settings = await readPasskeySettings();
  if (!settings.enabled || (await isAndroid())) return false;
  return !siteListed(host, settings.blocklist);
}

// ─── Requests from the page ─────────────────────────────────────────────────

function strings(list: unknown, max = 16): string[] {
  return Array.isArray(list) ? list.filter((s): s is string => typeof s === 'string').slice(0, max) : [];
}

function descriptors(list: SerializedDescriptor[]): SerializedDescriptor[] {
  return list.map((d) => ({
    type: String(d.type),
    id: d.id,
    ...(Array.isArray(d.transports) ? { transports: strings(d.transports) } : {}),
  }));
}

function extensions(ext: SerializedExtensions): SerializedExtensions {
  return {
    names: strings(ext.names, 32),
    ...(ext.credProps === true ? { credProps: true } : {}),
    ...(typeof ext.largeBlobSupport === 'string' ? { largeBlobSupport: ext.largeBlobSupport.slice(0, 32) } : {}),
    ...(typeof ext.appid === 'string' ? { appid: ext.appid.slice(0, 2048) } : {}),
  };
}

/** A copy of a (limit-checked) request with only the fields we know. */
function sanitize(request: SerializedRequest): SerializedRequest {
  const timeout = typeof request.options.timeout === 'number' ? { timeout: request.options.timeout } : {};
  const mediation = typeof request.mediation === 'string' ? { mediation: request.mediation.slice(0, 32) } : {};
  if (request.type === 'create') {
    const o = request.options;
    const sel = o.authenticatorSelection;
    return {
      type: 'create',
      ...mediation,
      options: {
        rp: { ...(typeof o.rp.id === 'string' ? { id: o.rp.id } : {}), name: o.rp.name },
        user: { id: o.user.id, name: o.user.name, displayName: o.user.displayName },
        challenge: o.challenge,
        pubKeyCredParams: o.pubKeyCredParams.map((p) => ({ type: String(p.type), alg: Number(p.alg) })),
        ...timeout,
        excludeCredentials: descriptors(o.excludeCredentials),
        ...(sel && typeof sel === 'object'
          ? {
              authenticatorSelection: {
                ...(typeof sel.authenticatorAttachment === 'string' ? { authenticatorAttachment: sel.authenticatorAttachment } : {}),
                ...(typeof sel.residentKey === 'string' ? { residentKey: sel.residentKey } : {}),
                ...(typeof sel.requireResidentKey === 'boolean' ? { requireResidentKey: sel.requireResidentKey } : {}),
                ...(typeof sel.userVerification === 'string' ? { userVerification: sel.userVerification } : {}),
              },
            }
          : {}),
        hints: strings(o.hints),
        ...(typeof o.attestation === 'string' ? { attestation: o.attestation } : {}),
        extensions: extensions(o.extensions),
      },
    };
  }
  const o = request.options;
  return {
    type: 'get',
    ...mediation,
    options: {
      challenge: o.challenge,
      ...timeout,
      ...(typeof o.rpId === 'string' ? { rpId: o.rpId } : {}),
      allowCredentials: descriptors(o.allowCredentials),
      ...(typeof o.userVerification === 'string' ? { userVerification: o.userVerification } : {}),
      hints: strings(o.hints),
      extensions: extensions(o.extensions),
    },
  };
}

/** Only hardware keys can answer: every allowed credential lists transports,
 * all of them usb/nfc/ble/smart-card. */
function hardwareOnly(allow: SerializedDescriptor[]): boolean {
  return (
    allow.length > 0 &&
    allow.every((d) => (d.transports?.length ?? 0) > 0 && d.transports!.every((t) => HARDWARE_TRANSPORTS.has(t)))
  );
}

async function begin(payload: Record<string, unknown>, sender: PasskeySender): Promise<BeginReply> {
  const page = pageSender(sender, payload.selfOrigin);
  if (!page) return NATIVE;
  const raw = payload.request as SerializedRequest;
  if (!requestWithinLimits(raw) || passthroughReason(raw) !== null) return NATIVE;
  const request = sanitize(raw);
  const requestedRpId = request.type === 'create' ? request.options.rp.id : request.options.rpId;
  const check = checkRequestOrigin(page.url, requestedRpId);
  if (!check.ok) return NATIVE;
  if (!(await passkeysAllowedOn(check.host))) return NATIVE;

  const conditional = request.type === 'get' && request.mediation === 'conditional';
  if (request.type === 'get' && !conditional) {
    const allow = request.options.allowCredentials;
    const session = getSession();
    if (!session) {
      if (hardwareOnly(allow)) return NATIVE;
    } else {
      const rows = rowsForRp(await getPasskeyIndex(session), check.rpId, allow.map((d) => d.id));
      if (rows.length === 0) return NATIVE;
    }
  }

  return update((all): BeginReply => {
    if (!conditional && Object.values(all).some((p) => p.tabId === page.tabId && !p.conditional)) {
      return { action: 'error', name: 'NotAllowedError', message: 'Another passkey request is already open on this page.' };
    }
    if (conditional) {
      for (const p of Object.values(all)) if (p.tabId === page.tabId && p.conditional) delete all[p.id];
    }
    const now = Date.now();
    const pending: PendingPasskey = {
      id: randomToken(),
      tabId: page.tabId,
      ...(page.documentId ? { documentId: page.documentId } : {}),
      origin: check.origin,
      host: check.host,
      rpId: check.rpId,
      type: request.type,
      conditional,
      request,
      createdAt: now,
      deadline: now + (conditional ? CONDITIONAL_LIFETIME_MS : clampTimeout(request.options.timeout)),
      ...(conditional ? {} : { uiToken: randomToken() }),
    };
    all[pending.id] = pending;
    return conditional ? { action: 'conditional', id: pending.id } : { action: 'ui', id: pending.id, uiToken: pending.uiToken! };
  });
}

async function cancel(payload: Record<string, unknown>, sender: PasskeySender): Promise<{ ok: boolean }> {
  const tabId = sender.tab?.id;
  const id = payload.id;
  if (sender.frameId !== 0 || typeof tabId !== 'number' || typeof id !== 'string') return { ok: false };
  if (savingCancelled.has(id)) savingCancelled.set(id, true);
  return update((all) => {
    const p = all[id];
    if (!p || p.tabId !== tabId) return { ok: false };
    delete all[id];
    return { ok: true };
  });
}

// ─── The dialog ─────────────────────────────────────────────────────────────

function hostOfEntryUrl(url: string | undefined): string {
  if (!url) return '';
  try {
    return new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(url) ? url : `https://${url}`).hostname.toLowerCase();
  } catch {
    return '';
  }
}

/** Password entries for the site (URL host is the rp id or below it) whose
 * user name is the new passkey's, ignoring case. */
async function createTargetsFor(
  session: UnlockedSession,
  rpId: string,
  userName: string,
): Promise<Array<{ entryId: string; entryName: string; userName: string }>> {
  const want = userName.toLowerCase();
  const entries = (await loadEntriesCache(session)).filter((e) => {
    if (e.type !== 'password') return false;
    const h = hostOfEntryUrl(e.url);
    return !!h && (h === rpId || h.endsWith(`.${rpId}`));
  });
  const client = vaultClient(session);
  const out: Array<{ entryId: string; entryName: string; userName: string }> = [];
  for (const e of entries.slice(0, MAX_TARGET_CANDIDATES)) {
    try {
      const data = await decryptEntry(await client.getEntry(e.id), session);
      const name = typeof data.username === 'string' ? data.username : '';
      if (name.toLowerCase() === want) out.push({ entryId: e.id, entryName: e.name, userName: name });
    } catch (err) {
      if (err instanceof VaultLockedError) throw err;
    }
  }
  return out;
}

function setDerived(id: string, patch: Partial<PendingPasskey>): Promise<boolean> {
  return update((all) => {
    if (!all[id]) return false;
    all[id] = { ...all[id], ...patch };
    return true;
  });
}

async function lockedState(p: PendingPasskey): Promise<DialogState> {
  const rpName = p.request.type === 'create' ? p.request.options.rp.name || undefined : undefined;
  return { mode: 'locked', type: p.type, host: p.host, rpId: p.rpId, ...(rpName ? { rpName } : {}), loggedIn: !!(await loadAccount()) };
}

async function describe(p: PendingPasskey): Promise<DialogState> {
  const base = { host: p.host, rpId: p.rpId };
  const rpName = p.request.type === 'create' ? p.request.options.rp.name || undefined : undefined;
  const session = getSession();
  if (!session) return lockedState(p);
  const index = await getPasskeyIndex(session);
  if (p.request.type === 'get') {
    const rows = rowsForRp(index, p.rpId, p.request.options.allowCredentials.map((d) => d.id));
    await setDerived(p.id, { candidates: rows.map((r) => ({ credentialId: r.credentialId, entryId: r.entryId })) });
    return {
      mode: 'get',
      type: 'get',
      ...base,
      accounts: rows.map((r) => ({ credentialId: r.credentialId, userName: r.userName, entryName: r.entryName })),
    };
  }
  const o = p.request.options;
  const excluded = new Set(o.excludeCredentials.map((d) => credentialIdKey(d.id)).filter(Boolean));
  if (rowsForRp(index, p.rpId).some((r) => excluded.has(r.credentialId))) {
    await setDerived(p.id, { excludeMatched: true });
    return { mode: 'exists', type: 'create', ...base, ...(rpName ? { rpName } : {}) };
  }
  const targets = await createTargetsFor(session, p.rpId, o.user.name);
  await setDerived(p.id, { excludeMatched: false, createTargets: targets.map((t) => t.entryId) });
  return {
    mode: 'create',
    type: 'create',
    ...base,
    ...(rpName ? { rpName } : {}),
    userName: o.user.name,
    ...(o.user.displayName ? { userDisplayName: o.user.displayName } : {}),
    targets,
    readOnlyTargets: p.readOnlyTargets ?? [],
  };
}

async function uiState(
  payload: Record<string, unknown>,
  sender: PasskeySender,
): Promise<{ state?: DialogState; error?: string; tokenValid?: boolean }> {
  const token = payload.token;
  if (typeof token !== 'string' || !token) return { error: GONE };
  const found = await update((all) => {
    const p = Object.values(all).find((x) => x.uiToken === token);
    if (!p || !extensionFrameOk(sender, 'passkey-dialog/', p, p.dialogFrameId)) return null;
    p.dialogFrameId ??= sender.frameId;
    return { ...p };
  });
  if (!found) return { error: GONE };
  try {
    return { state: await describe(found) };
  } catch (err) {
    // The vault locked (or the server signed us out) meanwhile: say so.
    if (err instanceof VaultLockedError) return { state: await lockedState(found) };
    // The token is good, the vault just could not be read right now.
    return { tokenValid: true, error: `Could not read your passkeys: ${errorMessage(err)}` };
  }
}

// ─── The user's decision ────────────────────────────────────────────────────

function creationExtensionResults(ext: SerializedExtensions): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  if (ext.names.includes('credProps') && ext.credProps) out.credProps = { rk: true };
  if (ext.names.includes('prf')) out.prf = { enabled: false };
  if (ext.names.includes('largeBlob')) out.largeBlob = { supported: false };
  return out;
}

function assertionExtensionResults(ext: SerializedExtensions): Record<string, unknown> {
  return ext.names.includes('appid') && ext.appid ? { appid: false } : {};
}

function sameUserHandle(a: unknown, b: string): boolean {
  return typeof a === 'string' && credentialIdKey(a) === b;
}

/** Saves `record` into the chosen entry (merged by rp id + user handle) or a
 * new one. Throws when nothing could be saved. */
async function savePasskey(
  session: UnlockedSession,
  target: string,
  rpId: string,
  opts: SerializedCreationOptions,
  record: PasskeyRecord,
): Promise<void> {
  const client = vaultClient(session);
  if (target === 'new') {
    if (session.pubX25519?.length !== 32) throw new Error('Session keys incomplete — please log out and log in again.');
    const data: EntryData = { username: opts.user.name, password: '', passkeys: [record] };
    await createEntry(client, opts.rp.name || rpId, 'password', `https://${rpId}`, data, session);
  } else {
    const apiEntry: EntryResponse = await client.getEntry(target);
    if (apiEntry.type !== 'password') throw new Error('Passkeys can only be saved in a login entry.');
    const data = ((await decryptEntry(apiEntry, session)) ?? {}) as EntryData;
    const handle = record.user_handle;
    const existing: unknown[] = Array.isArray(data.passkeys) ? data.passkeys : [];
    const kept = existing.filter((r) => {
      const rec = r as Partial<PasskeyRecord> | null;
      const sameAccount =
        typeof rec === 'object' && rec !== null && String(rec.rp_id ?? '').toLowerCase() === rpId && sameUserHandle(rec.user_handle, handle);
      return !sameAccount;
    });
    data.passkeys = [...(kept as PasskeyRecord[]), record];
    const encrypted = await reencryptEntry(data, apiEntry, session);
    await client.updateEntry(target, { ...encrypted, folder_id: apiEntry.folder_id });
  }
  clearEntryTagsCache();
  invalidatePasskeyIndex();
  try {
    setEntriesCache(await client.listEntries());
  } catch {
    // saved; the next listing rebuilds the cache
  }
}

async function completeCreate(p: PendingPasskey, target: string): Promise<Record<string, unknown>> {
  const session = getSession();
  if (!session) return { ok: false, locked: true, error: 'The vault is locked.' };
  if (target !== 'new' && (!(p.createTargets ?? []).includes(target) || (p.readOnlyTargets ?? []).includes(target))) {
    return { ok: false, error: 'That entry was not offered for this passkey.' };
  }
  if (!(await take(p.id))) return { ok: false, error: GONE };
  const opts = p.request.options as SerializedCreationOptions;
  savingCancelled.set(p.id, false);
  let created: Awaited<ReturnType<typeof createPasskey>>;
  try {
    created = await createPasskey({
      rpId: p.rpId,
      ...(opts.rp.name ? { rpName: opts.rp.name } : {}),
      userHandle: base64urlDecode(opts.user.id),
      userName: opts.user.name,
      ...(opts.user.displayName ? { userDisplayName: opts.user.displayName } : {}),
    });
    await savePasskey(session, target, p.rpId, opts, created.record);
  } catch (err) {
    const cancelled = savingCancelled.get(p.id) === true;
    savingCancelled.delete(p.id);
    if (target !== 'new' && httpStatus(err) === 403 && !cancelled && Date.now() < p.deadline) {
      // A shared entry we may only read: the request stays open, so the user
      // can save the passkey as a new entry instead.
      await update((all) => {
        all[p.id] = { ...p, readOnlyTargets: [...(p.readOnlyTargets ?? []), target] };
      });
      return { ok: false, readOnly: true, error: 'You only have read access to this entry. Save the passkey as a new entry instead.' };
    }
    // Fail closed: the site must never register a passkey the vault does not hold.
    await push(p, notAllowed('The passkey could not be saved.'), true);
    if (err instanceof VaultLockedError) throw err;
    return { ok: false, error: `Could not save the passkey: ${errorMessage(err)}` };
  }
  savingCancelled.delete(p.id);
  const clientDataJSON = buildClientDataJSON({ type: 'webauthn.create', challenge: opts.challenge, origin: p.origin });
  const credential: SerializedCredential = {
    type: 'create',
    id: created.record.credential_id,
    clientDataJSON: base64urlEncode(clientDataJSON),
    authenticatorData: base64urlEncode(created.authenticatorData),
    attestationObject: base64urlEncode(created.attestationObject),
    publicKey: base64urlEncode(created.publicKeySpki),
    publicKeyAlgorithm: -7,
    transports: TRANSPORTS,
    clientExtensionResults: creationExtensionResults(opts.extensions),
  };
  await push(p, { kind: 'credential', credential });
  return { ok: true };
}

async function completeGet(p: PendingPasskey, credentialId: unknown): Promise<Record<string, unknown>> {
  const session = getSession();
  if (!session) return { ok: false, locked: true, error: 'The vault is locked.' };
  const key = typeof credentialId === 'string' ? credentialIdKey(credentialId) : '';
  const candidate = key ? p.candidates?.find((c) => c.credentialId === key) : undefined;
  if (!candidate) return { ok: false, error: 'That passkey was not offered for this request.' };
  if (!(await take(p.id))) return { ok: false, error: GONE };
  const opts = p.request.options as SerializedRequestOptions;
  try {
    // Never trust the index for the key: read the entry as it is now.
    const apiEntry = await vaultClient(session).getEntry(candidate.entryId);
    if (apiEntry.type !== 'password') throw new Error('The passkey is no longer in the vault.');
    const record = passkeysOf(await decryptEntry(apiEntry, session)).find(
      (r) => credentialIdKey(r.credential_id) === candidate.credentialId && r.rp_id.toLowerCase() === p.rpId,
    );
    if (!record) throw new Error('The passkey is no longer in the vault.');
    const allowed = opts.allowCredentials.map((d) => credentialIdKey(d.id)).filter(Boolean);
    if (allowed.length > 0 && !allowed.includes(candidate.credentialId)) throw new Error('The site does not accept this passkey.');
    const clientDataJSON = buildClientDataJSON({ type: 'webauthn.get', challenge: opts.challenge, origin: p.origin });
    const assertion = await signAssertion({ ...record, rp_id: p.rpId }, p.rpId, await sha256(clientDataJSON));
    const credential: SerializedCredential = {
      type: 'get',
      id: candidate.credentialId,
      clientDataJSON: base64urlEncode(clientDataJSON),
      authenticatorData: base64urlEncode(assertion.authenticatorData),
      signature: base64urlEncode(assertion.signature),
      userHandle: base64urlEncode(assertion.userHandle),
      transports: TRANSPORTS,
      clientExtensionResults: assertionExtensionResults(opts.extensions),
    };
    await push(p, { kind: 'credential', credential });
    return { ok: true };
  } catch (err) {
    await push(p, notAllowed(), true);
    if (err instanceof VaultLockedError) throw err;
    return { ok: false, error: errorMessage(err) };
  }
}

async function complete(payload: Record<string, unknown>, sender: PasskeySender): Promise<Record<string, unknown>> {
  const token = payload.token;
  const choice = payload.choice as PasskeyChoice | undefined;
  if (typeof token !== 'string' || !token || !choice || typeof choice.action !== 'string') return { ok: false, error: GONE };
  const found = await update((all) => {
    for (const p of Object.values(all)) {
      if (p.uiToken === token) {
        if (!extensionFrameOk(sender, 'passkey-dialog/', p, p.dialogFrameId)) return null;
        p.dialogFrameId ??= sender.frameId;
        return { p: { ...p }, via: 'dialog' as const };
      }
      if (p.suggestToken === token) {
        if (!extensionFrameOk(sender, 'fill-iframe/', p, p.suggestFrameId)) return null;
        p.suggestFrameId ??= sender.frameId;
        return { p: { ...p }, via: 'fill' as const };
      }
    }
    return null;
  });
  if (!found) return { ok: false, error: GONE };
  const { p, via } = found;

  // The fill box only picks a passkey; everything else belongs to the dialog.
  if (via === 'fill' && choice.action !== 'select') return { ok: false, error: GONE };

  switch (choice.action) {
    case 'cancel':
      if (!(await take(p.id))) return { ok: false, error: GONE };
      await push(p, notAllowed('The user cancelled the request.'));
      return { ok: true };
    case 'native':
      if (!(await take(p.id))) return { ok: false, error: GONE };
      await push(p, { kind: 'native' });
      return { ok: true };
    case 'exists-ok':
      if (p.type !== 'create' || !p.excludeMatched) return { ok: false, error: GONE };
      if (!(await take(p.id))) return { ok: false, error: GONE };
      await push(p, {
        kind: 'error',
        name: 'InvalidStateError',
        message: 'The user attempted to register an authenticator that contains one of the credentials already registered with the relying party.',
      });
      return { ok: true };
    case 'select':
      if (p.type !== 'get') return { ok: false, error: GONE };
      return completeGet(p, choice.credentialId);
    case 'create':
      if (p.type !== 'create' || p.excludeMatched !== false || typeof choice.target !== 'string') {
        return { ok: false, error: GONE };
      }
      return completeCreate(p, choice.target);
    default:
      return { ok: false, error: GONE };
  }
}

// ─── Conditional UI (passkeys in the fill box) ──────────────────────────────

async function suggestions(_payload: Record<string, unknown>, sender: PasskeySender): Promise<Record<string, unknown>> {
  const tabId = sender.tab?.id;
  if (sender.frameId !== 0 || typeof tabId !== 'number' || typeof sender.url !== 'string') return { rows: [] };
  let origin: string;
  try {
    origin = new URL(sender.url).origin;
  } catch {
    return { rows: [] };
  }
  const p = Object.values(await readAll()).find(
    (x) =>
      x.tabId === tabId &&
      x.conditional &&
      x.origin === origin &&
      (!x.documentId || !sender.documentId || x.documentId === sender.documentId),
  );
  if (!p || p.request.type !== 'get') return { rows: [] };
  const session = getSession();
  if (!session) return { rows: [], locked: true };
  const rows = rowsForRp(await getPasskeyIndex(session), p.rpId, p.request.options.allowCredentials.map((d) => d.id));
  if (rows.length === 0) return { rows: [] };
  const token = randomToken();
  const stored = await setDerived(p.id, {
    suggestToken: token,
    suggestFrameId: undefined,
    candidates: rows.map((r) => ({ credentialId: r.credentialId, entryId: r.entryId })),
  });
  if (!stored) return { rows: [] };
  return {
    token,
    rows: rows.map((r) => ({ credentialId: r.credentialId, userName: r.userName, entryName: r.entryName })),
  };
}

// ─── Popup: remove a passkey from an entry ──────────────────────────────────

async function deletePasskey(payload: Record<string, unknown>): Promise<Record<string, unknown>> {
  const session = getSession();
  if (!session) return { locked: true };
  const { entryId, credentialId } = payload as { entryId?: unknown; credentialId?: unknown };
  if (typeof entryId !== 'string' || typeof credentialId !== 'string') return { ok: false, error: 'Nothing to delete.' };
  const key = credentialIdKey(credentialId);
  const client = vaultClient(session);
  let apiEntry: EntryResponse;
  let encrypted: { encrypted_data: string; data_nonce: string };
  try {
    apiEntry = await client.getEntry(entryId);
    const data = ((await decryptEntry(apiEntry, session)) ?? {}) as EntryData;
    const list: unknown[] = Array.isArray(data.passkeys) ? data.passkeys : [];
    const kept = list.filter((r) => {
      const id = (r as { credential_id?: unknown } | null)?.credential_id;
      return !(typeof id === 'string' && credentialIdKey(id) === key);
    });
    if (kept.length === list.length) return { ok: false, error: 'This passkey is no longer in the entry.' };
    if (kept.length > 0) data.passkeys = kept as PasskeyRecord[];
    else delete data.passkeys;
    encrypted = await reencryptEntry(data, apiEntry, session);
  } catch (err) {
    if (err instanceof VaultLockedError) throw err;
    return { ok: false, error: 'Could not read the entry — nothing was changed.' };
  }
  try {
    await client.updateEntry(entryId, { ...encrypted, folder_id: apiEntry.folder_id });
  } catch (err) {
    if (err instanceof VaultLockedError) throw err;
    return {
      ok: false,
      error:
        httpStatus(err) === 403
          ? 'You only have read access to this entry — nothing was changed.'
          : `Could not save the entry: ${errorMessage(err)}`,
    };
  }
  clearEntryTagsCache();
  invalidatePasskeyIndex();
  try {
    setEntriesCache(await client.listEntries());
  } catch {
    // saved; the next listing rebuilds the cache
  }
  return { ok: true };
}

// ─── Handlers ───────────────────────────────────────────────────────────────

type PasskeyHandler = (payload: Record<string, unknown>, sender: PasskeySender) => Promise<unknown>;

export function passkeyHandlers(): Record<string, PasskeyHandler> {
  return {
    [MessageType.PASSKEY_BEGIN]: begin,
    [MessageType.PASSKEY_CANCEL]: cancel,
    [MessageType.PASSKEY_UI_STATE]: uiState,
    [MessageType.PASSKEY_COMPLETE]: complete,
    [MessageType.PASSKEY_SUGGESTIONS]: suggestions,
    [MessageType.PASSKEY_DELETE]: (payload) => deletePasskey(payload),
  };
}

/** For tests: the stored open requests. */
export async function pendingPasskeys(): Promise<Record<string, PendingPasskey>> {
  return readAll();
}

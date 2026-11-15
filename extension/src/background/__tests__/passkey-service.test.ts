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

// The passkey authenticator in the background, through buildHandlers() like
// the service worker calls it. The browser API, the HTTP client and the entry
// encryption are mocked; the passkey crypto (key generation, CBOR, signing)
// is the real shared-ts code, and its signatures are verified with WebCrypto.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { EntryData, EntryResponse, PasskeyRecord, UnlockedSession } from '@passbubble/shared-ts';

const stores = vi.hoisted(() => ({ session: new Map<string, unknown>(), local: new Map<string, unknown>() }));
const tabsSendMessage = vi.hoisted(() => vi.fn(async () => undefined));
const platform = vi.hoisted(() => ({ os: 'linux' }));

vi.mock('webextension-polyfill', () => {
  const area = (map: Map<string, unknown>) => ({
    get: vi.fn(async (keys?: string | string[] | null) => {
      const list = keys == null ? [...map.keys()] : Array.isArray(keys) ? keys : [keys];
      const out: Record<string, unknown> = {};
      for (const k of list) if (map.has(k)) out[k] = structuredClone(map.get(k));
      return out;
    }),
    set: vi.fn(async (obj: Record<string, unknown>) => {
      for (const [k, v] of Object.entries(obj)) map.set(k, structuredClone(v));
    }),
    remove: vi.fn(async (keys: string | string[]) => {
      for (const k of Array.isArray(keys) ? keys : [keys]) map.delete(k);
    }),
  });
  return {
    default: {
      storage: { session: area(stores.session), local: area(stores.local), sync: { get: vi.fn(async () => ({})), set: vi.fn() } },
      runtime: {
        getURL: (p: string) => `chrome-extension://ext/${p}`,
        getPlatformInfo: vi.fn(async () => ({ ...platform })),
      },
      tabs: { sendMessage: tabsSendMessage },
      alarms: { create: vi.fn(), clear: vi.fn() },
    },
  };
});

const client = vi.hoisted(() => ({
  setTokens: vi.fn(),
  listEntries: vi.fn(),
  listEntriesFull: vi.fn(),
  getEntry: vi.fn(),
  updateEntry: vi.fn(),
}));
const decryptEntryMock = vi.hoisted(() => vi.fn());
const reencryptEntryMock = vi.hoisted(() => vi.fn());
const createEntryMock = vi.hoisted(() => vi.fn());

vi.mock('@passbubble/shared-ts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@passbubble/shared-ts')>()),
  PassbubbleClient: vi.fn().mockImplementation(() => client),
  decryptEntry: decryptEntryMock,
  reencryptEntry: reencryptEntryMock,
  createEntry: createEntryMock,
}));

import { base64urlDecode, base64urlEncode, createPasskey } from '@passbubble/shared-ts';
import { buildHandlers } from '../message-handler.js';
import { dropPendingForTab, pendingPasskeys } from '../passkey-service.js';
import { clearSession, setSession } from '../session-store.js';
import { MessageType, STORAGE_KEYS } from '../../shared/constants.js';
import type { SerializedRequest } from '../../webauthn/protocol.js';

// ─── Fixtures ───────────────────────────────────────────────────────────────

/** DER ECDSA signature → the r ‖ s form WebCrypto verifies. */
function derToRaw(der: Uint8Array): Uint8Array {
  expect(der[0]).toBe(0x30);
  let i = 2;
  const part = () => {
    expect(der[i]).toBe(0x02);
    const len = der[i + 1];
    const value = der.slice(i + 2, i + 2 + len);
    i += 2 + len;
    const trimmed = value[0] === 0 ? value.slice(1) : value;
    const out = new Uint8Array(32);
    out.set(trimmed, 32 - trimmed.length);
    return out;
  };
  const r = part();
  const s = part();
  return new Uint8Array([...r, ...s]);
}

async function verifies(spki: Uint8Array, signed: Uint8Array, der: Uint8Array): Promise<boolean> {
  const key = await crypto.subtle.importKey('spki', spki as BufferSource, { name: 'ECDSA', namedCurve: 'P-256' }, false, ['verify']);
  return crypto.subtle.verify({ name: 'ECDSA', hash: 'SHA-256' }, key, derToRaw(der) as BufferSource, signed as BufferSource);
}

function makeSession(): UnlockedSession {
  return {
    privX25519: new Uint8Array(32),
    privMLKEM: new Uint8Array(64),
    pubX25519: new Uint8Array(32),
    pubMLKEM: new Uint8Array(1184),
    userId: 'u1',
    userEmail: 'me@example.org',
    userName: 'Me',
    role: 'user',
    accessToken: 'at',
    refreshToken: '',
    accessTokenExpiresAt: Date.now() + 3_600_000,
    serverUrl: 'https://srv',
    kdfSalt: new Uint8Array(16),
    kdfTime: 3,
    kdfMemory: 65536,
    encPrivX25519: new Uint8Array(60),
    encPrivMLKEM: new Uint8Array(60),
  };
}

const b64u = (s: string) => base64urlEncode(new TextEncoder().encode(s));
const CHALLENGE = base64urlEncode(Uint8Array.from({ length: 32 }, (_, i) => i));
const USER_ID = b64u('user-1234');

let entries: EntryResponse[];
let vault: Record<string, EntryData & Record<string, unknown>>;
let stamp = 0;
let lastWritten: EntryData | null;

function entry(id: string, over: Partial<EntryResponse> = {}): EntryResponse {
  return {
    id,
    owner_id: 'u1',
    type: 'password',
    name: `Entry ${id}`,
    url: 'https://example.com',
    folder_id: 'f1',
    created_at: '',
    updated_at: `t${stamp++}`,
    encrypted_data: 'blob',
    entry_key: { user_id: 'u1', encrypted_key: 'k' },
    ...over,
  };
}

const PAGE = {
  tab: { id: 7 },
  frameId: 0,
  url: 'https://login.example.com/signin',
  origin: 'https://login.example.com',
  documentId: 'doc-1',
  documentLifecycle: 'active',
};
const DIALOG = { tab: { id: 7 }, frameId: 42, url: 'chrome-extension://ext/passkey-dialog/index.html' };
const FILL = { tab: { id: 7 }, frameId: 43, url: 'chrome-extension://ext/fill-iframe/index.html' };

function createReq(over: Partial<Extract<SerializedRequest, { type: 'create' }>['options']> = {}, mediation?: string): SerializedRequest {
  return {
    type: 'create',
    ...(mediation ? { mediation } : {}),
    options: {
      rp: { id: 'example.com', name: 'Example' },
      user: { id: USER_ID, name: 'alice@example.com', displayName: 'Alice' },
      challenge: CHALLENGE,
      pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
      excludeCredentials: [],
      hints: [],
      extensions: { names: [] },
      ...over,
    },
  };
}

function getReq(over: Partial<Extract<SerializedRequest, { type: 'get' }>['options']> = {}, mediation?: string): SerializedRequest {
  return {
    type: 'get',
    ...(mediation ? { mediation } : {}),
    options: { challenge: CHALLENGE, rpId: 'example.com', allowCredentials: [], hints: [], extensions: { names: [] }, ...over },
  };
}

async function storedPasskey(over: Partial<PasskeyRecord> = {}): Promise<PasskeyRecord> {
  const { record } = await createPasskey({ rpId: 'example.com', userHandle: base64urlDecode(USER_ID), userName: 'alice@example.com' });
  return { ...record, ...over };
}

let handlers: ReturnType<typeof buildHandlers>;
type Reply = Record<string, unknown> & { action?: string; id?: string; uiToken?: string };
const call = (type: string, payload: Record<string, unknown>, sender: object) => handlers[type](payload, sender) as Promise<Reply>;
const begin = (request: SerializedRequest, sender: object = PAGE, selfOrigin?: string) =>
  call(MessageType.PASSKEY_BEGIN, { request, ...(selfOrigin !== undefined ? { selfOrigin } : {}) }, sender);
const uiState = (token: string, sender: object = DIALOG) => call(MessageType.PASSKEY_UI_STATE, { token }, sender);
const complete = (token: string, choice: Record<string, unknown>, sender: object = DIALOG) =>
  call(MessageType.PASSKEY_COMPLETE, { token, choice }, sender);

function pushes(): Array<{ tabId: number; id: string; outcome: Record<string, unknown>; keepDialog: boolean; options: unknown }> {
  return tabsSendMessage.mock.calls.map((c) => {
    const [tabId, msg, options] = c as unknown as [number, { type: string; payload: { id: string; outcome: Record<string, unknown>; keepDialog: boolean } }, unknown];
    expect(msg.type).toBe(MessageType.PASSKEY_RESULT);
    return { tabId, ...msg.payload, options };
  });
}

beforeEach(async () => {
  stores.session.clear();
  stores.local.clear();
  platform.os = 'linux';
  tabsSendMessage.mockClear();
  await clearSession();
  await setSession(makeSession());
  entries = [entry('e1')];
  vault = { e1: { username: 'alice@example.com', password: 'pw', notes: 'keep me', extra: { from: 'newer app' } } };
  lastWritten = null;
  client.listEntries.mockReset().mockImplementation(async () => entries.map((e) => ({ ...e })));
  client.listEntriesFull.mockReset().mockImplementation(async () => entries.map((e) => ({ ...e })));
  client.getEntry.mockReset().mockImplementation(async (id: string) => {
    const e = entries.find((x) => x.id === id);
    if (!e) throw Object.assign(new Error('not found'), { status: 404 });
    return { ...e };
  });
  client.updateEntry.mockReset().mockImplementation(async (id: string) => {
    vault[id] = structuredClone(lastWritten!) as EntryData & Record<string, unknown>;
    entries = entries.map((e) => (e.id === id ? { ...e, updated_at: `t${stamp++}` } : e));
  });
  decryptEntryMock.mockReset().mockImplementation(async (e: EntryResponse) => structuredClone(vault[e.id]));
  reencryptEntryMock.mockReset().mockImplementation(async (data: EntryData) => {
    lastWritten = structuredClone(data);
    return { encrypted_data: 'ed', data_nonce: 'dn' };
  });
  createEntryMock.mockReset().mockImplementation(async (_c: unknown, name: string, type: string, url: string, data: EntryData) => {
    const e = entry(`new${stamp}`, { name, type, url });
    entries.push(e);
    vault[e.id] = structuredClone(data) as EntryData & Record<string, unknown>;
    return { id: e.id };
  });
  handlers = buildHandlers();
});

// ─── Origin and rp id ───────────────────────────────────────────────────────

describe('PASSKEY_BEGIN: who may ask', () => {
  it('opens the dialog for a top-frame https page', async () => {
    const reply = await begin(createReq());
    expect(reply).toMatchObject({ action: 'ui' });
    expect(reply.id).toMatch(/^[A-Za-z0-9_-]{22}$/);
    expect(reply.uiToken).toMatch(/^[A-Za-z0-9_-]{22}$/);
    const [pending] = Object.values(await pendingPasskeys());
    expect(pending).toMatchObject({ tabId: 7, documentId: 'doc-1', origin: 'https://login.example.com', host: 'login.example.com', rpId: 'example.com' });
  });

  it('goes to the browser for frames, foreign tabs and stale documents', async () => {
    expect(await begin(createReq(), { ...PAGE, frameId: 3 })).toEqual({ action: 'native' });
    expect(await begin(createReq(), { ...PAGE, tab: undefined })).toEqual({ action: 'native' });
    expect(await begin(createReq(), { ...PAGE, origin: 'https://evil.example' })).toEqual({ action: 'native' });
    expect(await begin(createReq(), { ...PAGE, documentLifecycle: 'prerender' })).toEqual({ action: 'native' });
    expect(await begin(createReq(), { ...PAGE, url: 'chrome-extension://ext/popup/index.html', origin: 'chrome-extension://ext' })).toEqual({
      action: 'native',
    });
  });

  it('never takes the origin from the page message', async () => {
    // The payload's selfOrigin must agree with the sender, not replace it.
    expect(await begin(createReq(), PAGE, 'https://example.com')).toEqual({ action: 'native' });
  });

  it('on Firefox (no sender.origin) requires the bridge origin to match', async () => {
    const firefox = { tab: { id: 7 }, frameId: 0, url: 'https://login.example.com/signin' };
    expect(await begin(createReq(), firefox)).toEqual({ action: 'native' });
    expect(await begin(createReq(), firefox, 'null')).toEqual({ action: 'native' });
    expect(await begin(createReq(), firefox, 'https://other.example')).toEqual({ action: 'native' });
    expect(await begin(createReq(), firefox, 'https://login.example.com')).toMatchObject({ action: 'ui' });
  });

  it('checks the rp id against the page: suffixes, www, public suffixes, IPs', async () => {
    const at = (url: string) => ({ ...PAGE, url, origin: new URL(url).origin });
    expect(await begin(createReq({ rp: { id: 'github.io', name: 'x' } }), at('https://alice.github.io/'))).toEqual({ action: 'native' });
    expect(await begin(createReq({ rp: { id: 'co.uk', name: 'x' } }), at('https://shop.example.co.uk/'))).toEqual({ action: 'native' });
    expect(await begin(createReq({ rp: { id: 'www.example.com', name: 'x' } }), at('https://example.com/'))).toEqual({ action: 'native' });
    expect(await begin(createReq({ rp: { id: 'evil.com', name: 'x' } }))).toEqual({ action: 'native' });
    expect(await begin(createReq({ rp: { name: 'x' } }), at('http://example.com/'))).toEqual({ action: 'native' });
    expect(await begin(createReq({ rp: { name: 'x' } }), at('https://10.0.0.1/'))).toEqual({ action: 'native' });
  });

  it('answers http://localhost', async () => {
    const local = { ...PAGE, url: 'http://localhost:5173/', origin: 'http://localhost:5173' };
    expect(await begin(createReq({ rp: { name: 'Dev' } }), local)).toMatchObject({ action: 'ui' });
  });

  it('leaves to the browser what the page script would have passed through', async () => {
    expect(await begin(createReq({ hints: ['security-key'] }))).toEqual({ action: 'native' });
    expect(await begin(createReq({ pubKeyCredParams: [{ type: 'public-key', alg: -257 }] }))).toEqual({ action: 'native' });
    expect(await begin(createReq({ attestation: 'enterprise' }))).toEqual({ action: 'native' });
    expect(await begin(createReq({ challenge: base64urlEncode(new Uint8Array(2000)) }))).toEqual({ action: 'native' });
    expect(await begin(createReq({}, 'conditional'))).toEqual({ action: 'native' });
  });

  it('respects the feature switch, the site list and Firefox for Android', async () => {
    stores.local.set(STORAGE_KEYS.PASSKEYS_ENABLED, false);
    expect(await begin(createReq())).toEqual({ action: 'native' });
    stores.local.set(STORAGE_KEYS.PASSKEYS_ENABLED, true);
    stores.local.set(STORAGE_KEYS.PASSKEY_BLOCKLIST, ['example.com']);
    expect(await begin(createReq())).toEqual({ action: 'native' });
    stores.local.set(STORAGE_KEYS.PASSKEY_BLOCKLIST, []);
    platform.os = 'android';
    expect(await begin(createReq())).toEqual({ action: 'native' });
  });

});

describe('PASSKEY_BEGIN: locked vault and sign-in candidates', () => {
  it('passes hardware-key-only sign-ins to the browser while locked', async () => {
    await clearSession();
    const allow = (transports?: string[]) => [{ type: 'public-key', id: b64u('cred'), ...(transports ? { transports } : {}) }];
    expect(await begin(getReq({ allowCredentials: allow(['usb', 'nfc']) }))).toEqual({ action: 'native' });
    expect(await begin(getReq({ allowCredentials: allow(['usb', 'internal']) }))).toMatchObject({ action: 'ui' });
  });

  it('shows the locked dialog for other sign-ins and creations while locked', async () => {
    await clearSession();
    const reply = await begin(getReq({ allowCredentials: [{ type: 'public-key', id: b64u('cred') }] }));
    expect(reply).toMatchObject({ action: 'ui' });
    expect(await uiState(reply.uiToken!)).toEqual({
      state: { mode: 'locked', type: 'get', host: 'login.example.com', rpId: 'example.com', loggedIn: false },
    });
  });

  it('passes a sign-in without matching passkeys to the browser', async () => {
    expect(await begin(getReq())).toEqual({ action: 'native' });
    vault.e1.passkeys = [await storedPasskey({ rp_id: 'other.com' })];
    expect(await begin(getReq())).toEqual({ action: 'native' });
    vault.e1.passkeys = [await storedPasskey()];
    entries = [entry('e1')]; // a new listing stamp
    expect(await begin(getReq({ allowCredentials: [{ type: 'public-key', id: b64u('someone else') }] }))).toEqual({ action: 'native' });
  });
});

describe('dialog state when the vault cannot be read', () => {
  it('reports the problem with a valid token, and a lock as the locked state', async () => {
    const { uiToken } = await begin(createReq());
    client.listEntries.mockRejectedValueOnce(new TypeError('Failed to fetch'));
    expect(await uiState(uiToken!)).toEqual({ tokenValid: true, error: 'Could not read your passkeys: Failed to fetch' });
    await clearSession();
    expect(await uiState(uiToken!)).toMatchObject({ state: { mode: 'locked', type: 'create', rpName: 'Example' } });
  });
});

describe('one request at a time', () => {
  it('refuses a second modal request in the same tab until the first is done', async () => {
    const first = await begin(createReq());
    expect(await begin(createReq())).toEqual({
      action: 'error',
      name: 'NotAllowedError',
      message: 'Another passkey request is already open on this page.',
    });
    expect(await begin(createReq(), { ...PAGE, tab: { id: 8 } })).toMatchObject({ action: 'ui' });
    expect(await call(MessageType.PASSKEY_CANCEL, { id: first.id }, PAGE)).toEqual({ ok: true });
    expect(await begin(createReq())).toMatchObject({ action: 'ui' });
  });

  it('only lets the requesting tab cancel', async () => {
    const first = await begin(createReq());
    expect(await call(MessageType.PASSKEY_CANCEL, { id: first.id }, { ...PAGE, tab: { id: 8 } })).toEqual({ ok: false });
    expect(Object.keys(await pendingPasskeys())).toEqual([first.id]);
  });

  it('drops the requests of a document that was navigated away from', async () => {
    const first = await begin(createReq());
    await dropPendingForTab(7, { documentId: 'doc-1' });
    expect(Object.keys(await pendingPasskeys())).toEqual([first.id]);
    await dropPendingForTab(7, { documentId: 'doc-2' });
    expect(await pendingPasskeys()).toEqual({});
  });

  it('clamps the timeout to 30 s – 10 min', async () => {
    const now = Date.now();
    await begin(createReq({ timeout: 1000 }));
    const [p] = Object.values(await pendingPasskeys());
    expect(p.deadline - now).toBeGreaterThanOrEqual(30_000);
    expect(p.deadline - now).toBeLessThan(31_000);
  });
});

// ─── The dialog's sender ────────────────────────────────────────────────────

describe('dialog frames', () => {
  it('answers only our dialog page in the request tab, and only the frame that used the token first', async () => {
    const { uiToken } = await begin(createReq());
    expect(await uiState(uiToken!, { ...DIALOG, url: 'https://evil.example/passkey-dialog/index.html' })).toEqual({
      error: 'This passkey request is no longer open.',
    });
    expect(await uiState(uiToken!, FILL)).toMatchObject({ error: expect.any(String) });
    expect(await uiState(uiToken!, { ...DIALOG, tab: { id: 8 } })).toMatchObject({ error: expect.any(String) });
    expect(await uiState(uiToken!, { ...DIALOG, frameId: 0 })).toMatchObject({ error: expect.any(String) });
    expect(await uiState('wrong-token-0000000000')).toMatchObject({ error: expect.any(String) });

    expect(await uiState(uiToken!)).toMatchObject({ state: { mode: 'create' } });
    expect(await uiState(uiToken!, { ...DIALOG, frameId: 99 })).toMatchObject({ error: expect.any(String) });
    expect(await complete(uiToken!, { action: 'cancel' }, { ...DIALOG, frameId: 99 })).toMatchObject({ ok: false });
    expect(await uiState(uiToken!)).toMatchObject({ state: { mode: 'create' } });
  });

  it('carries out a decision once', async () => {
    const { uiToken, id } = await begin(createReq());
    await uiState(uiToken!);
    expect(await complete(uiToken!, { action: 'cancel' })).toEqual({ ok: true });
    expect(await complete(uiToken!, { action: 'cancel' })).toMatchObject({ ok: false });
    expect(pushes()).toEqual([
      {
        tabId: 7,
        id,
        outcome: { kind: 'error', name: 'NotAllowedError', message: 'The user cancelled the request.' },
        keepDialog: false,
        options: { frameId: 0, documentId: 'doc-1' },
      },
    ]);
  });

  it('hands the request to the browser on "use another device"', async () => {
    const { uiToken, id } = await begin(createReq());
    await uiState(uiToken!);
    expect(await complete(uiToken!, { action: 'native' })).toEqual({ ok: true });
    expect(pushes()[0]).toMatchObject({ id, outcome: { kind: 'native' } });
  });
});

// ─── Creating ───────────────────────────────────────────────────────────────

describe('creating a passkey', () => {
  it('offers matching login entries by user name and saves into a new entry before answering', async () => {
    entries.push(entry('e2', { url: 'https://other.com' }), entry('e3', { url: 'login.example.com/x' }));
    vault.e2 = { username: 'alice@example.com' };
    vault.e3 = { username: 'ALICE@example.com' };
    const { uiToken, id } = await begin(createReq({ extensions: { names: ['credProps', 'prf'], credProps: true } }));
    const { state } = (await uiState(uiToken!)) as { state: { mode: string; targets: Array<{ entryId: string }> } };
    expect(state.mode).toBe('create');
    expect(state.targets.map((t) => t.entryId)).toEqual(['e1', 'e3']);

    expect(await complete(uiToken!, { action: 'create', target: 'new' })).toEqual({ ok: true });
    expect(createEntryMock).toHaveBeenCalledTimes(1);
    const [, name, type, url, data] = createEntryMock.mock.calls[0] as [unknown, string, string, string, EntryData];
    expect([name, type, url]).toEqual(['Example', 'password', 'https://example.com']);
    expect(data.username).toBe('alice@example.com');
    expect(data.password).toBe('');
    expect(data.passkeys).toHaveLength(1);
    const record = data.passkeys![0];
    expect(record).toMatchObject({ rp_id: 'example.com', rp_name: 'Example', user_handle: USER_ID, user_display_name: 'Alice', sign_count: 0 });

    // Saved first, answered second.
    expect(createEntryMock.mock.invocationCallOrder[0]).toBeLessThan(tabsSendMessage.mock.invocationCallOrder[0]);
    const [push] = pushes();
    expect(push.id).toBe(id);
    const credential = push.outcome.credential as Record<string, unknown>;
    expect(credential).toMatchObject({
      type: 'create',
      id: record.credential_id,
      publicKeyAlgorithm: -7,
      transports: ['internal', 'hybrid'],
      clientExtensionResults: { credProps: { rk: true }, prf: { enabled: false } },
    });
    const clientData = JSON.parse(new TextDecoder().decode(base64urlDecode(credential.clientDataJSON as string)));
    expect(clientData).toEqual({ type: 'webauthn.create', challenge: CHALLENGE, origin: 'https://login.example.com', crossOrigin: false });
    expect(await pendingPasskeys()).toEqual({});
  });

  it('fails closed when the entry cannot be saved', async () => {
    createEntryMock.mockRejectedValue(new Error('server down'));
    const { uiToken, id } = await begin(createReq());
    await uiState(uiToken!);
    const reply = await complete(uiToken!, { action: 'create', target: 'new' });
    expect(reply).toEqual({ ok: false, error: 'Could not save the passkey: server down' });
    expect(pushes()).toEqual([
      expect.objectContaining({ id, outcome: { kind: 'error', name: 'NotAllowedError', message: 'The passkey could not be saved.' }, keepDialog: true }),
    ]);
  });

  it('replaces the passkey of the same account in an existing entry and keeps everything else', async () => {
    const sameAccount = await storedPasskey({ credential_id: b64u('old'), unknown_key: 'kept?' });
    const otherSite = await storedPasskey({ rp_id: 'other.com', credential_id: b64u('other site') });
    const otherUser = await storedPasskey({ user_handle: b64u('someone else'), credential_id: b64u('other user') });
    vault.e1.passkeys = [sameAccount, otherSite, otherUser];
    const { uiToken } = await begin(createReq());
    await uiState(uiToken!);
    expect(await complete(uiToken!, { action: 'create', target: 'e1' })).toEqual({ ok: true });

    const saved = vault.e1;
    expect(saved.notes).toBe('keep me');
    expect(saved.extra).toEqual({ from: 'newer app' });
    expect(saved.password).toBe('pw');
    const ids = saved.passkeys!.map((p) => p.credential_id);
    expect(ids).toHaveLength(3);
    expect(ids).not.toContain(b64u('old'));
    expect(ids.slice(0, 2)).toEqual([b64u('other site'), b64u('other user')]);
    expect(client.updateEntry).toHaveBeenCalledWith('e1', { encrypted_data: 'ed', data_nonce: 'dn', folder_id: 'f1' });
  });

  it('refuses entries that were not offered', async () => {
    entries.push(entry('e9', { url: 'https://unrelated.org' }));
    vault.e9 = { username: 'alice@example.com' };
    const { uiToken } = await begin(createReq());
    await uiState(uiToken!);
    expect(await complete(uiToken!, { action: 'create', target: 'e9' })).toMatchObject({ ok: false });
    expect(client.updateEntry).not.toHaveBeenCalled();
  });

  it('offers a new entry when the chosen one is read-only', async () => {
    client.updateEntry.mockRejectedValueOnce(Object.assign(new Error('forbidden'), { status: 403 }));
    const { uiToken } = await begin(createReq());
    await uiState(uiToken!);
    expect(await complete(uiToken!, { action: 'create', target: 'e1' })).toMatchObject({ ok: false, readOnly: true });
    expect(pushes()).toEqual([]);
    expect(await uiState(uiToken!)).toMatchObject({ state: { mode: 'create', readOnlyTargets: ['e1'] } });
    expect(await complete(uiToken!, { action: 'create', target: 'e1' })).toMatchObject({ ok: false });
    expect(await complete(uiToken!, { action: 'create', target: 'new' })).toEqual({ ok: true });
    expect(pushes()).toHaveLength(1);
  });

  it('does not create while locked', async () => {
    const { uiToken } = await begin(createReq());
    await uiState(uiToken!);
    await clearSession();
    expect(await complete(uiToken!, { action: 'create', target: 'new' })).toMatchObject({ ok: false, locked: true });
    expect(createEntryMock).not.toHaveBeenCalled();
  });
});

describe('excludeCredentials', () => {
  it('reports an existing passkey only after the user confirms', async () => {
    const existing = await storedPasskey();
    vault.e1.passkeys = [existing];
    const { uiToken, id } = await begin(createReq({ excludeCredentials: [{ type: 'public-key', id: existing.credential_id }] }));
    // No consent yet: no answer that would reveal the stored id.
    expect(await complete(uiToken!, { action: 'exists-ok' })).toMatchObject({ ok: false });
    expect(pushes()).toEqual([]);

    expect(await uiState(uiToken!)).toEqual({
      state: { mode: 'exists', type: 'create', host: 'login.example.com', rpId: 'example.com', rpName: 'Example' },
    });
    expect(await complete(uiToken!, { action: 'create', target: 'new' })).toMatchObject({ ok: false });
    expect(await complete(uiToken!, { action: 'exists-ok' })).toEqual({ ok: true });
    expect(pushes()).toEqual([expect.objectContaining({ id, outcome: expect.objectContaining({ kind: 'error', name: 'InvalidStateError' }) })]);
  });

  it('ignores excluded ids of other sites', async () => {
    const existing = await storedPasskey({ rp_id: 'other.com' });
    vault.e1.passkeys = [existing];
    const { uiToken } = await begin(createReq({ excludeCredentials: [{ type: 'public-key', id: existing.credential_id }] }));
    expect(await uiState(uiToken!)).toMatchObject({ state: { mode: 'create' } });
  });
});

// ─── Signing in ─────────────────────────────────────────────────────────────

describe('signing in', () => {
  async function registered(): Promise<{ record: PasskeyRecord; spki: Uint8Array }> {
    const created = await createPasskey({ rpId: 'example.com', userHandle: base64urlDecode(USER_ID), userName: 'alice@example.com' });
    vault.e1.passkeys = [created.record];
    return { record: created.record, spki: created.publicKeySpki };
  }

  it('lists the passkeys and signs with the chosen one', async () => {
    const { record, spki } = await registered();
    const { uiToken, id } = await begin(getReq({ extensions: { names: ['appid'], appid: 'https://example.com/appid' } }));
    expect(await uiState(uiToken!)).toEqual({
      state: {
        mode: 'get',
        type: 'get',
        host: 'login.example.com',
        rpId: 'example.com',
        accounts: [{ credentialId: record.credential_id, userName: 'alice@example.com', entryName: 'Entry e1' }],
      },
    });
    expect(await complete(uiToken!, { action: 'select', credentialId: record.credential_id })).toEqual({ ok: true });
    const [push] = pushes();
    expect(push.id).toBe(id);
    const c = push.outcome.credential as Record<string, string> & { clientExtensionResults: unknown };
    expect(c).toMatchObject({ type: 'get', id: record.credential_id, userHandle: USER_ID, clientExtensionResults: { appid: false } });
    const authData = base64urlDecode(c.authenticatorData);
    expect(authData[32]).toBe(0x1d);
    const clientDataJSON = base64urlDecode(c.clientDataJSON);
    const clientHash = new Uint8Array(await crypto.subtle.digest('SHA-256', clientDataJSON as BufferSource));
    expect(await verifies(spki, new Uint8Array([...authData, ...clientHash]), base64urlDecode(c.signature))).toBe(true);
    expect(await verifies(spki, new Uint8Array([...authData, ...clientHash.reverse()]), base64urlDecode(c.signature))).toBe(false);
    expect(JSON.parse(new TextDecoder().decode(clientDataJSON))).toEqual({
      type: 'webauthn.get',
      challenge: CHALLENGE,
      origin: 'https://login.example.com',
      crossOrigin: false,
    });
  });

  it('re-reads the entry and fails when the passkey is gone', async () => {
    const { record } = await registered();
    const { uiToken } = await begin(getReq());
    await uiState(uiToken!);
    vault.e1.passkeys = []; // deleted on another device, index not yet refreshed
    expect(await complete(uiToken!, { action: 'select', credentialId: record.credential_id })).toMatchObject({ ok: false });
    expect(pushes()[0]).toMatchObject({ outcome: { kind: 'error', name: 'NotAllowedError' }, keepDialog: true });
  });

  it('only signs with offered passkeys, narrowed by allowCredentials', async () => {
    const { record } = await registered();
    const other = await storedPasskey({ credential_id: b64u('second') });
    vault.e1.passkeys = [record, other];
    const { uiToken } = await begin(getReq({ allowCredentials: [{ type: 'public-key', id: other.credential_id }] }));
    const { state } = (await uiState(uiToken!)) as { state: { accounts: Array<{ credentialId: string }> } };
    expect(state.accounts.map((a) => a.credentialId)).toEqual([other.credential_id]);
    expect(await complete(uiToken!, { action: 'select', credentialId: record.credential_id })).toMatchObject({ ok: false });
    expect(pushes()).toEqual([]);
    expect(await complete(uiToken!, { action: 'select', credentialId: other.credential_id })).toEqual({ ok: true });
  });

  it('accepts ids stored in standard base64', async () => {
    const { record } = await registered();
    const std = btoa(String.fromCharCode(...base64urlDecode(record.credential_id)));
    vault.e1.passkeys = [{ ...record, credential_id: std }];
    const { uiToken } = await begin(getReq());
    await uiState(uiToken!);
    expect(await complete(uiToken!, { action: 'select', credentialId: record.credential_id })).toEqual({ ok: true });
  });
});

describe('conditional requests (passkeys in the fill box)', () => {
  it('registers without looking at the vault and offers passkeys to the top frame only', async () => {
    const { record } = await (async () => {
      const created = await createPasskey({ rpId: 'example.com', userHandle: base64urlDecode(USER_ID), userName: 'alice@example.com' });
      vault.e1.passkeys = [created.record];
      return created;
    })();
    const reply = await begin(getReq({}, 'conditional'));
    expect(reply).toMatchObject({ action: 'conditional' });
    expect(reply.uiToken).toBeUndefined();
    expect(client.listEntriesFull).not.toHaveBeenCalled();

    expect(await call(MessageType.PASSKEY_SUGGESTIONS, {}, { ...PAGE, frameId: 5 })).toEqual({ rows: [] });
    expect(await call(MessageType.PASSKEY_SUGGESTIONS, {}, { ...PAGE, tab: { id: 8 } })).toEqual({ rows: [] });
    const s = (await call(MessageType.PASSKEY_SUGGESTIONS, {}, PAGE)) as { token: string; rows: unknown[] };
    expect(s.rows).toEqual([{ credentialId: record.credential_id, userName: 'alice@example.com', entryName: 'Entry e1' }]);

    // The fill box may only pick a passkey.
    expect(await complete(s.token, { action: 'cancel' }, FILL)).toMatchObject({ ok: false });
    expect(await complete(s.token, { action: 'select', credentialId: record.credential_id }, DIALOG)).toMatchObject({ ok: false });
    expect(await complete(s.token, { action: 'select', credentialId: record.credential_id }, FILL)).toEqual({ ok: true });
    expect(pushes()[0]).toMatchObject({ id: reply.id, outcome: { kind: 'credential' } });
  });

  it('does not count against the one-modal-request rule and replaces an older conditional request', async () => {
    const a = await begin(getReq({}, 'conditional'));
    const b = await begin(getReq({}, 'conditional'));
    expect(Object.keys(await pendingPasskeys())).toEqual([b.id]);
    expect(a.id).not.toBe(b.id);
    expect(await begin(createReq())).toMatchObject({ action: 'ui' });
  });

  it('reports a locked vault', async () => {
    await begin(getReq({}, 'conditional'));
    await clearSession();
    expect(await call(MessageType.PASSKEY_SUGGESTIONS, {}, PAGE)).toEqual({ rows: [], locked: true });
  });
});

describe('PASSKEY_DELETE', () => {
  it('removes one passkey and keeps the rest of the entry', async () => {
    const a = await storedPasskey({ credential_id: b64u('a') });
    const b = await storedPasskey({ credential_id: b64u('b') });
    vault.e1.passkeys = [a, b];
    expect(await call(MessageType.PASSKEY_DELETE, { entryId: 'e1', credentialId: a.credential_id }, {})).toEqual({ ok: true });
    expect(vault.e1.passkeys!.map((p) => p.credential_id)).toEqual([b.credential_id]);
    expect(vault.e1.notes).toBe('keep me');
    expect(await call(MessageType.PASSKEY_DELETE, { entryId: 'e1', credentialId: a.credential_id }, {})).toMatchObject({ ok: false });
    expect(await call(MessageType.PASSKEY_DELETE, { entryId: 'e1', credentialId: b.credential_id }, {})).toEqual({ ok: true });
    expect('passkeys' in vault.e1).toBe(false);
  });
});

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

// Lock / login lifecycle across service-worker restarts, with the real
// session-store, account-store, pin-store and vault-client on top of fake
// storage areas. A "service-worker restart" is simulated by resetting the module
// registry: in-memory state is gone, the storage areas survive (like Chrome's
// storage.session / storage.local do).

import { describe, it, expect, vi, beforeEach } from 'vitest';

type Area = Record<string, unknown>;
const stores = vi.hoisted(() => ({ local: {} as Area, session: {} as Area, sync: {} as Area }));
const alarms = vi.hoisted(() => ({ create: vi.fn(), clear: vi.fn() }));
const api = vi.hoisted(() => ({
  refresh: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  listEntries: vi.fn(),
  listFolders: vi.fn(),
}));
const crypto = vi.hoisted(() => ({ unlock: vi.fn(), unlockWithPin: vi.fn() }));

vi.mock('webextension-polyfill', () => {
  // Values are JSON round-tripped like the real storage API, so a non-JSON-safe
  // value (e.g. a raw Uint8Array) would show up as broken data in these tests.
  const area = (name: 'local' | 'session' | 'sync') => ({
    get: vi.fn(async (keys?: string | string[] | null) => {
      const s = stores[name];
      const arr = keys == null ? Object.keys(s) : Array.isArray(keys) ? keys : [keys];
      const out: Area = {};
      for (const k of arr) if (k in s) out[k] = JSON.parse(JSON.stringify(s[k]));
      return out;
    }),
    set: vi.fn(async (obj: Area) => {
      for (const [k, v] of Object.entries(obj)) {
        if (v !== undefined) stores[name][k] = JSON.parse(JSON.stringify(v));
      }
    }),
    remove: vi.fn(async (keys: string | string[]) => {
      for (const k of Array.isArray(keys) ? keys : [keys]) delete stores[name][k];
    }),
    clear: vi.fn(async () => {
      for (const k of Object.keys(stores[name])) delete stores[name][k];
    }),
  });
  return {
    default: {
      storage: { local: area('local'), session: area('session'), sync: area('sync') },
      alarms,
      action: { setBadgeText: vi.fn(), setBadgeBackgroundColor: vi.fn() },
    },
  };
});

vi.mock('@passbubble/shared-ts', () => {
  // Records the access token each API call was made with.
  class FakeClient {
    token = '';
    constructor(public baseUrl: string) {}
    setTokens(accessToken: string) {
      this.token = accessToken;
    }
    refresh(refreshToken: string) {
      return api.refresh(refreshToken);
    }
    login(email: string, password: string) {
      return api.login(email, password);
    }
    logout(refreshToken: string) {
      return api.logout(refreshToken, this.token);
    }
    listEntries() {
      return api.listEntries(this.token);
    }
    listFolders() {
      return api.listFolders(this.token);
    }
  }
  return {
    PassbubbleClient: FakeClient,
    unlock: crypto.unlock,
    unlockWithPin: crypto.unlockWithPin,
    decryptEntry: vi.fn(),
    encryptEntry: vi.fn(),
    createEntry: vi.fn(),
    clampPwIntervalDays: (d: number) => Math.min(60, Math.max(1, Math.round(d || 14))),
    pwIntervalElapsed: (last: number, days: number, now: number = Date.now()) =>
      now - last >= Math.min(60, Math.max(1, days || 14)) * 86400000,
    PIN_KDF_TIME: 3,
    PIN_KDF_MEMORY: 65536,
    DEFAULT_PIN_MAX_TRIES: 5,
    DEFAULT_PIN_PW_INTERVAL_DAYS: 14,
  };
});

import { STORAGE_KEYS, TOKEN_REFRESH_ALARM } from '../../shared/constants.js';

const SERVER = 'https://passbubble.example.com';
const unauthorized = (msg = 'session expired or revoked') => Object.assign(new Error(msg), { status: 401 });

function seedAccount(refreshToken = 'rt-1') {
  Object.assign(stores.local, {
    [STORAGE_KEYS.REFRESH_TOKEN]: refreshToken,
    [STORAGE_KEYS.ENC_PRIV_X25519]: btoa('encx'),
    [STORAGE_KEYS.ENC_PRIV_MLKEM]: btoa('encm'),
    [STORAGE_KEYS.KDF_SALT]: btoa('kdfsalt'),
    [STORAGE_KEYS.KDF_TIME]: 3,
    [STORAGE_KEYS.KDF_MEMORY]: 65536,
    [STORAGE_KEYS.USER_ID]: 'u1',
    [STORAGE_KEYS.USER_EMAIL]: 'alice@example.com',
    [STORAGE_KEYS.USER_NAME]: 'Alice',
    [STORAGE_KEYS.ROLE]: 'user',
    [STORAGE_KEYS.PUB_X25519]: btoa('p'.repeat(32)),
    [STORAGE_KEYS.PUB_MLKEM]: btoa('pubm'),
  });
}

function seedPin(extra: Area = {}) {
  Object.assign(stores.local, {
    [STORAGE_KEYS.PIN_ENABLED]: true,
    [STORAGE_KEYS.PIN_SALT]: btoa('saltsaltsaltsalt'),
    [STORAGE_KEYS.PIN_WRAPPED]: btoa('wrappedwrappedwrapped'),
    [STORAGE_KEYS.PIN_KDF_TIME]: 3,
    [STORAGE_KEYS.PIN_KDF_MEMORY]: 65536,
    [STORAGE_KEYS.PIN_MAX_TRIES]: 5,
    [STORAGE_KEYS.PIN_FAIL_COUNT]: 0,
    [STORAGE_KEYS.PIN_PW_INTERVAL_DAYS]: 14,
    [STORAGE_KEYS.PIN_LAST_MASTER_UNLOCK]: Date.now(),
    ...extra,
  });
}

// Serves rotating tokens: rt-2/at-2, rt-3/at-3, …
function rotatingRefresh() {
  let n = 1;
  api.refresh.mockImplementation(async () => {
    n += 1;
    return { access_token: `at-${n}`, refresh_token: `rt-${n}`, expires_in: 900 };
  });
}

/** Fresh module instances = a freshly started service worker. */
async function startWorker() {
  vi.resetModules();
  const mh = await import('../message-handler.js');
  const ss = await import('../session-store.js');
  const vc = await import('../vault-client.js');
  const handlers = mh.buildHandlers();
  const send = (type: string, payload: Record<string, unknown> = {}) =>
    handlers[type](payload, {}) as Promise<Record<string, unknown>>;
  return { mh, ss, vc, send };
}

async function unlockedWorker() {
  seedAccount();
  const w = await startWorker();
  const res = await w.send('UNLOCK', { masterPassword: 'master' });
  expect(res).toEqual({ ok: true });
  return w;
}

beforeEach(() => {
  for (const area of Object.values(stores)) for (const k of Object.keys(area)) delete area[k];
  stores.sync[STORAGE_KEYS.SERVER_URL] = SERVER;
  stores.sync[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 15;
  api.refresh.mockReset();
  api.login.mockReset();
  api.logout.mockReset().mockResolvedValue(undefined);
  api.listEntries.mockReset().mockResolvedValue([]);
  api.listFolders.mockReset().mockResolvedValue([]);
  crypto.unlock.mockReset().mockResolvedValue({ privX25519: new Uint8Array(32).fill(7), privMLKEM: new Uint8Array(64).fill(9) });
  crypto.unlockWithPin.mockReset().mockResolvedValue({ privX25519: new Uint8Array(32), privMLKEM: new Uint8Array(64) });
  alarms.create.mockClear();
  alarms.clear.mockClear();
  rotatingRefresh();
});

describe('login state', () => {
  it('LOGIN keeps the refresh token + key material in storage.local, not storage.session', async () => {
    api.login.mockResolvedValue({
      access_token: 'at', refresh_token: 'rt', expires_in: 900, user_id: 'u1', email: 'a@b.c', name: 'A',
      role: 'user', enc_priv_x25519: 'e1', enc_priv_mlkem768: 'e2', pub_x25519: 'p1', pub_mlkem768: 'p2',
      kdf_salt: 's', kdf_time: 3, kdf_memory: 65536,
    });
    const { send } = await startWorker();
    await send('LOGIN', { email: 'a@b.c', password: 'pw' });
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBe('rt');
    expect(stores.local[STORAGE_KEYS.ENC_PRIV_X25519]).toBe('e1');
    expect(stores.session[STORAGE_KEYS.REFRESH_TOKEN]).toBeUndefined();

    // A browser restart wipes storage.session; the device stays logged in.
    for (const k of Object.keys(stores.session)) delete stores.session[k];
    const after = await startWorker();
    const s = await after.send('GET_SESSION');
    expect(s.isLoggedIn).toBe(true);
    expect(s.isUnlocked).toBe(false);
  });
});

describe('unlock', () => {
  it('persists the rotated refresh token before loading entries', async () => {
    seedAccount('rt-1');
    let tokenDuringList: unknown;
    api.listEntries.mockImplementation(async () => {
      tokenDuringList = stores.local[STORAGE_KEYS.REFRESH_TOKEN];
      return [];
    });
    const { send } = await startWorker();
    expect(await send('UNLOCK', { masterPassword: 'master' })).toEqual({ ok: true });
    expect(api.refresh).toHaveBeenCalledWith('rt-1');
    expect(tokenDuringList).toBe('rt-2');
    // The unlocked session is in storage.session — without the refresh token.
    const stored = stores.session[STORAGE_KEYS.UNLOCKED_SESSION] as Record<string, unknown>;
    expect(stored.accessToken).toBe('at-2');
    expect(JSON.stringify(stored)).not.toContain('rt-2');
  });

  it('still unlocks (with the new token persisted) when the entry pre-load fails', async () => {
    seedAccount('rt-1');
    api.listEntries.mockRejectedValue(new TypeError('Failed to fetch'));
    const { send, ss } = await startWorker();
    expect(await send('UNLOCK', { masterPassword: 'master' })).toEqual({ ok: true });
    expect(ss.getSession()).not.toBeNull();
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBe('rt-2');
  });

  it('reports "Wrong master password" only for a decryption failure', async () => {
    seedAccount();
    crypto.unlock.mockRejectedValueOnce(new DOMException('tag mismatch', 'OperationError'));
    const { send } = await startWorker();
    const res = await send('UNLOCK', { masterPassword: 'nope' });
    expect(res).toMatchObject({ ok: false, wrongPassword: true, error: 'Wrong master password' });
    expect(api.refresh).not.toHaveBeenCalled();
  });

  it('reports server / network problems distinctly and keeps the login', async () => {
    seedAccount('rt-1');
    api.refresh.mockRejectedValueOnce(new TypeError('Failed to fetch'));
    const { send } = await startWorker();
    let res = await send('UNLOCK', { masterPassword: 'master' });
    expect(res.ok).toBe(false);
    expect(res.error).toMatch(/could not reach the server/i);
    expect(res.error).not.toMatch(/wrong master password/i);

    api.refresh.mockRejectedValueOnce(Object.assign(new Error('failed to check session'), { status: 500 }));
    res = await send('UNLOCK', { masterPassword: 'master' });
    expect(res.error).toMatch(/server reported an error \(500\)/i);
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBe('rt-1');
  });

  it('reports a missing login state as signed out', async () => {
    const { send } = await startWorker();
    const res = await send('UNLOCK', { masterPassword: 'master' });
    expect(res).toMatchObject({ ok: false, loggedOut: true });
  });
});

describe('service-worker restart', () => {
  it('restores the unlocked session from storage.session', async () => {
    await unlockedWorker();

    const fresh = await startWorker();
    expect(fresh.ss.getSession()).toBeNull(); // memory is gone…
    await fresh.mh.ensureSessionRestored(); // …the worker restores it before handling messages
    const s = await fresh.send('GET_SESSION');
    expect(s.isUnlocked).toBe(true);
    expect(fresh.ss.getSession()?.privX25519).toEqual(new Uint8Array(32).fill(7));

    api.listEntries.mockResolvedValue([{ id: 'e1' }]);
    expect(await fresh.send('SEARCH_ENTRIES', { query: '' })).toEqual([{ id: 'e1' }]);
    expect(api.listEntries).toHaveBeenLastCalledWith('at-2');
  });

  it('locks instead of restoring when the vault was idle past the auto-lock timeout', async () => {
    stores.sync[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 5;
    await unlockedWorker();
    stores.session[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 6 * 60_000;

    const fresh = await startWorker();
    await fresh.mh.ensureSessionRestored();
    expect(fresh.ss.getSession()).toBeNull();
    expect(stores.session[STORAGE_KEYS.UNLOCKED_SESSION]).toBeUndefined();
    const s = await fresh.send('GET_SESSION');
    expect(s).toMatchObject({ isLoggedIn: true, isUnlocked: false });
  });

  it('the auto-lock alarm locks a restored-but-idle session', async () => {
    stores.sync[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 5;
    await unlockedWorker();

    const fresh = await startWorker();
    await fresh.mh.ensureSessionRestored();
    expect(fresh.ss.getSession()).not.toBeNull();
    stores.session[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 6 * 60_000;
    expect(await fresh.mh.maybeAutoLock()).toBe(true);
    expect(fresh.ss.getSession()).toBeNull();
    expect(stores.session[STORAGE_KEYS.UNLOCKED_SESSION]).toBeUndefined();
  });

  it('with auto-lock "never" the session is restored regardless of idle time', async () => {
    stores.sync[STORAGE_KEYS.AUTO_LOCK_MINUTES] = 0;
    await unlockedWorker();
    stores.session[STORAGE_KEYS.LAST_ACTIVITY] = Date.now() - 24 * 60 * 60_000;

    const fresh = await startWorker();
    await fresh.mh.ensureSessionRestored();
    expect(fresh.ss.getSession()).not.toBeNull();
  });
});

describe('token refresh', () => {
  it('refreshes once and retries once when an API call gets a 401', async () => {
    const { send } = await unlockedWorker();
    api.listEntries.mockRejectedValueOnce(unauthorized('invalid or expired token')).mockResolvedValueOnce([{ id: 'e1' }]);
    api.refresh.mockClear();

    expect(await send('SEARCH_ENTRIES', { query: '' })).toEqual([{ id: 'e1' }]);
    expect(api.refresh).toHaveBeenCalledTimes(1);
    expect(api.refresh).toHaveBeenCalledWith('rt-2');
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBe('rt-3');
    expect(api.listEntries).toHaveBeenLastCalledWith('at-3');
    expect((stores.session[STORAGE_KEYS.UNLOCKED_SESSION] as Record<string, unknown>).accessToken).toBe('at-3');
  });

  it('concurrent 401s share a single refresh', async () => {
    const { send } = await unlockedWorker();
    api.listEntries.mockRejectedValueOnce(unauthorized()).mockResolvedValue([]);
    api.listFolders.mockRejectedValueOnce(unauthorized()).mockResolvedValue([]);
    api.refresh.mockClear();

    await Promise.all([send('SEARCH_ENTRIES', { query: '' }), send('LIST_FOLDERS')]);
    expect(api.refresh).toHaveBeenCalledTimes(1);
  });

  it('refreshes an expired access token before the request', async () => {
    const { send, ss } = await unlockedWorker();
    ss.getSession()!.accessTokenExpiresAt = Date.now() - 1000;
    api.refresh.mockClear();

    await send('SEARCH_ENTRIES', { query: '' });
    expect(api.refresh).toHaveBeenCalledTimes(1);
    expect(api.listEntries).toHaveBeenLastCalledWith('at-3');
  });

  it('a rejected refresh token signs the device out', async () => {
    seedPin();
    const { send } = await unlockedWorker();
    api.listEntries.mockRejectedValue(unauthorized('invalid or expired token'));
    api.refresh.mockRejectedValue(unauthorized());

    expect(await send('SEARCH_ENTRIES', { query: '' })).toEqual({ locked: true, loggedOut: true });
    expect(await send('GET_SESSION')).toMatchObject({ isLoggedIn: false, isUnlocked: false, pinEnabled: false });
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBeUndefined();
    expect(stores.local[STORAGE_KEYS.PIN_WRAPPED]).toBeUndefined();
    expect(stores.session[STORAGE_KEYS.UNLOCKED_SESSION]).toBeUndefined();
  });

  it('a transient refresh failure in the alarm keeps the vault unlocked and retries soon', async () => {
    const { send, vc, ss } = await unlockedWorker();
    alarms.create.mockClear();
    api.refresh.mockRejectedValueOnce(new TypeError('Failed to fetch'));

    await vc.handleTokenRefreshAlarm();
    expect(ss.getSession()).not.toBeNull();
    expect(alarms.create).toHaveBeenCalledWith(TOKEN_REFRESH_ALARM, { delayInMinutes: 1 });
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBe('rt-2');
    expect(await send('GET_SESSION')).toMatchObject({ isLoggedIn: true, isUnlocked: true });

    // 429 / 5xx behave the same.
    api.refresh.mockRejectedValueOnce(Object.assign(new Error('rate limited'), { status: 429 }));
    await vc.handleTokenRefreshAlarm();
    expect(ss.getSession()).not.toBeNull();

    // The retry succeeds and rotates the token.
    await vc.handleTokenRefreshAlarm();
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBe('rt-3');
  });
});

describe('migration from the old storage layout', () => {
  const legacyBootstrap = (refreshToken: string) => ({
    refresh_token: refreshToken,
    enc_priv_x25519: btoa('encx'),
    enc_priv_mlkem: btoa('encm'),
    kdf_salt: btoa('kdfsalt'),
    kdf_time: 3,
    kdf_memory: 65536,
    user_id: 'u1',
    user_email: 'alice@example.com',
    user_name: 'Alice',
    role: 'user',
    pub_x25519: btoa('p'.repeat(32)),
    pub_mlkem: btoa('pubm'),
  });

  it('moves a PIN bootstrap into the login state and keeps the PIN usable', async () => {
    seedPin({ [STORAGE_KEYS.PIN_BOOTSTRAP]: legacyBootstrap('pin-rt') });
    const { send } = await startWorker();

    expect(await send('GET_SESSION')).toMatchObject({ isLoggedIn: true, pinEnabled: true, pinAvailable: true });
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBe('pin-rt');
    expect(stores.local[STORAGE_KEYS.PIN_BOOTSTRAP]).toBeUndefined();

    expect(await send('UNLOCK_WITH_PIN', { pin: '123456' })).toEqual({ ok: true });
    expect(api.refresh).toHaveBeenCalledWith('pin-rt');
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBe('rt-2');
  });

  it('prefers the (fresher) storage.session bootstrap and removes both old copies', async () => {
    const b = legacyBootstrap('sess-rt');
    Object.assign(stores.session, {
      [STORAGE_KEYS.REFRESH_TOKEN]: b.refresh_token,
      [STORAGE_KEYS.ENC_PRIV_X25519]: b.enc_priv_x25519,
      [STORAGE_KEYS.ENC_PRIV_MLKEM]: b.enc_priv_mlkem,
      [STORAGE_KEYS.KDF_SALT]: b.kdf_salt,
      [STORAGE_KEYS.KDF_TIME]: b.kdf_time,
      [STORAGE_KEYS.KDF_MEMORY]: b.kdf_memory,
      [STORAGE_KEYS.USER_ID]: b.user_id,
      [STORAGE_KEYS.USER_EMAIL]: b.user_email,
      [STORAGE_KEYS.USER_NAME]: b.user_name,
      [STORAGE_KEYS.ROLE]: b.role,
      [STORAGE_KEYS.PUB_X25519]: b.pub_x25519,
      [STORAGE_KEYS.PUB_MLKEM]: b.pub_mlkem,
    });
    seedPin({ [STORAGE_KEYS.PIN_BOOTSTRAP]: legacyBootstrap('stale-pin-rt') });
    const { send } = await startWorker();

    expect(await send('UNLOCK', { masterPassword: 'master' })).toEqual({ ok: true });
    expect(api.refresh).toHaveBeenCalledWith('sess-rt');
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBe('rt-2');
    expect(stores.local[STORAGE_KEYS.PIN_BOOTSTRAP]).toBeUndefined();
    expect(stores.session[STORAGE_KEYS.REFRESH_TOKEN]).toBeUndefined();
    expect(stores.local[STORAGE_KEYS.PIN_WRAPPED]).toBeTruthy(); // PIN itself is kept
  });
});

describe('lock and logout', () => {
  it('LOCK drops the stored unlocked session but keeps the login', async () => {
    const { send } = await unlockedWorker();
    await send('LOCK');
    expect(stores.session[STORAGE_KEYS.UNLOCKED_SESSION]).toBeUndefined();

    const fresh = await startWorker();
    await fresh.mh.ensureSessionRestored();
    expect(await fresh.send('GET_SESSION')).toMatchObject({ isLoggedIn: true, isUnlocked: false });
  });

  it('LOGOUT revokes the server session and wipes the login state', async () => {
    seedPin();
    const { send } = await unlockedWorker();
    await send('LOGOUT');
    expect(api.logout).toHaveBeenCalledWith('rt-2', 'at-2');
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBeUndefined();
    expect(stores.local[STORAGE_KEYS.ENC_PRIV_X25519]).toBeUndefined();
    expect(stores.local[STORAGE_KEYS.PIN_WRAPPED]).toBeUndefined();
    expect(stores.session[STORAGE_KEYS.UNLOCKED_SESSION]).toBeUndefined();
    expect(await send('GET_SESSION')).toMatchObject({ isLoggedIn: false, isUnlocked: false });
  });

  it('LOGOUT while locked still revokes (via a one-off refresh) and signs out offline too', async () => {
    seedAccount('rt-1');
    const { send } = await startWorker();
    await send('LOGOUT');
    expect(api.logout).toHaveBeenCalledWith('rt-2', 'at-2');

    seedAccount('rt-1');
    api.refresh.mockRejectedValueOnce(new TypeError('Failed to fetch'));
    await send('LOGOUT');
    expect(stores.local[STORAGE_KEYS.REFRESH_TOKEN]).toBeUndefined();
  });
});

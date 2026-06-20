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

import { describe, it, expect, vi, beforeEach } from 'vitest';

// Must be hoisted before importing the module under test
vi.mock('webextension-polyfill', () => ({
  default: {
    storage: {
      sync: { get: vi.fn(), set: vi.fn() },
      session: { get: vi.fn(), set: vi.fn() },
      local: { get: vi.fn(), set: vi.fn(), remove: vi.fn() },
    },
    alarms: { create: vi.fn(), clear: vi.fn() },
  },
}));

vi.mock('../session-store.js', () => ({
  getSession: vi.fn(),
  setSession: vi.fn(),
  clearSession: vi.fn(),
  getEntriesCache: vi.fn(),
  setEntriesCache: vi.fn(),
}));

vi.mock('@passbubble/shared-ts', () => ({
  PassbubbleClient: vi.fn().mockImplementation(() => ({
    setTokens: vi.fn(),
    generate: vi.fn().mockResolvedValue({
      passwords: [{ password: 'P@ssw0rd!', strength: 85 }],
    }),
    listEntries: vi.fn().mockResolvedValue([]),
    searchEntries: vi.fn().mockResolvedValue([]),
    getEntry: vi.fn().mockResolvedValue({ id: '1', name: 'Test', type: 'password', owner_id: 'u1', created_at: '', updated_at: '' }),
    refresh: vi.fn().mockResolvedValue({ access_token: 'at', refresh_token: 'rt', expires_in: 900 }),
  })),
  unlock: vi.fn(),
  decryptEntry: vi.fn().mockResolvedValue({ username: 'alice', password: 'secret' }),
  encryptEntry: vi.fn(),
  createEntry: vi.fn(),
  // PIN crypto: control-flow helpers use real logic; the heavy crypto is mocked
  // (its round-trip is covered by shared-ts pin.test.ts).
  deriveKey: vi.fn().mockResolvedValue(new Uint8Array(32)),
  aesGcmDecrypt: vi.fn().mockResolvedValue(new Uint8Array(32)),
  wrapMasterKeyWithPin: vi.fn().mockResolvedValue(new Uint8Array(48)),
  unlockWithPin: vi.fn(),
  generatePinSalt: vi.fn(() => new Uint8Array(16)),
  clampPwIntervalDays: (d: number) => Math.min(60, Math.max(1, Math.round(d || 14))),
  pwIntervalElapsed: (last: number, days: number, now: number = Date.now()) =>
    now - last >= Math.min(60, Math.max(1, days || 14)) * 86400000,
  PIN_KDF_TIME: 3,
  PIN_KDF_MEMORY: 65536,
  DEFAULT_PIN_MAX_TRIES: 5,
  DEFAULT_PIN_PW_INTERVAL_DAYS: 14,
}));

import browser from 'webextension-polyfill';
import { unlockWithPin, PassbubbleClient } from '@passbubble/shared-ts';
import { getSession, clearSession } from '../session-store.js';
import { buildHandlers } from '../message-handler.js';
import { savePinRefreshToken } from '../pin-store.js';
import { STORAGE_KEYS } from '../../shared/constants.js';

const mockUnlockWithPin = unlockWithPin as ReturnType<typeof vi.fn>;
const mockClient = PassbubbleClient as unknown as ReturnType<typeof vi.fn>;

// A complete PIN record as it would be read from storage.local. lastMasterUnlock
// defaults to "now" so the interval is not elapsed.
function pinLocal(overrides: Record<string, unknown> = {}) {
  return {
    [STORAGE_KEYS.PIN_ENABLED]: true,
    [STORAGE_KEYS.PIN_SALT]: btoa('saltsaltsaltsalt'),
    [STORAGE_KEYS.PIN_WRAPPED]: btoa('wrappedwrappedwrapped'),
    [STORAGE_KEYS.PIN_KDF_TIME]: 3,
    [STORAGE_KEYS.PIN_KDF_MEMORY]: 65536,
    [STORAGE_KEYS.PIN_MAX_TRIES]: 5,
    [STORAGE_KEYS.PIN_FAIL_COUNT]: 0,
    [STORAGE_KEYS.PIN_PW_INTERVAL_DAYS]: 14,
    [STORAGE_KEYS.PIN_LAST_MASTER_UNLOCK]: Date.now(),
    [STORAGE_KEYS.PIN_BOOTSTRAP]: {
      refresh_token: 'rt',
      enc_priv_x25519: btoa('encx'),
      enc_priv_mlkem: btoa('encm'),
      kdf_salt: btoa('kdfsalt'),
      kdf_time: 3,
      kdf_memory: 65536,
      user_id: 'u1',
      user_email: 'alice@example.com',
      user_name: 'Alice',
      role: 'user',
      pub_x25519: btoa('pubx'),
      pub_mlkem: btoa('pubm'),
    },
    ...overrides,
  };
}

const mockBrowser = browser as unknown as {
  storage: {
    sync: { get: ReturnType<typeof vi.fn>; set: ReturnType<typeof vi.fn> };
    session: { get: ReturnType<typeof vi.fn>; set: ReturnType<typeof vi.fn> };
    local: { get: ReturnType<typeof vi.fn>; set: ReturnType<typeof vi.fn>; remove: ReturnType<typeof vi.fn> };
  };
  alarms: { create: ReturnType<typeof vi.fn>; clear: ReturnType<typeof vi.fn> };
};

const mockGetSession = getSession as ReturnType<typeof vi.fn>;
const mockClearSession = clearSession as ReturnType<typeof vi.fn>;

function makeSession() {
  return {
    privX25519: new Uint8Array(32),
    privMLKEM: new Uint8Array(64),
    pubX25519: new Uint8Array(32),
    pubMLKEM: new Uint8Array(32),
    userId: 'u1',
    userEmail: 'alice@example.com',
    userName: 'Alice',
    role: 'user',
    accessToken: 'access-token',
    refreshToken: 'refresh-token',
    accessTokenExpiresAt: Date.now() + 900_000,
    serverUrl: 'https://passbubble.example.com',
    kdfSalt: new Uint8Array(32),
    kdfTime: 3,
    kdfMemory: 65536,
    encPrivX25519: new Uint8Array(44),
    encPrivMLKEM: new Uint8Array(44),
  };
}

describe('buildHandlers', () => {
  let handlers: ReturnType<typeof buildHandlers>;

  beforeEach(() => {
    mockBrowser.storage.sync.get.mockResolvedValue({ [STORAGE_KEYS.SERVER_URL]: 'https://passbubble.example.com' });
    mockBrowser.storage.session.get.mockResolvedValue({});
    mockBrowser.storage.local.get.mockResolvedValue({});
    mockBrowser.storage.local.set.mockResolvedValue(undefined);
    mockBrowser.storage.local.remove.mockResolvedValue(undefined);
    mockBrowser.alarms.clear.mockResolvedValue(undefined);
    mockGetSession.mockReturnValue(null);
    handlers = buildHandlers();
  });

  describe('LOCK', () => {
    it('clears in-memory session', async () => {
      await handlers['LOCK']({});
      expect(mockClearSession).toHaveBeenCalledOnce();
    });

    it('clears the token-refresh alarm', async () => {
      await handlers['LOCK']({});
      expect(mockBrowser.alarms.clear).toHaveBeenCalledWith('token-refresh');
    });

    it('returns { ok: true }', async () => {
      const result = await handlers['LOCK']({});
      expect(result).toEqual({ ok: true });
    });
  });

  describe('GET_SESSION when session is null', () => {
    it('returns isUnlocked: false', async () => {
      const result = await handlers['GET_SESSION']({}) as Record<string, unknown>;
      expect(result.isUnlocked).toBe(false);
    });

    it('returns isLoggedIn: false when no refresh token stored', async () => {
      const result = await handlers['GET_SESSION']({}) as Record<string, unknown>;
      expect(result.isLoggedIn).toBe(false);
    });

    it('returns isLoggedIn: true when refresh token is in session storage', async () => {
      mockBrowser.storage.session.get.mockResolvedValue({
        [STORAGE_KEYS.REFRESH_TOKEN]: 'stored-token',
      });
      const result = await handlers['GET_SESSION']({}) as Record<string, unknown>;
      expect(result.isLoggedIn).toBe(true);
    });
  });

  describe('FILL_ENTRY when session is null', () => {
    it('returns { locked: true } without exposing data', async () => {
      const result = await handlers['FILL_ENTRY']({ entryId: 'abc' });
      expect(result).toEqual({ locked: true });
    });
  });

  describe('GENERATE when session is active', () => {
    it('proxies the request to the API client and returns the response', async () => {
      mockGetSession.mockReturnValue(makeSession());
      const result = await handlers['GENERATE']({ length: 20, include_symbols: true, count: 1 }) as Record<string, unknown>;
      expect(result).toHaveProperty('passwords');
      expect((result.passwords as unknown[]).length).toBeGreaterThan(0);
    });

    it('returns { locked: true } when session is null', async () => {
      mockGetSession.mockReturnValue(null);
      const result = await handlers['GENERATE']({ length: 20 });
      expect(result).toEqual({ locked: true });
    });
  });

  describe('GET_MATCHES_FOR_URL when session is null', () => {
    it('returns { locked: true }', async () => {
      const result = await handlers['GET_MATCHES_FOR_URL']({ url: 'https://example.com' });
      expect(result).toEqual({ locked: true });
    });
  });

  describe('SEARCH_ENTRIES when session is null', () => {
    it('returns { locked: true }', async () => {
      const result = await handlers['SEARCH_ENTRIES']({ query: 'test' });
      expect(result).toEqual({ locked: true });
    });
  });

  describe('PIN quick-unlock', () => {
    it('GET_PIN_STATUS reports disabled when no PIN is stored', async () => {
      const result = (await handlers['GET_PIN_STATUS']({})) as Record<string, unknown>;
      expect(result).toEqual({ enabled: false });
    });

    it('GET_PIN_STATUS reports enabled + not expired for a fresh PIN', async () => {
      mockBrowser.storage.local.get.mockResolvedValue(pinLocal());
      const result = (await handlers['GET_PIN_STATUS']({})) as Record<string, unknown>;
      expect(result.enabled).toBe(true);
      expect(result.expired).toBe(false);
      expect(result.triesRemaining).toBe(5);
    });

    it('GET_SESSION reports logged-in via the PIN bootstrap after a restart', async () => {
      // storage.session is empty (browser restart) but a PIN bootstrap exists.
      mockBrowser.storage.local.get.mockResolvedValue(pinLocal());
      const result = (await handlers['GET_SESSION']({})) as Record<string, unknown>;
      expect(result.isLoggedIn).toBe(true);
      expect(result.pinEnabled).toBe(true);
      expect(result.pinAvailable).toBe(true);
    });

    it('DISABLE_PIN wipes the stored PIN state', async () => {
      const result = await handlers['DISABLE_PIN']({});
      expect(result).toEqual({ ok: true });
      expect(mockBrowser.storage.local.remove).toHaveBeenCalled();
    });

    it('UNLOCK_WITH_PIN rejects once the interval has elapsed (no attempt consumed)', async () => {
      mockBrowser.storage.local.get.mockResolvedValue(
        pinLocal({ [STORAGE_KEYS.PIN_LAST_MASTER_UNLOCK]: Date.now() - 20 * 86400000 }),
      );
      const result = (await handlers['UNLOCK_WITH_PIN']({ pin: '123456' })) as Record<string, unknown>;
      expect(result).toEqual({ ok: false, expired: true });
      // Did not consume an attempt.
      expect(mockBrowser.storage.local.set).not.toHaveBeenCalled();
    });

    it('UNLOCK_WITH_PIN returns wrongPin + decrements remaining tries', async () => {
      mockBrowser.storage.local.get.mockResolvedValue(pinLocal());
      mockUnlockWithPin.mockRejectedValueOnce(new Error('bad tag'));
      const result = (await handlers['UNLOCK_WITH_PIN']({ pin: '000000' })) as Record<string, unknown>;
      expect(result.ok).toBe(false);
      expect(result.wrongPin).toBe(true);
      expect(result.triesRemaining).toBe(4);
    });

    it('UNLOCK_WITH_PIN wipes the PIN on the final wrong attempt', async () => {
      mockBrowser.storage.local.get.mockResolvedValue(
        pinLocal({ [STORAGE_KEYS.PIN_FAIL_COUNT]: 4 }), // one attempt left of 5
      );
      mockUnlockWithPin.mockRejectedValueOnce(new Error('bad tag'));
      const result = (await handlers['UNLOCK_WITH_PIN']({ pin: '000000' })) as Record<string, unknown>;
      expect(result).toEqual({ ok: false, lockedOut: true });
      expect(mockBrowser.storage.local.remove).toHaveBeenCalled();
    });

    it('UNLOCK_WITH_PIN does not consume a try or wipe the PIN when the session refresh fails', async () => {
      mockBrowser.storage.local.get.mockResolvedValue(pinLocal());
      // Correct PIN: the crypto unwrap succeeds...
      mockUnlockWithPin.mockResolvedValueOnce({
        privX25519: new Uint8Array(32),
        privMLKEM: new Uint8Array(64),
      });
      // ...but the server-side token refresh inside buildSession fails (e.g. a
      // stale/rotated refresh token or offline).
      mockClient.mockImplementationOnce(() => ({
        setTokens: vi.fn(),
        refresh: vi.fn().mockRejectedValue(new Error('401 session expired or revoked')),
        listEntries: vi.fn().mockResolvedValue([]),
      }));
      const result = (await handlers['UNLOCK_WITH_PIN']({ pin: '123456' })) as Record<string, unknown>;
      expect(result.ok).toBe(false);
      expect(typeof result.error).toBe('string');
      // PIN must NOT be wiped...
      expect(mockBrowser.storage.local.remove).not.toHaveBeenCalled();
      // ...and the pre-incremented counter is rolled back to its original value.
      const setCalls = mockBrowser.storage.local.set.mock.calls;
      expect(setCalls[setCalls.length - 1][0]).toEqual({ [STORAGE_KEYS.PIN_FAIL_COUNT]: 0 });
    });
  });

  describe('savePinRefreshToken', () => {
    it('updates only the bootstrap refresh token, leaving the rest intact', async () => {
      mockBrowser.storage.local.get.mockResolvedValue({
        [STORAGE_KEYS.PIN_ENABLED]: true,
        [STORAGE_KEYS.PIN_BOOTSTRAP]: { refresh_token: 'old-rt', user_id: 'u1' },
      });
      await savePinRefreshToken('new-rt');
      expect(mockBrowser.storage.local.set).toHaveBeenCalledWith({
        [STORAGE_KEYS.PIN_BOOTSTRAP]: { refresh_token: 'new-rt', user_id: 'u1' },
      });
    });

    it('is a no-op when no PIN is enabled', async () => {
      mockBrowser.storage.local.get.mockResolvedValue({});
      await savePinRefreshToken('new-rt');
      expect(mockBrowser.storage.local.set).not.toHaveBeenCalled();
    });
  });
});

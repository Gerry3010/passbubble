import { describe, it, expect, vi, beforeEach } from 'vitest';

// Must be hoisted before importing the module under test
vi.mock('webextension-polyfill', () => ({
  default: {
    storage: {
      sync: { get: vi.fn(), set: vi.fn() },
      session: { get: vi.fn(), set: vi.fn() },
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
}));

import browser from 'webextension-polyfill';
import { getSession, clearSession } from '../session-store.js';
import { buildHandlers } from '../message-handler.js';
import { STORAGE_KEYS } from '../../shared/constants.js';

const mockBrowser = browser as unknown as {
  storage: {
    sync: { get: ReturnType<typeof vi.fn>; set: ReturnType<typeof vi.fn> };
    session: { get: ReturnType<typeof vi.fn>; set: ReturnType<typeof vi.fn> };
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
});

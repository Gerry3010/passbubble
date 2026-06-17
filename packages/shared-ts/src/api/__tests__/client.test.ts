import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { PassbubbleClient } from '../client.js';
import { ApiError } from '../errors.js';

const BASE = 'https://passbubble.example.com';

function mockFetch(status: number, body: unknown) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: String(status),
    json: async () => body,
  } as Response);
}

describe('PassbubbleClient', () => {
  let client: PassbubbleClient;

  beforeEach(() => {
    client = new PassbubbleClient(BASE);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('login sends POST /api/v1/auth/login and returns response', async () => {
    const loginResp = {
      access_token: 'at123',
      refresh_token: 'rt456',
      expires_in: 900,
      token_type: 'Bearer',
      user_id: 'uid1',
      email: 'test@example.com',
      name: 'Test',
      role: 'user',
      enc_priv_x25519: 'aaaa',
      enc_priv_mlkem768: 'bbbb',
      pub_x25519: 'cccc',
      pub_mlkem768: 'dddd',
      kdf_salt: 'eeee',
      kdf_time: 3,
      kdf_memory: 65536,
    };
    vi.stubGlobal('fetch', mockFetch(200, loginResp));

    const result = await client.login('test@example.com', 'password');
    expect(result.access_token).toBe('at123');
    expect(result.user_id).toBe('uid1');

    const call = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe(`${BASE}/api/v1/auth/login`);
    expect(call[1].method).toBe('POST');
    expect(JSON.parse(call[1].body)).toEqual({ email: 'test@example.com', password: 'password' });
    // No Authorization header for login
    expect(call[1].headers['Authorization']).toBeUndefined();
  });

  it('setTokens injects Bearer token on subsequent requests', async () => {
    client.setTokens('mytoken', 'rt', 900);
    vi.stubGlobal('fetch', mockFetch(200, []));
    await client.listEntries();
    const call = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers['Authorization']).toBe('Bearer mytoken');
  });

  it('throws ApiError on non-ok response', async () => {
    vi.stubGlobal('fetch', mockFetch(401, { error: 'invalid credentials' }));
    await expect(client.listEntries()).rejects.toBeInstanceOf(ApiError);
    await expect(client.listEntries()).rejects.toThrow('invalid credentials');
  });

  it('healthCheck returns true on 200', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true }));
    expect(await client.healthCheck()).toBe(true);
  });

  it('healthCheck returns false on network error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network error')));
    expect(await client.healthCheck()).toBe(false);
  });

  it('isTokenExpiringSoon returns true when within 60s of expiry', () => {
    client.setTokens('tok', 'rt', 30); // expires in 30s
    expect(client.isTokenExpiringSoon()).toBe(true);
  });

  it('isTokenExpiringSoon returns false for fresh token', () => {
    client.setTokens('tok', 'rt', 900); // expires in 15min
    expect(client.isTokenExpiringSoon()).toBe(false);
  });

  it('generate sends POST /api/v1/generate with request body', async () => {
    vi.stubGlobal('fetch', mockFetch(200, { passwords: [{ password: 'abc', strength: 80 }] }));
    client.setTokens('tok', 'rt', 900);
    const result = await client.generate({ length: 20, include_symbols: true });
    expect(result.passwords[0].password).toBe('abc');
    const call = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe(`${BASE}/api/v1/generate`);
    expect(call[1].method).toBe('POST');
    expect(JSON.parse(call[1].body)).toMatchObject({ length: 20, include_symbols: true });
  });
});

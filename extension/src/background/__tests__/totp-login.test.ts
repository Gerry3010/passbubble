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

const loginMock = vi.fn();
const verifyTotpMock = vi.fn();

vi.mock('webextension-polyfill', () => ({
  default: {
    storage: {
      sync: { get: vi.fn().mockResolvedValue({ server_url: 'https://srv' }), set: vi.fn() },
      session: { get: vi.fn().mockResolvedValue({}), set: vi.fn().mockResolvedValue(undefined) },
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
    login: loginMock,
    verifyTotp: verifyTotpMock,
  })),
  unlock: vi.fn(),
  decryptEntry: vi.fn(),
  encryptEntry: vi.fn(),
  createEntry: vi.fn(),
}));

import browser from 'webextension-polyfill';
import { buildHandlers } from '../message-handler.js';
import { MessageType } from '../../shared/constants.js';

const mockBrowser = browser as unknown as {
  storage: { session: { set: ReturnType<typeof vi.fn> } };
};

const fullLogin = {
  access_token: 'at',
  refresh_token: 'rt',
  expires_in: 900,
  user_id: 'u1',
  email: 'a@b.c',
  name: 'Alice',
  role: 'user',
  enc_priv_x25519: 'e1',
  enc_priv_mlkem768: 'e2',
  pub_x25519: 'p1',
  pub_mlkem768: 'p2',
  kdf_salt: 's',
  kdf_time: 3,
  kdf_memory: 65536,
};

describe('2FA login flow (background)', () => {
  beforeEach(() => {
    loginMock.mockReset();
    verifyTotpMock.mockReset();
    mockBrowser.storage.session.set.mockClear();
  });

  it('LOGIN returns requiresTotp and does NOT persist when 2FA is required', async () => {
    loginMock.mockResolvedValue({ status: '2fa_required', pending_token: 'pending-1' });
    const handlers = buildHandlers();

    const res = (await handlers[MessageType.LOGIN]({ email: 'a@b.c', password: 'pw' })) as {
      requiresTotp?: boolean;
      pendingToken?: string;
    };

    expect(res.requiresTotp).toBe(true);
    expect(res.pendingToken).toBe('pending-1');
    expect(mockBrowser.storage.session.set).not.toHaveBeenCalled();
  });

  it('LOGIN persists the session when 2FA is not required', async () => {
    loginMock.mockResolvedValue(fullLogin);
    const handlers = buildHandlers();

    const res = (await handlers[MessageType.LOGIN]({ email: 'a@b.c', password: 'pw' })) as {
      needsUnlock?: boolean;
    };

    expect(res.needsUnlock).toBe(true);
    expect(mockBrowser.storage.session.set).toHaveBeenCalledTimes(1);
  });

  it('VERIFY_TOTP verifies the code and persists the session', async () => {
    verifyTotpMock.mockResolvedValue(fullLogin);
    const handlers = buildHandlers();

    const res = (await handlers[MessageType.VERIFY_TOTP]({
      pendingToken: 'pending-1',
      code: '123456',
    })) as { needsUnlock?: boolean };

    expect(verifyTotpMock).toHaveBeenCalledWith('pending-1', '123456');
    expect(res.needsUnlock).toBe(true);
    expect(mockBrowser.storage.session.set).toHaveBeenCalledTimes(1);
  });
});

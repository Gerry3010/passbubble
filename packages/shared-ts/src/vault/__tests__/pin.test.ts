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

import { describe, expect, it } from 'vitest';
import {
  clampPwIntervalDays,
  generatePinSalt,
  pwIntervalElapsed,
  unlockWithPin,
  unwrapMasterKeyWithPin,
  wrapMasterKeyWithPin,
  PIN_PW_INTERVAL_MAX_DAYS,
  PIN_PW_INTERVAL_MIN_DAYS,
} from '../pin.js';
import { b64Enc } from '../vault.js';
import { generateX25519 } from '../../crypto/x25519.js';
import { generateMLKEM768 } from '../../crypto/mlkem.js';
import { deriveKey } from '../../crypto/argon2.js';
import { aesGcmEncrypt } from '../../crypto/aes-gcm.js';

describe('pin crypto', () => {
  it('wrap + unwrap round-trips the master key', async () => {
    const masterKey = crypto.getRandomValues(new Uint8Array(32));
    const pinSalt = generatePinSalt();
    const wrapped = await wrapMasterKeyWithPin(masterKey, '123456', pinSalt);
    const recovered = await unwrapMasterKeyWithPin(wrapped, '123456', pinSalt);
    expect(recovered).toEqual(masterKey);
  }, 30_000);

  it('unwrap with the wrong PIN throws (GCM tag failure)', async () => {
    const masterKey = crypto.getRandomValues(new Uint8Array(32));
    const pinSalt = generatePinSalt();
    const wrapped = await wrapMasterKeyWithPin(masterKey, '123456', pinSalt);
    await expect(unwrapMasterKeyWithPin(wrapped, '000000', pinSalt)).rejects.toThrow();
  }, 30_000);

  it('unlockWithPin recovers the private keys (mirrors vault.unlock)', async () => {
    const { priv: privX } = generateX25519();
    const { priv: privM } = await generateMLKEM768();

    // Master key derived from the master password at login time.
    const masterKey = await deriveKey('master-pw', {
      salt: crypto.getRandomValues(new Uint8Array(32)),
      time: 1,
      memory: 8192,
    });
    const encPrivX = await aesGcmEncrypt(masterKey, privX);
    const encPrivM = await aesGcmEncrypt(masterKey, privM);

    // PIN setup wraps that same master key.
    const pinSalt = generatePinSalt();
    const wrapped = await wrapMasterKeyWithPin(masterKey, '654321', pinSalt);

    const { privX25519, privMLKEM } = await unlockWithPin(
      '654321',
      pinSalt,
      wrapped,
      b64Enc(encPrivX),
      b64Enc(encPrivM),
    );
    expect(privX25519).toEqual(privX);
    expect(privMLKEM).toEqual(privM);
  }, 30_000);

  it('clampPwIntervalDays enforces the 1..60 day bounds', () => {
    expect(clampPwIntervalDays(0)).toBe(PIN_PW_INTERVAL_MIN_DAYS);
    expect(clampPwIntervalDays(1000)).toBe(PIN_PW_INTERVAL_MAX_DAYS);
    expect(clampPwIntervalDays(14)).toBe(14);
    expect(clampPwIntervalDays(NaN)).toBeGreaterThanOrEqual(PIN_PW_INTERVAL_MIN_DAYS);
  });

  it('pwIntervalElapsed flips once the interval has passed', () => {
    const now = 1_000_000_000_000;
    const dayMs = 24 * 60 * 60 * 1000;
    expect(pwIntervalElapsed(now - 13 * dayMs, 14, now)).toBe(false);
    expect(pwIntervalElapsed(now - 15 * dayMs, 14, now)).toBe(true);
  });
});

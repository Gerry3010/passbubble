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

// PIN-based quick-unlock crypto core (shared across all clients).
//
// The PIN itself is never stored. It derives (Argon2id, with its own salt) a
// wrap-key that AES-256-GCM-encrypts a local copy of the master key. A wrong
// PIN simply fails the GCM auth-tag check, so no separate PIN hash is needed.
//
// Security: the failure counter only guards the in-app path. Anyone who copies
// the wrapped key + salt off disk can brute-force a 6-digit PIN offline, so the
// storage backend's protection (hardware keystore vs. plain file) is what
// ultimately matters — callers warn the user accordingly.

import { aesGcmDecrypt, aesGcmEncrypt } from '../crypto/aes-gcm.js';
import { deriveKey } from '../crypto/argon2.js';
import type { KDFParams } from '../types/vault.js';
import { b64Dec } from './vault.js';

// PIN Argon2id cost — same as the master key (Go defaults), fresh per-PIN salt.
export const PIN_KDF_TIME = 3;
export const PIN_KDF_MEMORY = 65536;
export const PIN_SALT_LEN = 16;

export const DEFAULT_PIN_MAX_TRIES = 5;
export const DEFAULT_PIN_PW_INTERVAL_DAYS = 14;
export const PIN_PW_INTERVAL_MIN_DAYS = 1;
export const PIN_PW_INTERVAL_MAX_DAYS = 60; // 2 months

export function clampPwIntervalDays(days: number): number {
  if (!Number.isFinite(days)) return DEFAULT_PIN_PW_INTERVAL_DAYS;
  return Math.min(PIN_PW_INTERVAL_MAX_DAYS, Math.max(PIN_PW_INTERVAL_MIN_DAYS, Math.round(days)));
}

export function generatePinSalt(): Uint8Array {
  return crypto.getRandomValues(new Uint8Array(PIN_SALT_LEN));
}

export function pinKdfParams(pinSalt: Uint8Array): KDFParams {
  return { salt: pinSalt, time: PIN_KDF_TIME, memory: PIN_KDF_MEMORY };
}

// Wrap a master key under a PIN-derived key. Returns nonce(12)||ciphertext.
export async function wrapMasterKeyWithPin(
  masterKey: Uint8Array,
  pin: string,
  pinSalt: Uint8Array,
): Promise<Uint8Array> {
  const pinKey = await deriveKey(pin, pinKdfParams(pinSalt));
  return aesGcmEncrypt(pinKey, masterKey);
}

// Recover the master key from a PIN. Throws if the PIN is wrong (GCM tag fails).
export async function unwrapMasterKeyWithPin(
  pinWrapped: Uint8Array,
  pin: string,
  pinSalt: Uint8Array,
): Promise<Uint8Array> {
  const pinKey = await deriveKey(pin, pinKdfParams(pinSalt));
  return aesGcmDecrypt(pinKey, pinWrapped);
}

// Full PIN unlock: recover master key, then decrypt the private keys — mirrors
// vault.unlock() but keyed by PIN instead of the master password.
export async function unlockWithPin(
  pin: string,
  pinSalt: Uint8Array,
  pinWrapped: Uint8Array,
  encPrivX25519Base64: string,
  encPrivMLKEMBase64: string,
): Promise<{ privX25519: Uint8Array; privMLKEM: Uint8Array }> {
  const masterKey = await unwrapMasterKeyWithPin(pinWrapped, pin, pinSalt);
  const privX25519 = await aesGcmDecrypt(masterKey, b64Dec(encPrivX25519Base64));
  const privMLKEM = await aesGcmDecrypt(masterKey, b64Dec(encPrivMLKEMBase64));
  return { privX25519, privMLKEM };
}

// True when the master password must be re-entered (PIN interval has elapsed).
export function pwIntervalElapsed(
  lastMasterUnlockMs: number,
  intervalDays: number,
  nowMs: number = Date.now(),
): boolean {
  const intervalMs = clampPwIntervalDays(intervalDays) * 24 * 60 * 60 * 1000;
  return nowMs - lastMasterUnlockMs >= intervalMs;
}

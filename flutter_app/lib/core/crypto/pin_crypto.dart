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

import 'dart:typed_data';

import 'vault_crypto.dart';

/// PIN quick-unlock crypto. The PIN is never stored; it derives (Argon2id, its
/// own salt) a wrap-key that AES-256-GCM-encrypts a copy of the master key.
/// A wrong PIN simply fails the GCM auth tag, so no separate PIN hash is needed.
class PinCrypto {
  // PIN Argon2id cost — same as the master key, with a fresh per-PIN salt.
  static const int kdfMemory = 65536;
  static const int kdfIterations = 3;
  static const int saltLen = 16;

  static const int defaultMaxTries = 5;
  static const int defaultIntervalDays = 14;
  static const int minIntervalDays = 1;
  static const int maxIntervalDays = 60; // 2 months

  static int clampIntervalDays(int days) {
    if (days < minIntervalDays) return minIntervalDays;
    if (days > maxIntervalDays) return maxIntervalDays;
    return days;
  }

  /// Wraps [masterKeyBytes] under a PIN-derived key. Returns nonce||ciphertext.
  static Future<Uint8List> wrapMasterKey(
    Uint8List masterKeyBytes,
    String pin,
    Uint8List pinSalt,
  ) async {
    final pinKey = await VaultCrypto.deriveMasterKey(
      pin,
      pinSalt,
      memory: kdfMemory,
      iterations: kdfIterations,
    );
    return VaultCrypto.encrypt(pinKey, masterKeyBytes);
  }

  /// Recovers the master key bytes from a PIN. Throws if the PIN is wrong.
  static Future<Uint8List> unwrapMasterKey(
    Uint8List wrapped,
    String pin,
    Uint8List pinSalt,
  ) async {
    final pinKey = await VaultCrypto.deriveMasterKey(
      pin,
      pinSalt,
      memory: kdfMemory,
      iterations: kdfIterations,
    );
    return VaultCrypto.decrypt(pinKey, wrapped);
  }
}

/// Outcome of a PIN unlock attempt.
enum PinUnlockStatus { ok, wrongPin, expired, lockedOut, notEnabled }

class PinUnlockResult {
  final PinUnlockStatus status;
  final int triesRemaining;
  const PinUnlockResult(this.status, {this.triesRemaining = 0});
}

/// PIN configuration snapshot for the settings/unlock UI.
class PinStatus {
  final bool enabled;
  final bool expired;
  final int intervalDays;
  final int triesRemaining;
  const PinStatus({
    required this.enabled,
    this.expired = false,
    this.intervalDays = PinCrypto.defaultIntervalDays,
    this.triesRemaining = 0,
  });
}

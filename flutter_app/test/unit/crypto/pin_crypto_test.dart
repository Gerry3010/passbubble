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

import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/crypto/pin_crypto.dart';
import 'package:passbubble/core/crypto/vault_crypto.dart';

void main() {
  group('PinCrypto', () {
    test('wrap + unwrap round-trips the master key', () async {
      final masterKey = VaultCrypto.randomKey();
      final pinSalt = VaultCrypto.randomSalt(PinCrypto.saltLen);

      final wrapped = await PinCrypto.wrapMasterKey(masterKey, '123456', pinSalt);
      final recovered =
          await PinCrypto.unwrapMasterKey(wrapped, '123456', pinSalt);

      expect(recovered, equals(masterKey));
    });

    test('unwrap with the wrong PIN throws', () async {
      final masterKey = VaultCrypto.randomKey();
      final pinSalt = VaultCrypto.randomSalt(PinCrypto.saltLen);
      final wrapped = await PinCrypto.wrapMasterKey(masterKey, '123456', pinSalt);

      expect(
        () => PinCrypto.unwrapMasterKey(wrapped, '000000', pinSalt),
        throwsA(anything),
      );
    });

    test('wrong salt does not recover the key', () async {
      final masterKey = VaultCrypto.randomKey();
      final saltA = VaultCrypto.randomSalt(PinCrypto.saltLen);
      final saltB = VaultCrypto.randomSalt(PinCrypto.saltLen);
      final wrapped = await PinCrypto.wrapMasterKey(masterKey, '123456', saltA);

      expect(
        () => PinCrypto.unwrapMasterKey(
            Uint8List.fromList(wrapped), '123456', saltB),
        throwsA(anything),
      );
    });

    test('clampIntervalDays enforces 1..60', () {
      expect(PinCrypto.clampIntervalDays(0), PinCrypto.minIntervalDays);
      expect(PinCrypto.clampIntervalDays(-5), PinCrypto.minIntervalDays);
      expect(PinCrypto.clampIntervalDays(14), 14);
      expect(PinCrypto.clampIntervalDays(61), PinCrypto.maxIntervalDays);
      expect(PinCrypto.clampIntervalDays(1000), PinCrypto.maxIntervalDays);
    });
  });
}

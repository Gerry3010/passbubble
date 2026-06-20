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

@TestOn('vm')
library;

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/crypto/ml_kem.dart';
import 'package:passbubble/core/crypto/vault_crypto.dart';

const _libPath = 'native/passbubble_crypto/build/libpassbubble_crypto.so';

void main() {
  final libExists = File(_libPath).existsSync();

  setUpAll(() {
    mlKemLibraryPathOverride = File(_libPath).absolute.path;
  });

  group('VaultCrypto hybrid data-key wrapping', () {
    test('hybrid encrypt → decrypt round-trips via VaultCrypto', () async {
      final x = await VaultCrypto.generateX25519KeyPair();
      final privX = Uint8List.fromList(await x.extractPrivateKeyBytes());
      final pubX = Uint8List.fromList((await x.extractPublicKey()).bytes);
      final (privM, pubM) = await mlKemGenerate();

      final dataKey = VaultCrypto.randomKey();
      final encB64 = await VaultCrypto.encryptDataKey(
          dataKey, base64.encode(pubX), base64.encode(pubM));
      final recovered = await VaultCrypto.decryptDataKey(encB64, privX, privM);

      expect(recovered, equals(dataKey));
    });

    test('falls back to X25519-only when the recipient has no ML-KEM key', () async {
      final x = await VaultCrypto.generateX25519KeyPair();
      final privX = Uint8List.fromList(await x.extractPrivateKeyBytes());
      final pubX = Uint8List.fromList((await x.extractPublicKey()).bytes);
      final (privM, _) = await mlKemGenerate();

      final dataKey = VaultCrypto.randomKey();
      // 32-byte placeholder = an X25519-only account → legacy wire format.
      final placeholder = base64.encode(Uint8List(32));
      final encB64 = await VaultCrypto.encryptDataKey(dataKey, base64.encode(pubX), placeholder);

      // Legacy blob is short; decrypt auto-detects it (privM is irrelevant here).
      expect(base64.decode(encB64).length, lessThan(32 + 1088));
      final recovered = await VaultCrypto.decryptDataKey(encB64, privX, privM);
      expect(recovered, equals(dataKey));
    });
  }, skip: libExists ? false : 'native lib not built ($_libPath)');
}

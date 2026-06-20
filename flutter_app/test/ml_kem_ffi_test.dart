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

import 'package:cryptography/cryptography.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/crypto/ml_kem_ffi.dart';
import 'package:passbubble/core/crypto/vault_crypto.dart';

// Built by: cd native/passbubble_crypto && go build -buildmode=c-shared \
//   -o build/libpassbubble_crypto.so .
const _libPath = 'native/passbubble_crypto/build/libpassbubble_crypto.so';

void main() {
  final libExists = File(_libPath).existsSync();

  setUpAll(() {
    mlKemLibraryPathOverride = File(_libPath).absolute.path;
  });

  group('ml_kem_ffi (native hybrid KEM)', () {
    test('generates a real 1184-byte ML-KEM-768 public key', () async {
      final (priv, pub) = await mlKemGenerate();
      expect(pub.length, 1184);
      expect(priv.isNotEmpty, true);
    });

    test('hybrid encrypt → decrypt round-trips the data key', () async {
      final x = await X25519().newKeyPair();
      final privX = Uint8List.fromList(await x.extractPrivateKeyBytes());
      final pubX = Uint8List.fromList((await x.extractPublicKey()).bytes);
      final (privM, pubM) = await mlKemGenerate();

      final dataKey = Uint8List.fromList(List<int>.generate(32, (i) => i * 7 % 256));
      final enc = await mlKemEncryptDataKey(dataKey, pubX, pubM);
      // Hybrid wire: ephPub(32) + mlkemCT(1088) + AES-GCM(12+32+16).
      expect(enc.length, greaterThan(32 + 1088));

      final recovered = await mlKemDecryptDataKey(enc, privX, privM);
      expect(recovered, equals(dataKey));
    });

    test('decrypts a legacy X25519-only blob produced by the Flutter app', () async {
      final x = await VaultCrypto.generateX25519KeyPair();
      final privX = Uint8List.fromList(await x.extractPrivateKeyBytes());
      final pubX = Uint8List.fromList((await x.extractPublicKey()).bytes);
      final (privM, _) = await mlKemGenerate();

      final dataKey = Uint8List.fromList(List<int>.generate(32, (i) => (i + 3) % 256));

      // VaultCrypto with a 32-byte placeholder ML-KEM key falls back to the
      // legacy X25519-only format (an X25519-only account); the native lib must
      // decrypt it via its legacy auto-detect path (proves the fallback interop).
      final legacyB64 =
          await VaultCrypto.encryptDataKey(dataKey, base64.encode(pubX), base64.encode(Uint8List(32)));
      final legacy = base64.decode(legacyB64);

      final recovered = await mlKemDecryptDataKey(legacy, privX, privM);
      expect(recovered, equals(dataKey));
    });
  }, skip: libExists ? false : 'native lib not built ($_libPath) — run the go c-shared build');
}

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

import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/crypto/vault_crypto.dart';
import 'package:cryptography/cryptography.dart';

void main() {
  group('VaultCrypto', () {
    group('encrypt / decrypt roundtrip', () {
      test('decrypted bytes match original plaintext', () async {
        final key = SecretKey(VaultCrypto.randomKey());
        final plaintext = Uint8List.fromList(utf8.encode('hello world 🔐'));

        final ciphertext = await VaultCrypto.encrypt(key, plaintext);
        final recovered = await VaultCrypto.decrypt(key, ciphertext);

        expect(recovered, equals(plaintext));
      });

      test('ciphertext differs from plaintext', () async {
        final key = SecretKey(VaultCrypto.randomKey());
        final plaintext = Uint8List.fromList(utf8.encode('sensitive data'));
        final ciphertext = await VaultCrypto.encrypt(key, plaintext);
        expect(ciphertext, isNot(equals(plaintext)));
      });

      test('wrong key fails to decrypt', () async {
        final key1 = SecretKey(VaultCrypto.randomKey());
        final key2 = SecretKey(VaultCrypto.randomKey());
        final plaintext = Uint8List.fromList(utf8.encode('secret'));
        final ciphertext = await VaultCrypto.encrypt(key1, plaintext);
        expect(
          () => VaultCrypto.decrypt(key2, ciphertext),
          throwsA(anything),
        );
      });

      test('tampered ciphertext fails to decrypt', () async {
        final key = SecretKey(VaultCrypto.randomKey());
        final plaintext = Uint8List.fromList(utf8.encode('secret'));
        final ciphertext = await VaultCrypto.encrypt(key, plaintext);
        final tampered = Uint8List.fromList(ciphertext)
          ..[15] ^= 0xFF;
        expect(
          () => VaultCrypto.decrypt(key, tampered),
          throwsA(anything),
        );
      });
    });

    group('deriveMasterKey', () {
      // Use minimal Argon2 params in tests — we're verifying determinism and
      // output properties, not KDF hardness. Production params (memory=65536,
      // iterations=3) exceed the 30s CI timeout.
      const testMemory = 1024;
      const testIter = 1;

      test('same password + salt yields same key bytes', () async {
        final salt = VaultCrypto.randomSalt();
        final k1 = await VaultCrypto.deriveMasterKey('mypassword', salt,
            memory: testMemory, iterations: testIter);
        final k2 = await VaultCrypto.deriveMasterKey('mypassword', salt,
            memory: testMemory, iterations: testIter);
        expect(await k1.extractBytes(), equals(await k2.extractBytes()));
      });

      test('different passwords yield different keys', () async {
        final salt = VaultCrypto.randomSalt();
        final k1 = await VaultCrypto.deriveMasterKey('password1', salt,
            memory: testMemory, iterations: testIter);
        final k2 = await VaultCrypto.deriveMasterKey('password2', salt,
            memory: testMemory, iterations: testIter);
        expect(await k1.extractBytes(), isNot(equals(await k2.extractBytes())));
      });

      test('different salts yield different keys', () async {
        final s1 = VaultCrypto.randomSalt();
        final s2 = VaultCrypto.randomSalt();
        final k1 = await VaultCrypto.deriveMasterKey('password', s1,
            memory: testMemory, iterations: testIter);
        final k2 = await VaultCrypto.deriveMasterKey('password', s2,
            memory: testMemory, iterations: testIter);
        expect(await k1.extractBytes(), isNot(equals(await k2.extractBytes())));
      });

      test('derived key is 32 bytes', () async {
        final salt = VaultCrypto.randomSalt();
        final key = await VaultCrypto.deriveMasterKey('password', salt,
            memory: testMemory, iterations: testIter);
        final bytes = await key.extractBytes();
        expect(bytes.length, equals(32));
      });
    });

    group('encryptEntryData / decryptEntryData roundtrip', () {
      test('map is preserved through encrypt + decrypt cycle', () async {
        final data = {
          'username': 'alice@example.com',
          'password': 'P@ssw0rd!',
          'notes': 'test entry',
          'totp_secret': 'JBSWY3DPEHPK3PXP',
        };

        final (:encryptedData, :dataNonce, :dataKey) =
            await VaultCrypto.encryptEntryData(data);

        final recovered = await VaultCrypto.decryptEntryData(
          encryptedData,
          dataKey,
        );

        expect(recovered['username'], equals(data['username']));
        expect(recovered['password'], equals(data['password']));
        expect(recovered['notes'], equals(data['notes']));
        expect(recovered['totp_secret'], equals(data['totp_secret']));
      });

      test('data nonce placeholder is 12 zero bytes', () async {
        final (:encryptedData, :dataNonce, :dataKey) =
            await VaultCrypto.encryptEntryData({'key': 'value'});
        final nonceBytes = base64.decode(dataNonce);
        expect(nonceBytes.length, equals(12));
        expect(nonceBytes.every((b) => b == 0), isTrue);
      });

      test('each encryption produces a unique ciphertext', () async {
        final data = {'username': 'test', 'password': 'pass'};
        final r1 = await VaultCrypto.encryptEntryData(data);
        final r2 = await VaultCrypto.encryptEntryData(data);
        // Different random keys → different ciphertexts
        expect(r1.encryptedData, isNot(equals(r2.encryptedData)));
      });
    });

    group('encryptDataKey / decryptDataKey roundtrip (X25519 ECDH)', () {
      test('data key is recovered correctly', () async {
        final keyPair = await VaultCrypto.generateX25519KeyPair();
        final pubBytes =
            Uint8List.fromList((await keyPair.extractPublicKey()).bytes);
        final privBytes = Uint8List.fromList(
          (await keyPair.extract()).bytes,
        );

        final dataKey = VaultCrypto.randomKey();
        final encrypted = await VaultCrypto.encryptDataKey(
          dataKey,
          base64.encode(pubBytes),
        );
        final recovered = await VaultCrypto.decryptDataKey(encrypted, privBytes);

        expect(recovered, equals(dataKey));
      });

      test('wrong private key fails to recover data key', () async {
        final kp1 = await VaultCrypto.generateX25519KeyPair();
        final kp2 = await VaultCrypto.generateX25519KeyPair();
        final pub1 = Uint8List.fromList((await kp1.extractPublicKey()).bytes);
        final priv2 = Uint8List.fromList((await kp2.extract()).bytes);

        final dataKey = VaultCrypto.randomKey();
        final encrypted = await VaultCrypto.encryptDataKey(
          dataKey,
          base64.encode(pub1),
        );

        expect(
          () => VaultCrypto.decryptDataKey(encrypted, priv2),
          throwsA(anything),
        );
      });
    });

    group('randomKey / randomSalt', () {
      test('randomKey returns 32 bytes', () {
        expect(VaultCrypto.randomKey().length, equals(32));
      });

      test('randomKey values are different on each call', () {
        final k1 = VaultCrypto.randomKey();
        final k2 = VaultCrypto.randomKey();
        expect(k1, isNot(equals(k2)));
      });

      test('randomSalt default is 32 bytes', () {
        expect(VaultCrypto.randomSalt().length, equals(32));
      });

      test('randomSalt respects custom length', () {
        expect(VaultCrypto.randomSalt(16).length, equals(16));
      });
    });
  });
}

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

import 'dart:collection';
import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/api/api_client.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/core/crypto/vault_crypto.dart';

/// Answers every request with 204 (enough for logout).
class _NoContentAdapter implements HttpClientAdapter {
  @override
  Future<ResponseBody> fetch(RequestOptions options,
          Stream<Uint8List>? requestStream, Future<void>? cancelFuture) async =>
      ResponseBody.fromString('', 204);

  @override
  void close({bool force = false}) {}
}

/// Backing map for a storage whose values cannot be decrypted any more (web:
/// the AES key they were written under is gone) — every read throws.
class _UnreadableStore extends MapBase<String, String> {
  @override
  String? operator [](Object? key) => throw StateError('OperationError');

  @override
  void operator []=(String key, String value) {}

  @override
  void clear() {}

  @override
  Iterable<String> get keys => const [];

  @override
  String? remove(Object? key) => null;
}

const _sessionKeys = {
  'access_token': 'A1',
  'refresh_token': 'RT1',
  'user_id': 'u1',
  'email': 'a@b.c',
  'role': 'user',
  'name': 'A',
  'kdf_salt': 'c2FsdA==',
  'kdf_time': '1',
  'kdf_memory': '1024',
  'enc_priv_x25519': 'eA==',
  'enc_priv_mlkem768': 'bQ==',
  'pub_x25519': 'cA==',
  'pub_mlkem768': 'cQ==',
  'biometric_master': 'secret',
  'pin_enabled': '1',
  'pin_salt': 'cw==',
  'pin_wrapped_master_key': 'dw==',
  'pin_max_tries': '5',
  'pin_fail_count': '0',
  'pin_pw_interval_days': '14',
  'pin_last_master_unlock': '0',
};

/// Keys that are not session data and must survive logout / expiry.
const _keptKeys = {
  'server_url': 'https://vault.example',
  'auto_lock_minutes': '30',
  'autofill_intro_seen': '1',
  // flutter_secure_storage_web keeps its AES key under this name.
  'FlutterSecureStorage': 'aes-key-jwk',
  'some_other_app_on_this_origin': 'x',
};

void main() {
  group('session wipe', () {
    late Map<String, String> store;
    late AuthService svc;

    setUp(() async {
      store = {..._sessionKeys, ..._keptKeys};
      FlutterSecureStorage.setMockInitialValues(store);
      final api = ApiClient(httpClientAdapter: _NoContentAdapter());
      await api.init();
      api.setBaseUrlEphemeral('http://test');
      svc = AuthService(api);
    });

    test('session expiry deletes only the app session keys', () async {
      await svc.clearLocalSession();
      expect(store, _keptKeys);
    });

    test('logout deletes only the app session keys', () async {
      await svc.logout();
      expect(store, _keptKeys);
    });
  });

  group('unlockWithMasterPassword', () {
    const password = 'correct horse battery staple';
    late Map<String, String> store;
    late AuthService svc;

    /// Stores key material the way login does, with a cheap KDF for tests.
    Future<void> storeKeys({bool withMlkem = true}) async {
      final salt = VaultCrypto.randomSalt();
      final masterKey = await VaultCrypto.deriveMasterKey(password, salt,
          memory: 1024, iterations: 1);
      store
        ..['kdf_salt'] = base64.encode(salt)
        ..['kdf_time'] = '1'
        ..['kdf_memory'] = '1024'
        ..['enc_priv_x25519'] = base64.encode(
            await VaultCrypto.encrypt(masterKey, Uint8List(32)..[0] = 7))
        ..['enc_priv_mlkem768'] = withMlkem
            ? base64.encode(
                await VaultCrypto.encrypt(masterKey, Uint8List(64)..[0] = 9))
            : '';
    }

    setUp(() {
      store = {};
      FlutterSecureStorage.setMockInitialValues(store);
      svc = AuthService(ApiClient());
    });

    test('right password unlocks and loads both keys', () async {
      await storeKeys();
      expect(await svc.unlockWithMasterPassword(password), UnlockStatus.ok);
      expect(svc.hasVaultKeys, isTrue);
      expect(svc.privX25519![0], 7);
      expect(svc.privMLKEM![0], 9);
    });

    test('X25519-only account (empty ML-KEM blob) unlocks', () async {
      await storeKeys(withMlkem: false);
      expect(await svc.unlockWithMasterPassword(password), UnlockStatus.ok);
      expect(svc.privMLKEM, isNull);
    });

    test('wrong password is reported as such and loads nothing', () async {
      await storeKeys();
      expect(await svc.unlockWithMasterPassword('nope'),
          UnlockStatus.wrongPassword);
      expect(svc.hasVaultKeys, isFalse);
    });

    test('missing key material is not a wrong password', () async {
      expect(await svc.unlockWithMasterPassword(password),
          UnlockStatus.missingKeys);
    });

    test('undecodable key material is reported as unreadable', () async {
      store
        ..['kdf_salt'] = '***'
        ..['enc_priv_x25519'] = '***';
      expect(await svc.unlockWithMasterPassword(password),
          UnlockStatus.unreadableKeys);
    });

    test('a failing storage read is reported as unreadable', () async {
      FlutterSecureStorage.setMockInitialValues(_UnreadableStore());
      expect(await svc.unlockWithMasterPassword(password),
          UnlockStatus.unreadableKeys);
      expect(svc.hasVaultKeys, isFalse);
    });
  });
}

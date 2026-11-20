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

// The autofill sync on iOS and macOS: pending passkeys from the AutoFill
// extension are saved into the vault first and cleared by id, then every
// passkey goes to the extension with its entry. Real crypto, see fake_vault.
@TestOn('vm')
library;

import 'dart:convert';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/autofill/autofill_service.dart';

import '../../fixtures/fake_vault.dart';
import '../../fixtures/passkey_fixtures.dart';

Map<String, dynamic> _pending(String id, {String rpId = 'example.com', String user = 'alice'}) => {
      'credential_id': id,
      'rp_id': rpId,
      'rp_name': 'Example',
      'user_handle': 'dXNlcg',
      'user_name': user,
      'key_algorithm': -7,
      'private_key': p256Long,
      'sign_count': 0,
      'discoverable': true,
      'prf_secret': 'cHJm',
      'created_at': '2026-11-14T03:00:00Z',
    };

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  late TestKeys keys;

  setUpAll(() async {
    if (!nativeCryptoBuilt) return;
    useNativeCrypto();
    keys = await TestKeys.generate();
  });

  group('sync on Apple platforms', () {
    test('pending passkeys are attached, cleared by id, and synced with their entry',
        () async {
      final login = await keys.encryptEntry(
          {'username': 'Alice', 'password': 'pw'},
          id: 'e1', name: 'Example', url: 'https://www.example.com/login');
      final api = FakeVaultApi([login]);
      final bridge = FakeAppleBridge(
          pending: jsonEncode([
        // Matches e1: host is a subdomain of the rp id, username ignoring case.
        {..._pending('AAAA=='), 'credential_id': 'AAAA=='},
        // No login for it: a new entry.
        _pending('BBBB', rpId: 'other.org', user: 'bob'),
        // Unusable: stays pending.
        {'rp_id': 'x.com'},
      ]));
      final auth = FakeVaultAuth(api, keys, autofill: bridge, realSync: true);

      await auth.refreshAutofill();

      expect(bridge.cleared, [
        ['AAAA==', 'BBBB'],
      ], reason: 'cleared with the ids exactly as the extension wrote them');
      final e1 = await keys.decrypt(api.entries['e1']!);
      expect(e1['password'], 'pw');
      expect((e1['passkeys'] as List).single['credential_id'], 'AAAA');
      expect(api.created.single.name, 'Example');
      expect(api.created.single.url, 'https://other.org');
      final created = await keys.decrypt(api.entries['new-1']!);
      expect(created['username'], 'bob');
      expect((created['passkeys'] as List).single['prf_secret'], 'cHJm');

      final synced = (jsonDecode(bridge.syncedPasskeys!) as List).cast<Map>();
      expect(synced.map((p) => (p['credential_id'], p['entry_id'], p['entry_name'])), [
        ('AAAA', 'e1', 'Example'),
        ('BBBB', 'new-1', 'Example'),
      ]);
      expect(synced.first['private_key'], p256Long);

      // The passkey-only entry is not a password credential.
      final creds = (jsonDecode(bridge.syncedCredentials!) as List).cast<Map>();
      expect(creds.map((c) => c['id']), ['e1']);
    });

    test('a pending passkey saved twice stays one passkey', () async {
      final login = await keys.encryptEntry({'username': 'alice', 'password': 'pw'},
          id: 'e1', url: 'https://example.com');
      final api = FakeVaultApi([login]);
      final bridge = FakeAppleBridge(pending: jsonEncode([_pending('AAAA')]));
      final auth = FakeVaultAuth(api, keys, autofill: bridge, realSync: true);

      await auth.refreshAutofill();
      await auth.refreshAutofill(); // the extension did not drop it in between
      final e1 = await keys.decrypt(api.entries['e1']!);
      expect((e1['passkeys'] as List), hasLength(1));
      expect(api.created, isEmpty);
    });

    test('nothing pending: no clear call, and an empty passkey list is still synced',
        () async {
      final login = await keys.encryptEntry({'username': 'alice', 'password': 'pw'}, id: 'e1');
      final api = FakeVaultApi([login]);
      final bridge = FakeAppleBridge();
      await FakeVaultAuth(api, keys, autofill: bridge, realSync: true).refreshAutofill();
      expect(bridge.cleared, isEmpty);
      expect(bridge.syncedPasskeys, '[]');
      expect(api.updates, isEmpty);
    });

    test('the detail fields go to the Apple extension only', () async {
      final login = await keys.encryptEntry({
        'username': 'alice',
        'password': 'pw',
        'notes': 'Door code',
        'custom_fields': [
          {'label': 'PIN', 'type': 'password', 'value': '4711'},
        ],
      }, id: 'e1');
      final api = FakeVaultApi([login]);
      final bridge = FakeAppleBridge(android: true);
      await FakeVaultAuth(api, keys, autofill: bridge, realSync: true).refreshAutofill();

      final apple = (jsonDecode(bridge.syncedCredentials!) as List).single as Map;
      expect(apple['password'], 'pw');
      expect(apple['notes'], 'Door code');
      expect(apple['fields'], [
        {'label': 'PIN', 'type': 'password', 'value': '4711'},
      ]);
      expect(apple['favorite'], isFalse);
      final android = (jsonDecode(bridge.androidCredentials!) as List).single as Map;
      expect(android.keys,
          unorderedEquals(['id', 'url', 'name', 'type', 'username', 'password', 'totp']));
    });

    test('elsewhere (not Apple, no Android autofill) nothing is read or synced', () async {
      final api = FakeVaultApi([]);
      final bridge = FakeAppleBridge(isApple: false, pending: jsonEncode([_pending('AAAA')]));
      await FakeVaultAuth(api, keys, autofill: bridge, realSync: true).refreshAutofill();
      expect(bridge.syncedPasskeys, isNull);
      expect(bridge.cleared, isEmpty);
      expect(api.created, isEmpty);
    });
  }, skip: nativeCryptoBuilt ? false : 'native crypto lib not built ($nativeCryptoLib)');

  group('channel', () {
    test('credentialExchangeReceived hands over the JSON string', () async {
      String? got;
      expect(
          await AutofillBridge.handleNativeCall(
              const MethodCall('credentialExchangeReceived', '{"accounts":[]}'), (j) => got = j),
          isTrue);
      expect(got, '{"accounts":[]}');
      await AutofillBridge.handleNativeCall(
          const MethodCall('credentialExchangeReceived', {'json': '{}'}), (j) => got = j);
      expect(got, '{}');
      expect(
          () => AutofillBridge.handleNativeCall(
              const MethodCall('credentialExchangeReceived', 42), (_) {}),
          throwsA(isA<PlatformException>()));
      expect(() => AutofillBridge.handleNativeCall(const MethodCall('other'), (_) {}),
          throwsA(isA<MissingPluginException>()));
    });

    test('off Apple platforms the passkey methods are no-ops', () async {
      const bridge = AutofillBridge();
      expect(bridge.isApple, isFalse);
      expect(await bridge.readPendingPasskeys(), '[]');
      await bridge.syncPasskeys('[]');
      await bridge.clearPendingPasskeys(['a']);
    });
  });
}

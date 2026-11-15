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

// Widget tests for passkeys on the entry page (list, delete, move, add by
// hand) and on the edit screen, and for the `server` entry type. Real crypto
// against an in-memory vault (test/fixtures/fake_vault.dart); without the
// native hybrid-KEM lib these tests skip.
@TestOn('vm')
library;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:passbubble/core/api/api_client.dart';
import 'package:passbubble/core/api/models.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/core/util/ssh_command.dart';
import 'package:passbubble/features/entries/add_edit_screen.dart';
import 'package:passbubble/features/entries/entry_detail_screen.dart';

import '../fixtures/fake_vault.dart';
import '../fixtures/passkey_fixtures.dart';

Map<String, dynamic> _passkey(String id, {String rpId = 'mail.example.com', String user = 'alice'}) =>
    {
      'credential_id': id,
      'rp_id': rpId,
      'user_handle': 'dXNlcg',
      'user_name': user,
      'key_algorithm': -7,
      'private_key': p256Long,
      'sign_count': 0,
      'discoverable': true,
      'created_at': '2026-11-14T12:00:00Z',
      'from_another_client': 'kept',
    };

Widget _app(FakeVaultApi api, FakeVaultAuth auth, String location) {
  final router = GoRouter(
    initialLocation: location,
    routes: [
      GoRoute(path: '/entries', builder: (_, _) => const Scaffold(body: Text('LIST'))),
      GoRoute(
        path: '/entries/new',
        builder: (_, s) => AddEditScreen(initialType: s.uri.queryParameters['type']),
      ),
      GoRoute(
        path: '/entries/:id',
        builder: (_, s) => EntryDetailScreen(id: s.pathParameters['id']!),
      ),
      GoRoute(
        path: '/entries/:id/edit',
        builder: (_, s) => AddEditScreen(editId: s.pathParameters['id']),
      ),
    ],
  );
  return ProviderScope(
    overrides: [
      apiClientProvider.overrideWithValue(api),
      authServiceProvider.overrideWithValue(auth),
    ],
    child: MaterialApp.router(routerConfig: router),
  );
}

void _tall(WidgetTester tester) {
  tester.view.physicalSize = const Size(1000, 2400);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
}

void main() {
  late TestKeys keys;

  setUpAll(() async {
    if (!nativeCryptoBuilt) return;
    useNativeCrypto();
    keys = await TestKeys.generate();
  });

  Future<Map<String, dynamic>> dataOf(WidgetTester tester, FakeVaultApi api, String id) async =>
      (await tester.runAsync(() => keys.decrypt(api.entries[id]!)))!;

  Future<({FakeVaultApi api, FakeVaultAuth auth})> open(
    WidgetTester tester,
    List<({Map<String, dynamic> data, String id, String type, String name, String permission})>
        vault,
    String location,
  ) async {
    _tall(tester);
    final entries = (await tester.runAsync(() => Future.wait([
          for (final e in vault)
            keys.encryptEntry(e.data,
                id: e.id, type: e.type, name: e.name, folderId: 'f1', permission: e.permission),
        ])))!;
    final api = FakeVaultApi(entries);
    final auth = FakeVaultAuth(api, keys);
    await tester.pumpWidget(_app(api, auth, location));
    await tester.pumpAndSettle();
    return (api: api, auth: auth);
  }

  ({Map<String, dynamic> data, String id, String type, String name, String permission}) entry(
    String id,
    Map<String, dynamic> data, {
    String type = 'password',
    String name = 'Mail',
    String permission = 'owner',
  }) =>
      (data: data, id: id, type: type, name: name, permission: permission);

  Future<void> menu(WidgetTester tester, String credentialId, String action) async {
    await tester.tap(find.descendant(
        of: find.byKey(ValueKey('passkey-$credentialId')),
        matching: find.byTooltip('Passkey actions')));
    await tester.pumpAndSettle();
    await tester.tap(find.text(action).last);
    await tester.pumpAndSettle();
  }

  group('entry page', () {
    testWidgets('lists the passkeys without their keys', (tester) async {
      await open(tester, [
        entry('e1', {
          'username': 'alice',
          'password': '',
          'passkeys': [_passkey('AAAA'), _passkey('BBBB', rpId: 'example.org', user: 'al')],
        }),
      ], '/entries/e1');

      expect(find.text('PASSKEYS'), findsOneWidget);
      expect(find.text('mail.example.com'), findsOneWidget);
      expect(find.text('alice · added 2026-11-14'), findsOneWidget);
      expect(find.text('example.org'), findsOneWidget);
      expect(find.textContaining(p256Long.substring(0, 16)), findsNothing);
      expect(find.byKey(const ValueKey('add-passkey')), findsOneWidget);
    });

    testWidgets('a read-only share lists them without actions', (tester) async {
      await open(tester, [
        entry('e1', {
          'password': 'pw',
          'passkeys': [_passkey('AAAA')],
        }, permission: 'read'),
      ], '/entries/e1');
      expect(find.text('mail.example.com'), findsOneWidget);
      expect(find.byTooltip('Passkey actions'), findsNothing);
      expect(find.byKey(const ValueKey('add-passkey')), findsNothing);
    });

    testWidgets('delete asks first and removes only that passkey', (tester) async {
      final v = await open(tester, [
        entry('e1', {
          'username': 'alice',
          'password': 'pw',
          'passkeys': [_passkey('AAAA'), _passkey('BBBB', rpId: 'example.org')],
        }),
      ], '/entries/e1');

      await menu(tester, 'AAAA', 'Delete');
      expect(find.text('Delete passkey?'), findsOneWidget);
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();
      expect(v.api.updates, isEmpty);

      await menu(tester, 'AAAA', 'Delete');
      await tester.tap(find.widgetWithText(TextButton, 'Delete'));
      await tester.pumpAndSettle();

      expect(v.api.updates.single.id, 'e1');
      expect(v.api.updates.single.req.folderId, 'f1');
      expect(v.api.updates.single.req.entryKeys, isNull);
      final data = await dataOf(tester, v.api, 'e1');
      expect(data['password'], 'pw');
      expect((data['passkeys'] as List).map((p) => p['credential_id']), ['BBBB']);
      expect(find.text('mail.example.com'), findsNothing);
      expect(find.text('example.org'), findsOneWidget);
      expect(v.auth.autofillRefreshes, 1);
    });

    testWidgets('deleting the last passkey removes the key', (tester) async {
      final v = await open(tester, [
        entry('e1', {
          'password': 'pw',
          'passkeys': [_passkey('AAAA')],
        }),
      ], '/entries/e1');
      await menu(tester, 'AAAA', 'Delete');
      await tester.tap(find.widgetWithText(TextButton, 'Delete'));
      await tester.pumpAndSettle();
      expect((await dataOf(tester, v.api, 'e1')).containsKey('passkeys'), isFalse);
      expect(find.text('PASSKEYS'), findsNothing);
    });

    testWidgets('move writes the target first, then removes it from the source',
        (tester) async {
      final record = _passkey('AAAA');
      final v = await open(tester, [
        entry('e1', {
          'username': 'alice',
          'password': 'pw',
          'passkeys': [record],
        }),
        entry('e2', {'username': 'alice@work', 'password': 'pw2'}, name: 'Work mail'),
        entry('e3', {'notes': 'n'}, type: 'note', name: 'A note'),
        entry('e4', {'username': 'x', 'password': 'y'}, name: 'Shared', permission: 'read'),
      ], '/entries/e1');

      await menu(tester, 'AAAA', 'Move to…');
      expect(find.text('Move passkey to…'), findsOneWidget);
      expect(find.text('Work mail'), findsOneWidget);
      expect(find.text('alice@work'), findsOneWidget);
      expect(find.byKey(const ValueKey('move-target-e1')), findsNothing);
      expect(find.byKey(const ValueKey('move-target-e3')), findsNothing, reason: 'not a login');
      expect(find.byKey(const ValueKey('move-target-e4')), findsNothing, reason: 'read-only');
      await tester.tap(find.text('Work mail'));
      await tester.pumpAndSettle();

      expect(v.api.updates.map((u) => u.id), ['e2', 'e1']);
      expect((await dataOf(tester, v.api, 'e2'))['passkeys'], [record]);
      expect((await dataOf(tester, v.api, 'e1')).containsKey('passkeys'), isFalse);
      expect(find.text('Passkey moved to Work mail'), findsOneWidget);
    });

    testWidgets('a failed move leaves the source alone', (tester) async {
      final v = await open(tester, [
        entry('e1', {
          'password': 'pw',
          'passkeys': [_passkey('AAAA')],
        }),
        entry('e2', {'username': 'bob', 'password': 'pw2'}, name: 'Other'),
      ], '/entries/e1');
      v.api.failUpdatesOf.add('e2');

      await menu(tester, 'AAAA', 'Move to…');
      await tester.tap(find.text('Other'));
      await tester.pumpAndSettle();

      expect(v.api.updates, isEmpty);
      expect((await dataOf(tester, v.api, 'e1'))['passkeys'], hasLength(1));
      expect(find.textContaining('Failed'), findsOneWidget);
    });

    group('add by hand', () {
      Future<void> fill(WidgetTester tester, {required String key}) async {
        await tester.tap(find.byKey(const ValueKey('add-passkey')));
        await tester.pumpAndSettle();
        await tester.enterText(find.byKey(const ValueKey('passkey-user-handle')), 'dXNlci0xMjM0==');
        await tester.enterText(
            find.byKey(const ValueKey('passkey-credential-id')), 'q2D0bQ2u3E7XoWlS1yZbVw==');
        await tester.enterText(find.byKey(const ValueKey('passkey-private-key')), key);
        await tester.tap(find.byKey(const ValueKey('passkey-save')));
        await tester.pumpAndSettle();
      }

      String fieldText(WidgetTester tester, String key) => tester
          .widget<TextFormField>(find.byKey(ValueKey(key)))
          .controller!
          .text;

      testWidgets('prefilled from the entry; a valid key is stored in the long form',
          (tester) async {
        final v = await open(tester, [
          entry('e1', {'username': 'alice', 'password': 'pw', 'notes': 'n'}),
        ], '/entries/e1');
        await tester.tap(find.byKey(const ValueKey('add-passkey')));
        await tester.pumpAndSettle();
        expect(fieldText(tester, 'passkey-rp-id'), 'mail.example.com');
        expect(fieldText(tester, 'passkey-user-name'), 'alice');
        await tester.tap(find.text('Cancel'));
        await tester.pumpAndSettle();

        await fill(tester, key: p256Sec1Pem);

        final data = await dataOf(tester, v.api, 'e1');
        expect(data['notes'], 'n');
        final p = (data['passkeys'] as List).single as Map;
        expect(p['credential_id'], 'q2D0bQ2u3E7XoWlS1yZbVw');
        expect(p['user_handle'], 'dXNlci0xMjM0');
        expect(p['rp_id'], 'mail.example.com');
        expect(p['user_name'], 'alice');
        expect(p['private_key'], p256Long);
        expect(p['key_algorithm'], -7);
        expect(p['sign_count'], 0);
        expect(p['discoverable'], true);
        expect(p.containsKey('prf_secret'), isFalse, reason: 'only a new key pair gets one');
        expect(DateTime.parse(p['created_at'] as String).isUtc, isTrue);
        expect(find.text('Passkey added'), findsOneWidget);
        expect(find.text('mail.example.com'), findsOneWidget);
        expect(v.auth.autofillRefreshes, 1);
      });

      testWidgets('a wrong curve or broken parts are explained and nothing is saved',
          (tester) async {
        final v = await open(tester, [
          entry('e1', {'username': 'alice', 'password': 'pw'}),
        ], '/entries/e1');
        await fill(tester, key: p384Pem);
        expect(find.text('This key uses P-384. Passkeys need a P-256 key.'), findsOneWidget);
        expect(v.api.updates, isEmpty);

        await tester.enterText(find.byKey(const ValueKey('passkey-private-key')), p256Long);
        await tester.enterText(find.byKey(const ValueKey('passkey-user-handle')), 'not base64!');
        await tester.enterText(find.byKey(const ValueKey('passkey-rp-id')), 'not a domain');
        await tester.tap(find.byKey(const ValueKey('passkey-save')));
        await tester.pumpAndSettle();
        expect(find.text('Not base64 or base64url'), findsOneWidget);
        expect(find.text("Enter the site's domain, e.g. example.com"), findsOneWidget);
        expect(v.api.updates, isEmpty);
      });
    });
  }, skip: nativeCryptoBuilt ? false : 'native crypto lib not built ($nativeCryptoLib)');

  group('edit screen', () {
    testWidgets('a login with passkeys saves without a password and keeps them',
        (tester) async {
      final passkeys = [_passkey('AAAA'), _passkey('BBBB')];
      final v = await open(tester, [
        entry('e1', {'username': 'alice', 'passkeys': passkeys}),
      ], '/entries/e1/edit');
      expect(find.text('2 passkeys · manage them on the entry page'), findsOneWidget);

      await tester.enterText(find.widgetWithText(TextFormField, 'Username'), 'alice2');
      await tester.tap(find.text('SAVE CHANGES'));
      await tester.pumpAndSettle();

      expect(find.text('Required'), findsNothing);
      expect(find.text('LIST'), findsOneWidget);
      final data = await dataOf(tester, v.api, 'e1');
      expect(data['username'], 'alice2');
      expect(data['passkeys'], passkeys);
      expect(data.containsKey('password'), isFalse);
    });

    testWidgets('one passkey reads in the singular', (tester) async {
      await open(tester, [
        entry('e1', {'password': 'pw', 'passkeys': [_passkey('AAAA')]}),
      ], '/entries/e1/edit');
      expect(find.text('1 passkey · manage it on the entry page'), findsOneWidget);
    });

    testWidgets('without passkeys the password stays required', (tester) async {
      final v = await open(tester, [
        entry('e1', {'username': 'alice'}),
      ], '/entries/e1/edit');
      expect(find.textContaining('passkey'), findsNothing);
      await tester.tap(find.text('SAVE CHANGES'));
      await tester.pumpAndSettle();
      expect(find.text('Required'), findsOneWidget);
      expect(v.api.updates, isEmpty);
    });
  }, skip: nativeCryptoBuilt ? false : 'native crypto lib not built ($nativeCryptoLib)');

  group('server entries', () {
    const key = '-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaA==\n-----END OPENSSH PRIVATE KEY-----\n';

    test('the SSH command leaves out port 22 and a missing user', () {
      expect(sshCommand('box.example.com', port: '2222', user: 'root'),
          'ssh -p 2222 root@box.example.com');
      expect(sshCommand('box.example.com', port: '22', user: 'root'), 'ssh root@box.example.com');
      expect(sshCommand(' box ', port: '', user: ''), 'ssh box');
      expect(sshCommand('10.0.0.1'), 'ssh 10.0.0.1');
    });

    testWidgets('create: the fields reach the data, the key exactly as typed', (tester) async {
      final v = await open(tester, const [], '/entries/new?type=server');
      expect(find.widgetWithText(ChoiceChip, 'Server'), findsOneWidget);

      await tester.enterText(find.widgetWithText(TextFormField, 'Name *'), 'Box');
      await tester.tap(find.text('SAVE ENTRY'));
      await tester.pumpAndSettle();
      expect(find.text('Required'), findsOneWidget, reason: 'the host is required');

      await tester.enterText(find.widgetWithText(TextFormField, 'Host'), ' box.example.com ');
      await tester.enterText(find.widgetWithText(TextFormField, 'Port'), '99999');
      await tester.tap(find.text('SAVE ENTRY'));
      await tester.pumpAndSettle();
      expect(find.text('Port 1–65535'), findsOneWidget);
      expect(v.api.created, isEmpty);

      await tester.enterText(find.widgetWithText(TextFormField, 'Port'), '2222');
      await tester.enterText(find.widgetWithText(TextFormField, 'Username'), 'root');
      await tester.enterText(find.widgetWithText(TextFormField, 'Password'), ' pw ');
      await tester.enterText(find.widgetWithText(TextFormField, 'Private Key'), key);
      await tester.enterText(find.widgetWithText(TextFormField, 'Key Passphrase'), ' phrase ');
      await tester.tap(find.text('SAVE ENTRY'));
      await tester.pumpAndSettle();

      final req = v.api.created.single;
      expect(req.type, 'server');
      expect(req.name, 'Box');
      expect(await dataOf(tester, v.api, 'new-1'), {
        'name': 'Box',
        'host': 'box.example.com',
        'port': '2222',
        'username': 'root',
        'password': ' pw ',
        'private_key': key,
        'key_passphrase': ' phrase ',
      });
    });

    testWidgets('edit keeps the private key with its final newline', (tester) async {
      final v = await open(tester, [
        entry('s1', {'host': 'box', 'username': 'root', 'private_key': key},
            type: 'server', name: 'Box'),
      ], '/entries/s1/edit');
      expect(find.text('Server'), findsOneWidget);
      await tester.tap(find.text('SAVE CHANGES'));
      await tester.pumpAndSettle();
      final data = await dataOf(tester, v.api, 's1');
      expect(data['private_key'], key);
      expect(data['host'], 'box');
    });

    testWidgets('detail: host, port, user, SSH command and masked secrets', (tester) async {
      await open(tester, [
        entry('s1', {
          'host': 'box.example.com',
          'port': '2222',
          'username': 'root',
          'password': 'pw',
          'private_key': key,
          'key_passphrase': 'phrase',
          'notes': 'rack 4',
        }, type: 'server', name: 'Box'),
      ], '/entries/s1');

      for (final label in ['Host', 'Port', 'User', 'SSH command', 'Password', 'Private key', 'Passphrase']) {
        expect(find.text(label), findsOneWidget, reason: label);
      }
      expect(find.text('ssh -p 2222 root@box.example.com'), findsOneWidget);
      expect(find.text('rack 4'), findsOneWidget);
      expect(find.textContaining('OPENSSH'), findsNothing, reason: 'masked until revealed');
      expect(find.text('phrase'), findsNothing);
      expect(find.text('PASSKEYS'), findsNothing);

      String? copied;
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform,
          (call) async {
        if (call.method == 'Clipboard.setData') copied = (call.arguments as Map)['text'] as String;
        return null;
      });
      addTearDown(() => tester.binding.defaultBinaryMessenger
          .setMockMethodCallHandler(SystemChannels.platform, null));
      await tester.tap(find.descendant(
          of: find.widgetWithText(ListTile, 'SSH command'), matching: find.byIcon(Icons.copy)));
      await tester.pump();
      expect(copied, 'ssh -p 2222 root@box.example.com');
    });

    testWidgets('detail: port 22 and no user give a plain command, no port row',
        (tester) async {
      await open(tester, [
        entry('s1', {'host': 'box', 'port': '22'}, type: 'server', name: 'Box'),
      ], '/entries/s1');
      expect(find.text('ssh box'), findsOneWidget);
      expect(find.text('User'), findsNothing);

      await tester.pumpWidget(const SizedBox());
      await open(tester, [
        entry('s2', {'host': 'box', 'username': 'u'}, type: 'server', name: 'Box'),
      ], '/entries/s2');
      expect(find.text('Port'), findsNothing);
      expect(find.text('ssh u@box'), findsOneWidget);

      // Another client may write the port as a number.
      await tester.pumpWidget(const SizedBox());
      final v = await open(tester, [
        entry('s3', {'host': 'box', 'port': 2200}, type: 'server', name: 'Box'),
      ], '/entries/s3');
      expect(find.text('ssh -p 2200 box'), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
      await tester.pumpWidget(_app(v.api, v.auth, '/entries/s3/edit'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('SAVE CHANGES'));
      await tester.pumpAndSettle();
      expect((await dataOf(tester, v.api, 's3'))['port'], '2200');
    });
  }, skip: nativeCryptoBuilt ? false : 'native crypto lib not built ($nativeCryptoLib)');

  test('EntryResponse type stays as the server sends it', () {
    expect(EntryResponse.fromJson({'id': 'x', 'name': 'n', 'type': 'server'}).type, 'server');
  });
}

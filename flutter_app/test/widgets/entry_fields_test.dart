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

// Widget tests for the entry detail view (selectable plaintext, masked
// secrets), the edit screen (drag & drop order of custom fields) and entry
// tags (edit, detail chips, list filter). The screens decrypt/encrypt for
// real, so the data-key wrapping needs the native hybrid-KEM lib
// (`make native-crypto`, built in CI); without it the tests skip.
@TestOn('vm')
library;

import 'dart:convert';
import 'dart:io';

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:passbubble/core/api/api_client.dart';
import 'package:passbubble/core/api/models.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/core/crypto/ml_kem.dart';
import 'package:passbubble/core/crypto/vault_crypto.dart';
import 'package:passbubble/features/entries/add_edit_screen.dart';
import 'package:passbubble/features/entries/entries_list_screen.dart';
import 'package:passbubble/features/entries/entry_detail_screen.dart';

const _libPath = 'native/passbubble_crypto/build/libpassbubble_crypto.so';

/// A test account's key material.
class _Keys {
  final Uint8List privX;
  final Uint8List privM;
  final String pubXB64;
  final String pubMB64;
  _Keys(this.privX, this.privM, this.pubXB64, this.pubMB64);

  static Future<_Keys> generate() async {
    final x = await VaultCrypto.generateX25519KeyPair();
    final privX = Uint8List.fromList(await x.extractPrivateKeyBytes());
    final pubX = Uint8List.fromList((await x.extractPublicKey()).bytes);
    final (privM, pubM) = await mlKemGenerate();
    return _Keys(privX, privM, base64.encode(pubX), base64.encode(pubM));
  }

  Future<EntryResponse> encryptEntry(
    Map<String, dynamic> data, {
    String type = 'password',
    String id = 'e1',
    String name = 'Mail',
  }) async {
    final enc = await VaultCrypto.encryptEntryData(data);
    final wrapped = await VaultCrypto.encryptDataKey(enc.dataKey, pubXB64, pubMB64);
    return EntryResponse(
      id: id,
      type: type,
      name: name,
      url: 'https://mail.example.com',
      encryptedData: enc.encryptedData,
      dataNonce: enc.dataNonce,
      entryKey: EntryKey(userId: 'u1', encryptedKey: wrapped),
      permission: 'owner',
      createdAt: '',
      updatedAt: '',
    );
  }

  Future<Map<String, dynamic>> decrypt(String encryptedData, String wrappedKey) async {
    final dataKey = await VaultCrypto.decryptDataKey(wrappedKey, privX, privM);
    return VaultCrypto.decryptEntryData(encryptedData, dataKey);
  }
}

/// An unlocked vault without touching secure storage.
class _FakeAuth extends AuthService {
  final _Keys keys;
  _FakeAuth(super.api, this.keys);

  @override
  Uint8List? get privX25519 => keys.privX;
  @override
  Uint8List? get privMLKEM => keys.privM;
  @override
  Future<String?> getPubX25519() async => keys.pubXB64;
  @override
  Future<String?> getPubMlkem768() async => keys.pubMB64;
  @override
  Future<String?> getUserId() async => 'u1';
  @override
  Future<void> refreshAutofill() async {}
}

/// A locked vault: no private keys in memory.
class _LockedAuth extends _FakeAuth {
  _LockedAuth(super.api, super.keys);

  @override
  Uint8List? get privX25519 => null;
  @override
  Uint8List? get privMLKEM => null;
}

class _FakeApi extends ApiClient {
  final EntryResponse entry;

  /// The whole vault, for the list and the decrypted search/tag index.
  final List<EntryResponse> vault;
  UpdateEntryRequest? updated;
  _FakeApi(this.entry, {this.vault = const []});

  @override
  Future<EntryResponse> getEntry(String id) async => entry;
  @override
  Future<void> updateEntry(String id, UpdateEntryRequest req) async => updated = req;
  @override
  Future<List<FolderResponse>> listFolders() async => const [];
  @override
  Future<List<EntryResponse>> listEntries() async => vault;
  @override
  Future<List<EntryResponse>> listEntriesFull() async => vault;
}

// The list only loads for a logged-in session.
class _LoggedInAuth extends AuthNotifier {
  _LoggedInAuth(super.svc) {
    state = const AuthState(isLoggedIn: true, isUnlocked: true, userId: 'u1');
  }
}

Widget _app(_FakeApi api, _Keys keys, String location, {bool locked = false}) {
  final router = GoRouter(
    initialLocation: location,
    routes: [
      GoRoute(
        path: '/entries',
        builder: (_, s) => Scaffold(
          body: Text(switch (s.uri.queryParameters['tag']) {
            final tag? => 'LIST #$tag',
            null => 'LIST',
          }),
        ),
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
      authServiceProvider
          .overrideWithValue(locked ? _LockedAuth(api, keys) : _FakeAuth(api, keys)),
    ],
    child: MaterialApp.router(routerConfig: router),
  );
}

/// [app] on a logged-in, unlocked session (the entry list loads only then).
Widget _loggedIn(_FakeApi api, _Keys keys, Widget app) {
  final auth = _FakeAuth(api, keys);
  return ProviderScope(
    overrides: [
      apiClientProvider.overrideWithValue(api),
      authServiceProvider.overrideWithValue(auth),
      authStateProvider.overrideWith((ref) => _LoggedInAuth(auth)),
    ],
    child: app,
  );
}

/// The real entry list.
Widget _listApp(_FakeApi api, _Keys keys, {String? initialTag}) => _loggedIn(
    api, keys, MaterialApp(home: EntriesListScreen(initialTag: initialTag)));

void _useTallView(WidgetTester tester) {
  tester.view.physicalSize = const Size(1000, 2400);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
}

/// Texts of every SelectableText on screen (i.e. what the user can select).
List<String> _selectableTexts(WidgetTester tester) => tester
    .widgetList<SelectableText>(find.byType(SelectableText))
    .map((w) => w.data ?? '')
    .toList();

void main() {
  final libExists = File(_libPath).existsSync();
  late _Keys keys;

  setUpAll(() async {
    if (!libExists) return;
    mlKemLibraryPathOverride = File(_libPath).absolute.path;
    keys = await _Keys.generate();
  });

  group('entry detail', () {
    const password = 'hunter2-secret';
    const pin = '4711';
    const backup = 'backup-code-9';
    late EntryResponse entry;

    setUpAll(() async {
      if (!libExists) return;
      entry = await keys.encryptEntry({
        'username': 'alice@example.com',
        'password': password,
        'notes': 'line one\nline two',
        'custom_fields': [
          {'label': 'Recovery', 'value': 'ask the cat', 'type': 'text'},
          {'label': 'PIN', 'value': pin, 'type': 'password'},
          {'label': 'Backup', 'value': backup, 'type': 'password'},
        ],
      });
    });

    Future<void> openDecrypted(WidgetTester tester) async {
      _useTallView(tester);
      await tester.pumpWidget(_app(_FakeApi(entry), keys, '/entries/e1'));
      // The unlocked vault decrypts on open, no tap on DECRYPT needed.
      await tester.pumpAndSettle();
      expect(find.text('DECRYPT'), findsNothing);
    }

    testWidgets('a locked vault does not decrypt on open and keeps DECRYPT',
        (tester) async {
      _useTallView(tester);
      await tester.pumpWidget(_app(_FakeApi(entry), keys, '/entries/e1', locked: true));
      await tester.pumpAndSettle();
      expect(find.text('DECRYPT'), findsOneWidget);
      expect(find.text('alice@example.com'), findsNothing);
      expect(find.byType(SnackBar), findsNothing);
    });

    testWidgets('plaintext fields are selectable, masked secrets are not',
        (tester) async {
      await openDecrypted(tester);

      final selectable = _selectableTexts(tester);
      expect(selectable, containsAll([
        'https://mail.example.com', // URL
        'alice@example.com', // username
        'line one\nline two', // multi-line notes
        'ask the cat', // custom text field
      ]));

      // Secrets render as plain, non-selectable bullets while masked: nothing
      // selectable carries the real value (or even the mask), and no
      // SelectionArea wraps the mask either.
      for (final secret in [password, pin, backup]) {
        expect(find.text(secret), findsNothing);
        final masked = find.text('•' * secret.length);
        expect(masked, findsOneWidget);
        expect(tester.widget(masked), isA<Text>());
        expect(find.ancestor(of: masked, matching: find.byType(SelectableRegion)),
            findsNothing);
      }
      expect(
          selectable.where(
              (t) => t.contains(password) || t.contains(pin) || t.contains(backup)),
          isEmpty);
      expect(selectable.where((t) => t.contains('•')), isEmpty);

      // Each eye reveals only its own row, also between custom fields; once
      // revealed, a secret is selectable like any other value.
      Finder eye(String label, IconData icon) => find.descendant(
          of: find.ancestor(of: find.text(label), matching: find.byType(ListTile)),
          matching: find.byIcon(icon));

      await tester.tap(eye('PIN', Icons.visibility));
      await tester.pump();
      expect(_selectableTexts(tester), contains(pin));
      for (final secret in [password, backup]) {
        expect(find.text(secret), findsNothing);
        expect(find.text('•' * secret.length), findsOneWidget);
      }

      await tester.tap(eye('Password', Icons.visibility));
      await tester.tap(eye('Backup', Icons.visibility));
      await tester.pump();
      expect(_selectableTexts(tester), containsAll([password, pin, backup]));

      // Hiding one row leaves the others revealed.
      await tester.tap(eye('PIN', Icons.visibility_off));
      await tester.pump();
      expect(find.text(pin), findsNothing);
      expect(_selectableTexts(tester), containsAll([password, backup]));
    });

    testWidgets('copy buttons still copy next to selectable text', (tester) async {
      final copied = <String>[];
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        (call) async {
          if (call.method == 'Clipboard.setData') {
            copied.add((call.arguments as Map)['text'] as String);
          }
          return null;
        },
      );
      addTearDown(() => tester.binding.defaultBinaryMessenger
          .setMockMethodCallHandler(SystemChannels.platform, null));

      await openDecrypted(tester);

      final usernameRow = find.ancestor(
          of: find.text('Username'), matching: find.byType(ListTile));
      await tester.tap(find.descendant(of: usernameRow, matching: find.byIcon(Icons.copy)));
      await tester.pump();
      expect(copied, ['alice@example.com']);
      expect(find.text('Username copied'), findsOneWidget);

      // The masked password copies the real value without revealing it.
      final passwordRow = find.ancestor(
          of: find.text('Password'), matching: find.byType(ListTile));
      await tester.tap(find.descendant(of: passwordRow, matching: find.byIcon(Icons.copy)));
      await tester.pump();
      expect(copied.last, password);
      expect(find.text(password), findsNothing);
    });

    testWidgets('selecting a value by drag yields its text', (tester) async {
      await openDecrypted(tester);

      final notes = find.byWidgetPredicate(
          (w) => w is SelectableText && w.data == 'line one\nline two');
      final editable = find.descendant(of: notes, matching: find.byType(EditableText));
      final rect = tester.getRect(editable);
      final gesture = await tester.startGesture(rect.topLeft + const Offset(1, 5),
          kind: PointerDeviceKind.mouse);
      await tester.pump();
      await gesture.moveTo(rect.bottomRight - const Offset(1, 5));
      await tester.pump();
      await gesture.up();
      await tester.pumpAndSettle();

      final value = tester.widget<EditableText>(editable).controller.value;
      expect(value.selection.isCollapsed, isFalse);
      expect(value.selection.textInside(value.text), contains('line'));
    });

    testWidgets('long-press selects a value on mobile', (tester) async {
      await openDecrypted(tester);

      final username = find.byWidgetPredicate(
          (w) => w is SelectableText && w.data == 'alice@example.com');
      final editable = find.descendant(of: username, matching: find.byType(EditableText));
      // On the text itself (the field is wider than the short value).
      await tester.longPressAt(tester.getTopLeft(editable) + const Offset(12, 8));
      await tester.pumpAndSettle();

      final value = tester.widget<EditableText>(editable).controller.value;
      expect(value.selection.isCollapsed, isFalse);
      expect(find.text('Copy'), findsOneWidget); // selection toolbar
    }, variant: TargetPlatformVariant.only(TargetPlatform.android));
  }, skip: libExists ? false : 'native crypto lib not built ($_libPath)');

  group('entry edit', () {
    testWidgets('custom fields reorder by drag handle and save in that order',
        (tester) async {
      _useTallView(tester);
      final entry = await tester.runAsync(() => keys.encryptEntry({
            'username': 'alice',
            'password': 'pw',
            'issuer': 'ACME', // managed by another client → must survive
            'custom_fields': [
              {'label': 'Alpha', 'value': 'a', 'type': 'text'},
              {'label': 'Beta', 'value': 'b', 'type': 'password'},
              {'label': 'Gamma', 'value': 'g', 'type': 'text', 'id': 'cf-3'},
            ],
          }));
      final api = _FakeApi(entry!);
      await tester.pumpWidget(_app(api, keys, '/entries/e1/edit'));
      await tester.pumpAndSettle();

      List<String> labelOrder() {
        final labels = ['Alpha', 'Beta', 'Gamma'];
        final ys = {for (final l in labels) l: tester.getCenter(find.text(l)).dy};
        return labels..sort((a, b) => ys[a]!.compareTo(ys[b]!));
      }

      expect(labelOrder(), ['Alpha', 'Beta', 'Gamma']);
      final handles = find.byIcon(Icons.drag_indicator);
      expect(handles, findsNWidgets(3));

      // Drag Gamma's handle above Alpha.
      final from = tester.getCenter(handles.at(2));
      final to = tester.getCenter(handles.at(0)) - const Offset(0, 40);
      final gesture = await tester.startGesture(from);
      await tester.pump();
      for (var i = 1; i <= 10; i++) {
        await gesture.moveTo(Offset.lerp(from, to, i / 10)!);
        await tester.pump(const Duration(milliseconds: 16));
      }
      await gesture.up();
      await tester.pumpAndSettle();

      expect(labelOrder(), ['Gamma', 'Alpha', 'Beta']);

      // The label fields stay editable (the drag starts on the handle only).
      await tester.enterText(find.widgetWithText(TextFormField, 'Alpha'), 'Alpha 2');
      await tester.pump();

      await tester.tap(find.text('SAVE CHANGES'));
      await tester.pumpAndSettle();

      final req = api.updated;
      expect(req, isNotNull);
      final saved = (await tester.runAsync(
          () => keys.decrypt(req!.encryptedData!, req.entryKeys!.single.encryptedKey)))!;
      final cfs = (saved['custom_fields'] as List).cast<Map<String, dynamic>>();
      expect(cfs.map((c) => c['label']), ['Gamma', 'Alpha 2', 'Beta']);
      expect(cfs.map((c) => c['value']), ['g', 'a', 'b']);
      expect(cfs[0]['id'], 'cf-3', reason: 'unknown per-field keys are kept');
      expect(cfs[2]['type'], 'password');
      expect(saved['issuer'], 'ACME', reason: 'unknown entry keys are kept');
      expect(find.text('LIST'), findsOneWidget); // navigated back after save
    });
  }, skip: libExists ? false : 'native crypto lib not built ($_libPath)');

  group('entry tags', () {
    const tooLong = 'Tags can be at most 40 characters';
    Finder tagChip(String tag) => find.widgetWithText(InputChip, tag);
    final tagInput = find.byKey(const ValueKey('tag-input'));

    Future<Map<String, dynamic>> savedData(WidgetTester tester, _FakeApi api) async {
      final req = api.updated!;
      return (await tester.runAsync(
          () => keys.decrypt(req.encryptedData!, req.entryKeys!.single.encryptedKey)))!;
    }

    testWidgets('edit screen adds and removes tags and keeps unknown keys',
        (tester) async {
      _useTallView(tester);
      final entry = (await tester.runAsync(() => keys.encryptEntry({
            'username': 'alice',
            'password': 'pw',
            'tags': ['Work', 42, '  #client   a '],
            'issuer': 'ACME', // managed by another client → must survive
            'custom_fields': [
              {'label': 'Alpha', 'value': 'a', 'type': 'text', 'id': 'cf-1'},
            ],
          })))!;
      final other = (await tester.runAsync(() => keys.encryptEntry(
          {'username': 'bob', 'tags': ['Team']}, id: 'e2', name: 'Other')))!;
      final api = _FakeApi(entry, vault: [entry, other]);
      await tester.pumpWidget(_app(api, keys, '/entries/e1/edit'));
      await tester.pumpAndSettle();

      // Stored tags show normalised; the non-string item is ignored.
      expect(find.byType(InputChip), findsNWidgets(2));
      expect(tagChip('Work'), findsOneWidget);
      expect(tagChip('client a'), findsOneWidget);

      // Each chip has its own remove button.
      await tester.tap(find.descendant(of: tagChip('Work'), matching: find.byIcon(Icons.close)));
      await tester.pump();
      expect(tagChip('Work'), findsNothing);

      // Enter adds a tag (leading # stripped) and clears the field.
      await tester.enterText(tagInput, '#home');
      await tester.testTextInput.receiveAction(TextInputAction.done);
      await tester.pump();
      expect(tagChip('home'), findsOneWidget);
      expect(tester.widget<TextField>(tagInput).controller!.text, isEmpty);

      // A comma ends a tag, several at once too; a case-insensitive duplicate
      // is not added again.
      await tester.enterText(tagInput, 'x, HOME,y,');
      await tester.pump();
      expect(tagChip('x'), findsOneWidget);
      expect(tagChip('y'), findsOneWidget);
      expect(tagChip('HOME'), findsNothing);
      expect(tester.widget<TextField>(tagInput).controller!.text, isEmpty);

      // Over-long tags are refused with a message, not silently dropped.
      await tester.enterText(tagInput, 'a' * 41);
      await tester.pump();
      expect(find.text(tooLong), findsOneWidget);
      await tester.testTextInput.receiveAction(TextInputAction.done);
      await tester.pump();
      expect(tagChip('a' * 41), findsNothing);
      expect(find.text(tooLong), findsOneWidget);

      // Typing suggests vault tags; tapping one adds it.
      await tester.enterText(tagInput, 'te');
      await tester.pump();
      expect(find.text(tooLong), findsNothing);
      await tester.tap(find.widgetWithText(ActionChip, 'Team'));
      await tester.pump();
      expect(tagChip('Team'), findsOneWidget);
      expect(find.widgetWithText(ActionChip, 'Team'), findsNothing);

      // A tag the vault already has keeps the vault's spelling.
      await tester.enterText(tagInput, 'WORK');
      await tester.testTextInput.receiveAction(TextInputAction.done);
      await tester.pump();
      expect(tagChip('Work'), findsOneWidget);

      // Text left in the field is saved as a tag too.
      await tester.enterText(tagInput, 'pending');
      await tester.pump();
      await tester.tap(find.text('SAVE CHANGES'));
      await tester.pumpAndSettle();

      final saved = await savedData(tester, api);
      expect(saved['tags'], ['client a', 'home', 'x', 'y', 'Team', 'Work', 'pending']);
      expect(saved['issuer'], 'ACME', reason: 'unknown entry keys are kept');
      expect(saved['username'], 'alice');
      final cfs = (saved['custom_fields'] as List).cast<Map<String, dynamic>>();
      expect(cfs.single['id'], 'cf-1', reason: 'unknown per-field keys are kept');
      expect(find.text('LIST'), findsOneWidget);
    });

    testWidgets('an over-long tag in the field blocks the save', (tester) async {
      _useTallView(tester);
      final entry = (await tester.runAsync(() => keys.encryptEntry({'password': 'pw'})))!;
      final api = _FakeApi(entry, vault: [entry]);
      await tester.pumpWidget(_app(api, keys, '/entries/e1/edit'));
      await tester.pumpAndSettle();

      await tester.enterText(tagInput, 'b' * 41);
      await tester.tap(find.text('SAVE CHANGES'));
      await tester.pumpAndSettle();
      expect(api.updated, isNull);
      expect(find.text(tooLong), findsOneWidget);
    });

    testWidgets('removing every tag omits the key and keeps the rest',
        (tester) async {
      _useTallView(tester);
      final entry = (await tester.runAsync(() => keys.encryptEntry({
            'password': 'pw',
            'tags': ['solo'],
            'issuer': 'ACME',
            'custom_fields': [
              {'label': 'A', 'value': 'a', 'type': 'text', 'id': 'cf-1', 'color': 'red'},
            ],
          })))!;
      final api = _FakeApi(entry, vault: [entry]);
      await tester.pumpWidget(_app(api, keys, '/entries/e1/edit'));
      await tester.pumpAndSettle();

      await tester.tap(find.byTooltip('Remove tag'));
      await tester.pump();
      expect(find.byType(InputChip), findsNothing);
      await tester.tap(find.text('SAVE CHANGES'));
      await tester.pumpAndSettle();

      final saved = await savedData(tester, api);
      expect(saved.containsKey('tags'), isFalse);
      expect(saved['issuer'], 'ACME');
      expect(saved['password'], 'pw');
      final cf = (saved['custom_fields'] as List).single as Map<String, dynamic>;
      expect(cf['id'], 'cf-1');
      expect(cf['color'], 'red');
    });

    testWidgets('detail shows tags as chips that open the filtered list',
        (tester) async {
      _useTallView(tester);
      final entry = (await tester.runAsync(
          () => keys.encryptEntry({'password': 'pw', 'tags': ['Work', 'client a']})))!;
      await tester.pumpWidget(_app(_FakeApi(entry), keys, '/entries/e1'));
      // Tags are part of the encrypted data; they show once the entry has
      // decrypted itself on open.
      await tester.pumpAndSettle();
      expect(find.widgetWithText(ActionChip, 'Work'), findsOneWidget);

      await tester.tap(find.widgetWithText(ActionChip, 'client a'));
      await tester.pumpAndSettle();
      expect(find.text('LIST #client a'), findsOneWidget);
    });

    group('list', () {
      late List<EntryResponse> vault;

      setUpAll(() async {
        if (!libExists) return;
        vault = [
          await keys.encryptEntry(
              {'username': 'alice', 'tags': ['Work', 'client']}, id: 'e1', name: 'Mail'),
          await keys.encryptEntry(
              {'username': 'bob', 'tags': ['work']}, id: 'e2', name: 'Bank'),
          await keys.encryptEntry({'username': 'carol'}, id: 'e3', name: 'Shop'),
        ];
      });

      List<String> shown() => [
            for (final name in ['Mail', 'Bank', 'Shop'])
              if (find.text(name).evaluate().isNotEmpty) name,
          ];
      Finder filterChip(String tag) => find.widgetWithText(FilterChip, tag);

      testWidgets('filter chips combine with AND', (tester) async {
        await tester.pumpWidget(_listApp(_FakeApi(vault.first, vault: vault), keys));
        await tester.pumpAndSettle();
        expect(shown(), ['Mail', 'Bank', 'Shop']);

        // Union of the vault's tags, first-seen spelling, sorted.
        expect(
            tester
                .widgetList<FilterChip>(find.byType(FilterChip))
                .map((c) => (c.label as Text).data),
            ['client', 'Work']);

        await tester.tap(filterChip('Work'));
        await tester.pumpAndSettle();
        expect(shown(), ['Mail', 'Bank']);
        expect(tester.widget<FilterChip>(filterChip('Work')).selected, isTrue);

        await tester.tap(filterChip('client'));
        await tester.pumpAndSettle();
        expect(shown(), ['Mail']);

        await tester.tap(filterChip('Work'));
        await tester.tap(filterChip('client'));
        await tester.pumpAndSettle();
        expect(shown(), ['Mail', 'Bank', 'Shop']);
      });

      testWidgets('#tag tokens filter, plain text also matches tags',
          (tester) async {
        await tester.pumpWidget(_listApp(_FakeApi(vault.first, vault: vault), keys));
        await tester.pumpAndSettle();
        final search = find.byType(TextField);

        Future<List<String>> searchFor(String q) async {
          await tester.enterText(search, q);
          await tester.pumpAndSettle();
          return shown();
        }

        expect(await searchFor('#WORK'), ['Mail', 'Bank']);
        expect(await searchFor('#work #client'), ['Mail']);
        expect(await searchFor('#work bank'), ['Bank']);
        expect(await searchFor('client'), ['Mail']);
        expect(await searchFor('#nope'), isEmpty);
        expect(find.text('No entries found'), findsOneWidget);

        // Chips and the search box combine.
        await searchFor('bank');
        await tester.tap(filterChip('client'));
        await tester.pumpAndSettle();
        expect(shown(), isEmpty);
      });

      testWidgets('opens filtered by the initial tag', (tester) async {
        await tester.pumpWidget(
            _listApp(_FakeApi(vault.first, vault: vault), keys, initialTag: 'CLIENT'));
        await tester.pumpAndSettle();
        expect(shown(), ['Mail']);
        expect(tester.widget<FilterChip>(filterChip('client')).selected, isTrue);
      });

      testWidgets('a tag chip on the detail page filters the list below it',
          (tester) async {
        _useTallView(tester);
        // Same nesting as the app router: the list page stays mounted below
        // the detail page and gets the tag as a new widget.
        final router = GoRouter(
          initialLocation: '/entries/e1',
          routes: [
            GoRoute(
              path: '/entries',
              builder: (_, s) =>
                  EntriesListScreen(initialTag: s.uri.queryParameters['tag']),
              routes: [
                GoRoute(
                  path: ':id',
                  builder: (_, s) => EntryDetailScreen(id: s.pathParameters['id']!),
                ),
              ],
            ),
          ],
        );
        await tester.pumpWidget(_loggedIn(_FakeApi(vault.first, vault: vault), keys,
            MaterialApp.router(routerConfig: router)));
        await tester.pumpAndSettle();

        await tester.tap(find.widgetWithText(ActionChip, 'client'));
        await tester.pumpAndSettle();
        expect(find.byType(EntryDetailScreen), findsNothing);
        expect(shown(), ['Mail']);
        expect(tester.widget<FilterChip>(filterChip('client')).selected, isTrue);
      });

      testWidgets('no chip row without tags', (tester) async {
        final untagged = [vault.last];
        await tester.pumpWidget(_listApp(_FakeApi(untagged.single, vault: untagged), keys));
        await tester.pumpAndSettle();
        expect(shown(), ['Shop']);
        expect(find.byKey(const ValueKey('tag-filter-bar')), findsNothing);
      });
    });
  }, skip: libExists ? false : 'native crypto lib not built ($_libPath)');
}

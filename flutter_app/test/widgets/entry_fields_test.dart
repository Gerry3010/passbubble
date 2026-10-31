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
// secrets), the edit screen (drag & drop order of custom fields, password
// generator dialog), entry tags (edit, detail chips, list filter) and the
// generator defaults and Generate tab. The entry screens decrypt/encrypt for
// real, so the data-key wrapping needs the native hybrid-KEM lib
// (`make native-crypto`, built in CI); without it those tests skip.
@TestOn('vm')
library;

import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart' show HttpClientAdapter, RequestOptions, ResponseBody;
import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:passbubble/core/api/api_client.dart';
import 'package:passbubble/core/api/models.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/core/crypto/ml_kem.dart';
import 'package:passbubble/core/crypto/vault_crypto.dart';
import 'package:passbubble/core/generator/password_generator.dart';
import 'package:passbubble/features/entries/add_edit_screen.dart';
import 'package:passbubble/features/entries/entries_list_screen.dart';
import 'package:passbubble/features/entries/entry_detail_screen.dart';
import 'package:passbubble/features/generate/generate_screen.dart';
import 'package:passbubble/features/settings/generator_settings_tile.dart';
import 'package:passbubble/shared/widgets/generator_dialog.dart';

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

/// Fails every request, and records it: the generator must not need the server.
class _NoServer implements HttpClientAdapter {
  final requests = <String>[];

  @override
  Future<ResponseBody> fetch(RequestOptions options,
      Stream<Uint8List>? requestStream, Future<void>? cancelFuture) async {
    requests.add(options.path);
    throw StateError('unexpected server call: ${options.path}');
  }

  @override
  void close({bool force = false}) {}
}

const _defaultsKey = 'generator_defaults';
final _generatorDialog = find.byType(GeneratorDialog);
final _shown = find.byKey(const ValueKey('generated-password'));

/// The password the open generator dialog shows.
String _shownPassword(WidgetTester tester) =>
    tester.widget<SelectableText>(_shown).data!;

/// The controller of the text field [field] (a TextFormField finder).
TextEditingController _ctrlOf(WidgetTester tester, Finder field) =>
    tester.widget<TextFormField>(field).controller!;

/// The text field using [ctrl]; stays valid while its text changes.
Finder _fieldWith(TextEditingController ctrl) =>
    find.byWidgetPredicate((w) => w is TextFormField && w.controller == ctrl);

Set<int> _cps(String s) => s.runes.toSet();

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

  group('entry password generator', () {
    late Map<String, String> store;
    final letters = _cps(lowercaseChars + uppercaseChars);

    setUp(() {
      // Stored defaults: 24 characters, only - and _ as symbols.
      store = {_defaultsKey: jsonEncode({'length': 24, 'symbol_set': '-_'})};
      FlutterSecureStorage.setMockInitialValues(store);
    });

    Future<Map<String, dynamic>> savedData(WidgetTester tester, _FakeApi api) async {
      final req = api.updated!;
      return (await tester.runAsync(
          () => keys.decrypt(req.encryptedData!, req.entryKeys!.single.encryptedKey)))!;
    }

    testWidgets('main password: Cancel keeps the field, OK writes the shown password',
        (tester) async {
      _useTallView(tester);
      final entry = (await tester.runAsync(
          () => keys.encryptEntry({'username': 'alice', 'password': 'old-pw'})))!;
      final api = _FakeApi(entry);
      await tester.pumpWidget(_app(api, keys, '/entries/e1/edit'));
      await tester.pumpAndSettle();

      final passwordField = find.widgetWithText(TextFormField, 'Password');
      final ctrl = _ctrlOf(tester, passwordField);
      final generate = find.descendant(
          of: _fieldWith(ctrl), matching: find.byTooltip('Generate'));
      expect(ctrl.text, 'old-pw');

      // Opens with the stored defaults; Cancel leaves the field alone.
      await tester.tap(generate);
      await tester.pumpAndSettle();
      expect(_generatorDialog, findsOneWidget);
      final first = _shownPassword(tester);
      expect(first.runes.length, 24);
      expect(_cps(first).difference({...letters, ..._cps(digitChars), ..._cps('-_')}),
          isEmpty);
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();
      expect(_generatorDialog, findsNothing);
      expect(ctrl.text, 'old-pw');

      // Regenerate makes a new one, so does every option change.
      await tester.tap(generate);
      await tester.pumpAndSettle();
      final before = _shownPassword(tester);
      await tester.tap(find.byTooltip('Regenerate'));
      await tester.pump();
      final regenerated = _shownPassword(tester);
      expect(regenerated, isNot(before));
      await tester.tap(find.byKey(const ValueKey('generator-symbols')));
      await tester.pump();
      await tester.tap(find.byKey(const ValueKey('generator-digits')));
      await tester.pump();
      final chosen = _shownPassword(tester);
      expect(chosen, isNot(regenerated));
      expect(_cps(chosen).difference(letters), isEmpty, reason: chosen);
      expect(ctrl.text, 'old-pw', reason: 'nothing is written before OK');

      await tester.tap(find.text('OK'));
      await tester.pumpAndSettle();
      expect(_generatorDialog, findsNothing);
      expect(ctrl.text, chosen);

      // The option changes applied to that dialog only.
      expect(jsonDecode(store[_defaultsKey]!), {'length': 24, 'symbol_set': '-_'});
      await tester.tap(generate);
      await tester.pumpAndSettle();
      expect(tester.widget<FilterChip>(find.byKey(const ValueKey('generator-symbols'))).selected,
          isTrue);
      expect(_cps(_shownPassword(tester)).intersection(_cps('-_')), isNotEmpty);
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();
      expect(ctrl.text, chosen);

      await tester.tap(find.text('SAVE CHANGES'));
      await tester.pumpAndSettle();
      final saved = await savedData(tester, api);
      expect(saved['password'], chosen);
      expect(saved['username'], 'alice');
    });

    testWidgets('custom password fields generate into their own field; SSH keys do not',
        (tester) async {
      _useTallView(tester);
      final entry = (await tester.runAsync(() => keys.encryptEntry({
            'password': 'main-pw',
            'custom_fields': [
              {'label': 'PIN', 'value': 'old-pin', 'type': 'password'},
              {'label': 'Deploy key', 'value': 'ssh-key-data', 'type': 'ssh'},
              {'label': 'Note', 'value': 'plain', 'type': 'text'},
            ],
          })))!;
      final api = _FakeApi(entry);
      await tester.pumpWidget(_app(api, keys, '/entries/e1/edit'));
      await tester.pumpAndSettle();

      final mainCtrl = _ctrlOf(tester, find.widgetWithText(TextFormField, 'Password'));
      final pinCtrl = _ctrlOf(tester, find.widgetWithText(TextFormField, 'old-pin'));
      final sshField = find.widgetWithText(TextFormField, 'ssh-key-data');
      final pinGenerate = find.descendant(
          of: _fieldWith(pinCtrl), matching: find.byTooltip('Generate'));

      // Main password + PIN; the SSH key keeps just its eye.
      expect(find.byTooltip('Generate'), findsNWidgets(2));
      expect(pinGenerate, findsOneWidget);
      expect(find.descendant(of: sshField, matching: find.byTooltip('Generate')),
          findsNothing);
      expect(find.descendant(of: sshField, matching: find.byIcon(Icons.visibility_off)),
          findsOneWidget);
      // Masked it is one line (TextField asserts that); revealed it gets four.
      expect(tester.widget<TextField>(
              find.descendant(of: sshField, matching: find.byType(TextField))).maxLines,
          1);
      await tester.tap(find.descendant(of: sshField, matching: find.byIcon(Icons.visibility_off)));
      await tester.pump();
      expect(tester.widget<TextField>(
              find.descendant(of: sshField, matching: find.byType(TextField))).maxLines,
          4);

      await tester.tap(pinGenerate);
      await tester.pumpAndSettle();
      expect(_shownPassword(tester).runes.length, 24);
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();
      expect(pinCtrl.text, 'old-pin');

      await tester.tap(pinGenerate);
      await tester.pumpAndSettle();
      await tester.tap(find.byTooltip('Regenerate'));
      await tester.pump();
      final chosen = _shownPassword(tester);
      await tester.tap(find.text('OK'));
      await tester.pumpAndSettle();
      expect(pinCtrl.text, chosen);
      expect(mainCtrl.text, 'main-pw', reason: 'the main password is not touched');

      await tester.tap(find.text('SAVE CHANGES'));
      await tester.pumpAndSettle();
      final saved = await savedData(tester, api);
      expect(saved['password'], 'main-pw');
      final cfs = (saved['custom_fields'] as List).cast<Map<String, dynamic>>();
      expect(cfs.map((c) => c['value']), [chosen, 'ssh-key-data', 'plain']);
    });
  }, skip: libExists ? false : 'native crypto lib not built ($_libPath)');

  // No vault involved, so these run without the native lib too.
  group('generator defaults and Generate tab', () {
    late Map<String, String> store;
    final symbolsChip = find.byKey(const ValueKey('generator-symbols'));
    final digitsChip = find.byKey(const ValueKey('generator-digits'));
    final symbolSet = find.byKey(const ValueKey('generator-symbol-set'));
    final error = find.byKey(const ValueKey('generator-error'));

    setUp(() {
      store = {};
      FlutterSecureStorage.setMockInitialValues(store);
    });

    /// The settings tile and a button that opens the generator dialog.
    Future<List<String?>> pumpSettings(WidgetTester tester) async {
      final results = <String?>[];
      _useTallView(tester);
      await tester.pumpWidget(ProviderScope(
        overrides: [
          apiClientProvider.overrideWithValue(ApiClient(httpClientAdapter: _NoServer())),
        ],
        child: MaterialApp(
          home: Scaffold(
            body: Column(children: [
              const GeneratorSettingsTile(),
              Consumer(
                builder: (context, ref, _) => TextButton(
                  onPressed: () async => results.add(await showGeneratorDialog(context, ref)),
                  child: const Text('OPEN GENERATOR'),
                ),
              ),
            ]),
          ),
        ),
      ));
      await tester.pumpAndSettle();
      return results;
    }

    Future<void> openSettings(WidgetTester tester) async {
      await tester.tap(find.byType(GeneratorSettingsTile));
      await tester.pumpAndSettle();
      expect(find.byType(AlertDialog), findsOneWidget);
    }

    Future<void> setLength(WidgetTester tester, int length) async {
      tester
          .widget<Slider>(find.byKey(const ValueKey('generator-length')))
          .onChanged!(length.toDouble());
      await tester.pump();
    }

    TextButton button(WidgetTester tester, String label) =>
        tester.widget<TextButton>(find.widgetWithText(TextButton, label));

    testWidgets('settings save and reset the defaults; the dialog starts from them',
        (tester) async {
      final results = await pumpSettings(tester);
      expect(find.text(describeGeneratorOptions(defaultGeneratorOptions)), findsOneWidget);

      // Change length, digits and the symbol set, then save.
      await openSettings(tester);
      expect(button(tester, 'Reset to defaults').onPressed, isNull,
          reason: 'nothing to reset yet');
      await setLength(tester, 32);
      await tester.tap(digitsChip);
      await tester.pump();
      await tester.enterText(symbolSet, ' -_ - ');
      await tester.pump();
      await tester.tap(find.text('Save'));
      await tester.pumpAndSettle();
      expect(find.byType(AlertDialog), findsNothing);

      const saved = GeneratorOptions(length: 32, digits: false, symbolSet: '-_');
      expect(jsonDecode(store[_defaultsKey]!), saved.toJson(),
          reason: 'stored with the symbol set cleaned');
      expect(find.text('32 characters · a-z A-Z symbols'), findsOneWidget);

      // The generator dialog starts from the new defaults.
      await tester.tap(find.text('OPEN GENERATOR'));
      await tester.pumpAndSettle();
      final pw = _shownPassword(tester);
      expect(pw.runes.length, 32);
      expect(_cps(pw).difference(_cps('$lowercaseChars$uppercaseChars-_')), isEmpty);
      expect(tester.widget<FilterChip>(digitsChip).selected, isFalse);
      await tester.tap(find.text('OK'));
      await tester.pumpAndSettle();
      expect(results, [pw]);

      // Cancel in the dialog returns nothing.
      await tester.tap(find.text('OPEN GENERATOR'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();
      expect(results, [pw, null]);

      // "Reset to defaults" resets the form; Cancel keeps the stored defaults.
      await openSettings(tester);
      expect(tester.widget<TextField>(symbolSet).controller!.text, '-_');
      await tester.tap(find.text('Reset to defaults'));
      await tester.pump();
      expect(tester.widget<TextField>(symbolSet).controller!.text, defaultSymbolSet);
      expect(tester.widget<FilterChip>(digitsChip).selected, isTrue);
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();
      expect(jsonDecode(store[_defaultsKey]!), saved.toJson());

      // Reset + Save goes back to the factory defaults (nothing stored).
      await openSettings(tester);
      await tester.tap(find.text('Reset to defaults'));
      await tester.pump();
      await tester.tap(find.text('Save'));
      await tester.pumpAndSettle();
      expect(store.containsKey(_defaultsKey), isFalse);
      expect(find.text(describeGeneratorOptions(defaultGeneratorOptions)), findsOneWidget);

      await tester.tap(find.text('OPEN GENERATOR'));
      await tester.pumpAndSettle();
      expect(_shownPassword(tester).runes.length, 20);
    });

    testWidgets('invalid options are explained and block OK and Save', (tester) async {
      await pumpSettings(tester);

      // Generator dialog: an empty symbol set with symbols on.
      await tester.tap(find.text('OPEN GENERATOR'));
      await tester.pumpAndSettle();
      await tester.enterText(symbolSet, '   ');
      await tester.pump();
      expect(tester.widget<Text>(error).data, contains('symbol'));
      expect(button(tester, 'OK').onPressed, isNull);
      expect(_shownPassword(tester), '—');
      // Symbols off: valid again.
      await tester.tap(symbolsChip);
      await tester.pump();
      expect(error, findsNothing);
      expect(button(tester, 'OK').onPressed, isNotNull);
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();

      // Settings: no character type at all.
      await openSettings(tester);
      for (final key in ['lowercase', 'uppercase', 'digits', 'symbols']) {
        await tester.tap(find.byKey(ValueKey('generator-$key')));
        await tester.pump();
      }
      expect(tester.widget<Text>(error).data, 'Select at least one character type');
      expect(button(tester, 'Save').onPressed, isNull);
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();
      expect(store, isEmpty);
    });

    testWidgets('the Generate tab generates locally from the defaults', (tester) async {
      store[_defaultsKey] = jsonEncode({'length': 30, 'symbol_set': '-_'});
      final server = _NoServer();
      _useTallView(tester);
      await tester.pumpWidget(ProviderScope(
        overrides: [
          apiClientProvider.overrideWithValue(ApiClient(httpClientAdapter: server)),
        ],
        child: const MaterialApp(home: GenerateScreen()),
      ));
      await tester.pumpAndSettle();

      List<String> shown() =>
          tester.widgetList<Text>(_shown).map((t) => t.data!).toList();
      final allowed = _cps('$lowercaseChars$uppercaseChars$digitChars-_');

      expect(shown(), hasLength(1));
      expect(shown().single.runes.length, 30);
      expect(_cps(shown().single).difference(allowed), isEmpty);

      await tester.tap(find.widgetWithText(ChoiceChip, '3'));
      await tester.pump();
      expect(shown(), hasLength(3));
      final before = shown();
      await tester.tap(find.text('REGENERATE'));
      await tester.pump();
      expect(shown(), hasLength(3));
      expect(shown(), isNot(before));

      // The four classes and the symbol set replace the old type chips.
      await tester.tap(symbolsChip);
      await tester.pump();
      for (final pw in shown()) {
        expect(_cps(pw).intersection(_cps('-_')), isEmpty, reason: pw);
      }
      await tester.tap(symbolsChip);
      await tester.pump();
      await tester.enterText(symbolSet, '€');
      await tester.pump();
      for (final pw in shown()) {
        expect(_cps(pw).difference(_cps('$lowercaseChars$uppercaseChars$digitChars€')),
            isEmpty);
        expect(pw, contains('€'));
      }

      // Nothing selected: no passwords, an explanation instead.
      for (final key in ['lowercase', 'uppercase', 'digits', 'symbols']) {
        await tester.tap(find.byKey(ValueKey('generator-$key')));
        await tester.pump();
      }
      expect(shown(), isEmpty);
      expect(error, findsOneWidget);

      // Changes here are not saved, and the server was never asked.
      expect(jsonDecode(store[_defaultsKey]!), {'length': 30, 'symbol_set': '-_'});
      expect(server.requests, isEmpty);
    });
  });

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

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

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:passbubble/core/api/api_client.dart';
import 'package:passbubble/core/api/models.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/core/desktop/desktop_window.dart';
import 'package:passbubble/core/desktop/tray_host.dart';
import 'package:passbubble/core/desktop/tray_menu.dart';
import 'package:passbubble/core/desktop/tray_scope.dart';
import 'package:passbubble/core/desktop/tray_settings.dart';
import 'package:passbubble/core/router/router.dart';
import 'package:passbubble/features/entries/entries_list_screen.dart';
import 'package:passbubble/features/settings/tray_settings_tile.dart';

class _UnlockedAuth extends AuthNotifier {
  _UnlockedAuth(super.svc) {
    state = const AuthState(isLoggedIn: true, isUnlocked: true, userId: 'u');
  }
}

class _FakeTray implements TrayHost {
  final _available = ValueNotifier(true);
  List<TrayMenuItem> menu = const [];
  bool? unlocked;
  String? toolTip;
  bool closed = false;

  @override
  ValueNotifier<bool> get available => _available;

  @override
  Future<void> update({
    required List<TrayMenuItem> menu,
    required bool unlocked,
    required String toolTip,
  }) async {
    this.menu = menu;
    this.unlocked = unlocked;
    this.toolTip = toolTip;
  }

  @override
  Future<void> close() async => closed = true;

  TrayMenuItem row(String label) => [
        ...menu,
        for (final m in menu) ...m.children,
      ].firstWhere((i) => i.label == label);
}

class _FakeWindow implements DesktopWindow {
  final calls = <String>[];
  bool? hideOnClose;

  @override
  Future<void> show({String? activationToken}) async =>
      calls.add('show ${activationToken ?? '-'}');

  @override
  Future<void> hide() async => calls.add('hide');

  @override
  Future<void> setHideOnClose(bool value) async => hideOnClose = value;

  @override
  Future<void> quit() async => calls.add('quit');
}

EntryResponse _entry(String id, String name, {bool favorite = false}) =>
    EntryResponse(
      id: id,
      type: 'login',
      name: name,
      url: '',
      encryptedData: '',
      dataNonce: '',
      permission: 'owner',
      favorite: favorite,
      createdAt: '',
      updatedAt: '',
    );

class _Harness {
  _Harness(this.container, this.tray, this.window, this.router, this.select);
  final ProviderContainer container;
  final _FakeTray tray;
  final _FakeWindow window;
  final GoRouter router;
  final TraySelectHandler Function() select;

  String get location => router.routerDelegate.currentConfiguration.uri.toString();

  Future<void> click(WidgetTester tester, String label, {String? token}) async {
    select()(tray.row(label), token);
    await tester.pumpAndSettle();
  }
}

Future<_Harness> _pump(WidgetTester tester, {String start = '/entries'}) async {
  FlutterSecureStorage.setMockInitialValues({});
  final tray = _FakeTray();
  final window = _FakeWindow();
  TraySelectHandler? select;
  final router = GoRouter(initialLocation: start, routes: [
    for (final path in ['/entries', '/entries/new', '/entries/:id', '/generate'])
      GoRoute(
          path: path,
          builder: (_, s) => Scaffold(body: Text('page ${s.uri.path}'))),
  ]);
  addTearDown(router.dispose);
  final container = ProviderContainer(overrides: [
    authStateProvider
        .overrideWith((ref) => _UnlockedAuth(AuthService(ApiClient()))),
    entriesProvider.overrideWith((ref) async => [
          _entry('e1', 'Bank', favorite: true),
          _entry('e2', 'Mail'),
        ]),
    routerProvider.overrideWithValue(router),
    desktopWindowProvider.overrideWithValue(window),
    trayHostFactoryProvider.overrideWithValue(
        ({required onSelected, required onActivate}) async {
      select = onSelected;
      return tray;
    }),
  ]);
  addTearDown(container.dispose);
  await tester.pumpWidget(UncontrolledProviderScope(
    container: container,
    child: TrayScope(child: MaterialApp.router(routerConfig: router)),
  ));
  await tester.pumpAndSettle();
  return _Harness(container, tray, window, router, () => select!);
}

final _linux = TargetPlatformVariant.only(TargetPlatform.linux);

void main() {
  testWidgets('the menu and icon follow the vault state', (tester) async {
    final h = await _pump(tester);
    expect(h.tray.unlocked, isTrue);
    expect(h.tray.toolTip, 'Vault unlocked');
    expect(h.tray.row('Favorites').children.map((i) => i.label), ['Bank']);

    h.container.read(authStateProvider.notifier).lock();
    await tester.pumpAndSettle();
    expect(h.tray.unlocked, isFalse);
    expect(h.tray.toolTip, 'Vault locked');
    expect(h.tray.menu.map((i) => i.label),
        allOf(contains('Vault locked'), isNot(contains('Favorites'))));
  }, variant: _linux);

  testWidgets('menu rows open pages, lock the vault and quit', (tester) async {
    final h = await _pump(tester);

    await h.click(tester, 'New entry…', token: 'tok');
    expect(h.location, '/entries/new');
    expect(h.window.calls.last, 'show tok');

    h.router.go('/entries');
    await tester.pumpAndSettle();
    await h.click(tester, 'Bank');
    expect(h.location, '/entries/e1');

    await h.click(tester, 'Password generator…');
    expect(h.location, '/generate');

    await h.click(tester, 'Search vault…');
    expect(h.location, '/entries');
    expect(h.container.read(vaultSearchFocusRequestProvider), isTrue);

    await h.click(tester, 'Lock vault');
    expect(h.container.read(authStateProvider).isUnlocked, isFalse);

    await h.click(tester, 'Quit Passbubble');
    expect(h.window.calls.last, 'quit');
  }, variant: _linux);

  testWidgets('an open editor stays put; the window just comes back',
      (tester) async {
    final h = await _pump(tester, start: '/entries/new');
    await h.click(tester, 'Bank');
    expect(h.location, '/entries/new');
    expect(h.window.calls.last, 'show -');
  }, variant: _linux);

  testWidgets('closing hides into the tray only while a host shows the icon',
      (tester) async {
    final h = await _pump(tester);
    expect(h.window.hideOnClose, isTrue);
    expect(h.container.read(trayAvailableProvider), isTrue);

    h.tray.available.value = false;
    await tester.pump();
    expect(h.window.hideOnClose, isFalse);
    expect(h.window.calls.last, 'show -',
        reason: 'a window hidden in a vanished tray comes back');
  }, variant: _linux);

  testWidgets('switching the setting off removes the icon', (tester) async {
    final h = await _pump(tester);
    await tester.pumpWidget(UncontrolledProviderScope(
      container: h.container,
      child: TrayScope(
        child: MaterialApp(
            home: Scaffold(body: ListView(children: const [TraySettingsTile()]))),
      ),
    ));
    expect(find.text('Closing the window keeps Passbubble running in the tray'),
        findsOneWidget);

    await tester.tap(find.byType(Switch));
    await tester.pumpAndSettle();
    expect(h.tray.closed, isTrue);
    expect(h.window.hideOnClose, isFalse);
    expect(find.text('Closing the window quits Passbubble'), findsOneWidget);
    expect(await const FlutterSecureStorage().read(key: 'tray_enabled'),
        'false');
  }, variant: _linux);

  testWidgets('no tray outside Linux', (tester) async {
    final h = await _pump(tester);
    expect(h.tray.menu, isEmpty);
    expect(h.window.hideOnClose, isNull);
  }, variant: TargetPlatformVariant.only(TargetPlatform.android));
}

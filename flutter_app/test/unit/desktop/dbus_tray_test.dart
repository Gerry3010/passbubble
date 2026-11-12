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

import 'dart:typed_data';

import 'package:dbus/dbus.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/desktop/dbus_menu.dart';
import 'package:passbubble/core/desktop/status_notifier_tray.dart';
import 'package:passbubble/core/desktop/tray_icon.dart';
import 'package:passbubble/core/desktop/tray_menu.dart';

List<String> _labels(List<TrayMenuItem> items) => [
      for (final i in items)
        i.children.isEmpty ? i.toString() : '$i > ${_labels(i.children)}',
    ];

DBusMethodCall _call(String name, List<DBusValue> values,
        {String interface = dbusMenuInterface}) =>
    DBusMethodCall(
        sender: ':1.42', interface: interface, name: name, values: values);

Future<List<DBusValue>> _ok(DBusObject object, DBusMethodCall call) async {
  final response = await object.handleMethodCall(call);
  expect(response, isA<DBusMethodSuccessResponse>(), reason: call.name);
  return (response as DBusMethodSuccessResponse).values;
}

/// GetLayout's (ia{sv}av) tree as "id:label" strings, children in brackets.
String _render(DBusValue node) {
  final fields = node.asStruct();
  final props = fields[1].asStringVariantDict();
  final label = props['type']?.asString() == 'separator'
      ? '---'
      : props['label']?.asString() ?? '';
  final enabled = props['enabled']?.asBoolean() ?? true;
  final children = fields[2].asVariantArray().map(_render).toList();
  return '${enabled ? '' : '!'}$label'
      '${children.isEmpty ? '' : ' [${children.join(', ')}]'}';
}

Future<String> _layout(DBusMenu menu) async {
  final out = await _ok(menu, _call('GetLayout', [
    const DBusInt32(0),
    const DBusInt32(-1),
    DBusArray.string(const []),
  ]));
  return _render(out[1]);
}

int _idOf(DBusMenu menu, String label) {
  int? find(DBusValue node) {
    final fields = node.asStruct();
    if (fields[1].asStringVariantDict()['label']?.asString() == label) {
      return fields[0].asInt32();
    }
    for (final child in fields[2].asVariantArray()) {
      final id = find(child);
      if (id != null) return id;
    }
    return null;
  }

  return find(menu.layout())!;
}

void main() {
  group('buildTrayMenu', () {
    test('signed out: open and quit only', () {
      expect(_labels(buildTrayMenu(loggedIn: false, unlocked: false)),
          ['Open Passbubble', '---', 'Quit Passbubble']);
    });

    test('locked: quick actions go through unlock, no favourite names', () {
      expect(
        _labels(buildTrayMenu(
            loggedIn: true,
            unlocked: false,
            favorites: const [(id: 'e1', name: 'Bank')])),
        [
          'Open Passbubble',
          'Search vault…',
          'New entry…',
          'Password generator…',
          '---',
          'Vault locked',
          '---',
          'Quit Passbubble',
        ],
      );
    });

    test('unlocked: favourites sorted by name, capped, and a lock row', () {
      final favorites = [
        for (var i = 0; i < maxTrayFavorites + 3; i++)
          (id: 'e$i', name: 'entry ${i.toString().padLeft(2, '0')}'),
        (id: 'x', name: 'Alpha'),
      ];
      final menu = buildTrayMenu(
          loggedIn: true, unlocked: true, favorites: favorites.reversed.toList());
      final sub = menu.firstWhere((i) => i.label == 'Favorites');
      expect(sub.children, hasLength(maxTrayFavorites));
      expect(sub.children.first.label, 'Alpha');
      expect(sub.children.first.command, TrayCommand.openEntry);
      expect(sub.children.first.entryId, 'x');
      expect(sub.children[1].label, 'entry 00');
      expect(menu.map((i) => i.label), contains('Lock vault'));
      expect(menu.last.command, TrayCommand.quit);
    });

    test('unlocked without favourites has no empty submenu', () {
      expect(
          buildTrayMenu(loggedIn: true, unlocked: true)
              .map((i) => i.label),
          isNot(contains('Favorites')));
    });
  });

  group('DBusMenu', () {
    late List<TrayMenuItem> clicked;
    late DBusMenu menu;

    setUp(() {
      clicked = [];
      menu = DBusMenu(onClicked: clicked.add);
    });

    test('GetLayout returns the whole tree with labels and separators',
        () async {
      await menu.setItems(buildTrayMenu(
          loggedIn: true,
          unlocked: true,
          favorites: const [(id: 'e1', name: 'my_bank')]));
      expect(
        await _layout(menu),
        ' [Open Passbubble, Search vault…, New entry…, Password generator…, '
        '---, Favorites [my__bank], ---, Lock vault, ---, Quit Passbubble]',
        reason: 'a literal underscore is doubled, it would be a mnemonic',
      );
    });

    test('a depth of 0 returns the item without children', () async {
      await menu.setItems(buildTrayMenu(loggedIn: false, unlocked: false));
      final out = await _ok(menu, _call('GetLayout', [
        const DBusInt32(0),
        const DBusInt32(0),
        DBusArray.string(const []),
      ]));
      expect(out[1].asStruct()[2].asArray(), isEmpty);
    });

    test('a click runs the row; a disabled row does nothing', () async {
      await menu.setItems(buildTrayMenu(loggedIn: true, unlocked: false));
      for (final label in ['New entry…', 'Vault locked']) {
        await _ok(menu, _call('Event', [
          DBusInt32(_idOf(menu, label)),
          const DBusString('clicked'),
          const DBusVariant(DBusInt32(0)),
          const DBusUint32(0),
        ]));
      }
      await Future<void>.delayed(Duration.zero);
      expect(clicked.map((i) => i.command), [TrayCommand.newEntry]);
    });

    test('ids survive a rebuild, so a stale click hits the same row',
        () async {
      await menu.setItems(buildTrayMenu(loggedIn: true, unlocked: true));
      final lockId = _idOf(menu, 'Lock vault');
      final quitId = _idOf(menu, 'Quit Passbubble');
      final revision = menu.revision;

      // The vault locks while the menu is open: "Lock vault" disappears.
      await menu.setItems(buildTrayMenu(loggedIn: true, unlocked: false));
      expect(menu.revision, greaterThan(revision));
      expect(_idOf(menu, 'Quit Passbubble'), quitId);

      final out = await _ok(menu, _call('EventGroup', [
        DBusArray(DBusSignature('(isvu)'), [
          DBusStruct([
            DBusInt32(lockId),
            const DBusString('clicked'),
            const DBusVariant(DBusInt32(0)),
            const DBusUint32(0),
          ]),
        ]),
      ]));
      await Future<void>.delayed(Duration.zero);
      expect(clicked, isEmpty, reason: 'the old id must not reach Quit');
      expect(out[0].asInt32Array(), [lockId]);
    });

    test('group properties and the object properties', () async {
      await menu.setItems(buildTrayMenu(loggedIn: true, unlocked: false));
      final id = _idOf(menu, 'Vault locked');
      final out = await _ok(menu, _call('GetGroupProperties', [
        DBusArray.int32([id, 999]),
        DBusArray.string(const ['enabled']),
      ]));
      final rows = out[0].asArray();
      expect(rows, hasLength(1));
      expect(rows.single.asStruct()[1].asStringVariantDict(),
          {'enabled': const DBusBoolean(false)});

      final version = await menu.getProperty(dbusMenuInterface, 'Version');
      expect((version as DBusGetPropertyResponse).values.single,
          const DBusVariant(DBusUint32(3)));
      expect(
          (await _ok(menu, _call('AboutToShow', [const DBusInt32(0)])))
              .single,
          const DBusBoolean(false));
    });
  });

  group('StatusNotifierItem', () {
    test('points the host at the menu and carries the icon and tooltip',
        () async {
      final item = StatusNotifierItem(onActivate: (_) {});
      final pixmap = TrayPixmap(1, 1, Uint8List.fromList([255, 0, 255, 65]));
      await item.setIcon([pixmap], 'Vault locked');
      final p = item.properties;
      expect(p['Menu'], DBusObjectPath(dbusMenuPath));
      expect(p['ItemIsMenu'], const DBusBoolean(false));
      expect(p['Status'], const DBusString('Active'));
      final icon = p['IconPixmap']!.asArray().single.asStruct();
      expect(icon[0], const DBusInt32(1));
      expect(icon[2].asByteArray(), [255, 0, 255, 65]);
      expect(p['ToolTip']!.asStruct()[3], const DBusString('Vault locked'));
    });

    test('a click opens the window with the host\'s activation token, once',
        () async {
      final tokens = <String?>[];
      final item = StatusNotifierItem(onActivate: tokens.add);
      Future<void> call(String name, List<DBusValue> values) async {
        final r = await item.handleMethodCall(DBusMethodCall(
            sender: ':1.42',
            interface: statusNotifierItemInterface,
            name: name,
            values: values));
        expect(r, isA<DBusMethodSuccessResponse>());
      }

      await call('ProvideXdgActivationToken', [const DBusString('tok')]);
      await call('Activate', [const DBusInt32(0), const DBusInt32(0)]);
      await call('Activate', [const DBusInt32(0), const DBusInt32(0)]);
      await Future<void>.delayed(Duration.zero);
      expect(tokens, ['tok', null]);
    });
  });

  group('tray icon', () {
    test('RGBA bytes become network-order ARGB', () {
      expect(rgbaToArgb(Uint8List.fromList([1, 2, 3, 4, 5, 6, 7, 8])),
          [4, 1, 2, 3, 8, 5, 6, 7]);
    });

    testWidgets('renders every size, open and closed padlocks differ',
        (tester) async {
      await tester.runAsync(() async {
        final locked = await renderTrayIcons(unlocked: false);
        final unlocked = await renderTrayIcons(unlocked: true);
        expect(locked.map((p) => p.width), trayIconSizes);
        for (final p in locked) {
          expect(p.argb.length, p.width * p.height * 4);
          expect(p.argb[0], 0, reason: 'corner stays transparent');
        }
        final i32 = trayIconSizes.indexOf(32);
        // Centre of the body: opaque green.
        final centre = ((20 * 32) + 12) * 4;
        expect(locked[i32].argb.sublist(centre, centre + 4),
            [255, 0, 255, 65]);
        expect(locked[i32].argb, isNot(equals(unlocked[i32].argb)));
      });
    });
  });
}

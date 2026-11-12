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

import 'dart:async';
import 'dart:io';

import 'package:dbus/dbus.dart';
import 'package:flutter/foundation.dart';

import 'dbus_menu.dart';
import 'tray_host.dart';
import 'tray_icon.dart';
import 'tray_menu.dart';

const statusNotifierItemInterface = 'org.kde.StatusNotifierItem';
const statusNotifierItemPath = '/StatusNotifierItem';
const _watcherName = 'org.kde.StatusNotifierWatcher';

DBusArray _pixmaps(List<TrayPixmap> pixmaps) =>
    DBusArray(DBusSignature('(iiay)'), [
      for (final p in pixmaps)
        DBusStruct([
          DBusInt32(p.width),
          DBusInt32(p.height),
          DBusArray.byte(p.argb),
        ]),
    ]);

/// The tray icon as a StatusNotifierItem: the D-Bus object a tray host
/// (GNOME's AppIndicator extension, KDE, XFCE, …) reads the icon, tooltip
/// and menu path from. Pure Dart, so the app needs no libappindicator.
class StatusNotifierItem extends DBusObject {
  StatusNotifierItem({required this.onActivate})
      : super(DBusObjectPath(statusNotifierItemPath));

  final TrayActivateHandler onActivate;

  List<TrayPixmap> _icon = const [];
  String _toolTip = '';
  String? _activationToken;

  /// The token the host handed over with the last click, used once.
  String? takeActivationToken() {
    final token = _activationToken;
    _activationToken = null;
    return token;
  }

  Future<void> setIcon(List<TrayPixmap> icon, String toolTip) async {
    _icon = icon;
    _toolTip = toolTip;
    if (client != null) {
      await emitSignal(statusNotifierItemInterface, 'NewIcon');
      await emitSignal(statusNotifierItemInterface, 'NewToolTip');
    }
  }

  @visibleForTesting
  Map<String, DBusValue> get properties => {
        'Category': const DBusString('ApplicationStatus'),
        'Id': const DBusString('passbubble'),
        'Title': const DBusString('Passbubble'),
        'Status': const DBusString('Active'),
        'WindowId': const DBusInt32(0),
        'IconName': const DBusString(''),
        'IconThemePath': const DBusString(''),
        'IconPixmap': _pixmaps(_icon),
        'OverlayIconName': const DBusString(''),
        'OverlayIconPixmap': _pixmaps(const []),
        'AttentionIconName': const DBusString(''),
        'AttentionIconPixmap': _pixmaps(const []),
        'AttentionMovieName': const DBusString(''),
        'ToolTip': DBusStruct([
          const DBusString(''),
          _pixmaps(const []),
          const DBusString('Passbubble'),
          DBusString(_toolTip),
        ]),
        'ItemIsMenu': const DBusBoolean(false),
        'Menu': DBusObjectPath(dbusMenuPath),
      };

  @override
  Future<DBusMethodResponse> handleMethodCall(DBusMethodCall methodCall) async {
    if (methodCall.interface != statusNotifierItemInterface) {
      return DBusMethodErrorResponse.unknownInterface();
    }
    switch (methodCall.name) {
      case 'Activate':
      case 'SecondaryActivate':
        final token = takeActivationToken();
        Future(() => onActivate(token));
        return DBusMethodSuccessResponse();
      case 'ContextMenu':
      case 'Scroll':
        return DBusMethodSuccessResponse();
      case 'ProvideXdgActivationToken':
        _activationToken = methodCall.values[0].asString();
        return DBusMethodSuccessResponse();
    }
    return DBusMethodErrorResponse.unknownMethod();
  }

  @override
  Future<DBusMethodResponse> getProperty(String interface, String name) async {
    final value =
        interface == statusNotifierItemInterface ? properties[name] : null;
    return value == null
        ? DBusMethodErrorResponse.unknownProperty()
        : DBusGetPropertyResponse(value);
  }

  @override
  Future<DBusMethodResponse> getAllProperties(String interface) async =>
      DBusGetAllPropertiesResponse(
          interface == statusNotifierItemInterface ? properties : {});

  @override
  List<DBusIntrospectInterface> introspect() {
    DBusIntrospectArgument arg(String type, String name,
            [DBusArgumentDirection direction = DBusArgumentDirection.in_]) =>
        DBusIntrospectArgument(DBusSignature(type), direction, name: name);
    final types = <String, String>{
      'Category': 's',
      'Id': 's',
      'Title': 's',
      'Status': 's',
      'WindowId': 'i',
      'IconName': 's',
      'IconThemePath': 's',
      'IconPixmap': 'a(iiay)',
      'OverlayIconName': 's',
      'OverlayIconPixmap': 'a(iiay)',
      'AttentionIconName': 's',
      'AttentionIconPixmap': 'a(iiay)',
      'AttentionMovieName': 's',
      'ToolTip': '(sa(iiay)ss)',
      'ItemIsMenu': 'b',
      'Menu': 'o',
    };
    return [
      DBusIntrospectInterface(statusNotifierItemInterface, methods: [
        DBusIntrospectMethod('Activate', args: [arg('i', 'x'), arg('i', 'y')]),
        DBusIntrospectMethod('SecondaryActivate',
            args: [arg('i', 'x'), arg('i', 'y')]),
        DBusIntrospectMethod('ContextMenu',
            args: [arg('i', 'x'), arg('i', 'y')]),
        DBusIntrospectMethod('Scroll',
            args: [arg('i', 'delta'), arg('s', 'orientation')]),
        DBusIntrospectMethod('ProvideXdgActivationToken',
            args: [arg('s', 'token')]),
      ], signals: [
        DBusIntrospectSignal('NewIcon'),
        DBusIntrospectSignal('NewToolTip'),
        DBusIntrospectSignal('NewTitle'),
        DBusIntrospectSignal('NewStatus',
            args: [arg('s', 'status', DBusArgumentDirection.out)]),
      ], properties: [
        for (final p in types.entries)
          DBusIntrospectProperty(p.key, DBusSignature(p.value),
              access: DBusPropertyAccess.read),
      ]),
    ];
  }
}

/// The tray on Linux: exports the item and its menu on the session bus and
/// registers with whatever StatusNotifierWatcher runs, again whenever the
/// watcher restarts.
class StatusNotifierTray implements TrayHost {
  StatusNotifierTray._(this._client, this._item, this._menu, this._busName);

  final DBusClient _client;
  final StatusNotifierItem _item;
  final DBusMenu _menu;
  final String _busName;
  final _available = ValueNotifier(false);
  StreamSubscription<DBusNameOwnerChangedEvent>? _watcherChanges;
  final _icons = <bool, List<TrayPixmap>>{};
  bool? _shownUnlocked;
  String? _shownToolTip;

  static Future<StatusNotifierTray> start({
    required TraySelectHandler onSelected,
    required TrayActivateHandler onActivate,
  }) async {
    final client = DBusClient.session();
    try {
      final item = StatusNotifierItem(onActivate: onActivate);
      final menu = DBusMenu(
          onClicked: (row) => onSelected(row, item.takeActivationToken()));
      final busName = 'org.kde.StatusNotifierItem-$pid-1';
      await client.requestName(busName,
          flags: {DBusRequestNameFlag.doNotQueue});
      await client.registerObject(item);
      await client.registerObject(menu);
      final tray = StatusNotifierTray._(client, item, menu, busName);
      tray._watcherChanges = client.nameOwnerChanged
          .where((e) => e.name == _watcherName)
          .listen((e) {
        if (e.newOwner != null) {
          tray._register();
        } else {
          tray._available.value = false;
        }
      });
      await tray._register();
      return tray;
    } catch (_) {
      await client.close();
      rethrow;
    }
  }

  Future<void> _register() async {
    try {
      await _client.callMethod(
        destination: _watcherName,
        path: DBusObjectPath('/StatusNotifierWatcher'),
        interface: _watcherName,
        name: 'RegisterStatusNotifierItem',
        values: [DBusString(_busName)],
      );
      _available.value = true;
    } catch (_) {
      // No watcher on the bus (e.g. GNOME without the AppIndicator
      // extension). Registration retries once one appears.
      _available.value = false;
    }
  }

  @override
  ValueListenable<bool> get available => _available;

  @override
  Future<void> update({
    required List<TrayMenuItem> menu,
    required bool unlocked,
    required String toolTip,
  }) async {
    await _menu.setItems(menu);
    if (_shownUnlocked != unlocked || _shownToolTip != toolTip) {
      _shownUnlocked = unlocked;
      _shownToolTip = toolTip;
      final icon = _icons[unlocked] ??=
          await renderTrayIcons(unlocked: unlocked);
      await _item.setIcon(icon, toolTip);
    }
  }

  @override
  Future<void> close() async {
    await _watcherChanges?.cancel();
    _available.dispose();
    await _client.close();
  }
}

Future<TrayHost?> startPlatformTray({
  required TraySelectHandler onSelected,
  required TrayActivateHandler onActivate,
}) async {
  if (!Platform.isLinux) return null;
  try {
    return await StatusNotifierTray.start(
        onSelected: onSelected, onActivate: onActivate);
  } catch (e) {
    debugPrint('tray: no session bus ($e)');
    return null;
  }
}

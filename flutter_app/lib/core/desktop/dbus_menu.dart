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

import 'package:dbus/dbus.dart';

import 'tray_menu.dart';

const dbusMenuInterface = 'com.canonical.dbusmenu';
const dbusMenuPath = '/MenuBar';

class _Node {
  _Node(this.id, this.item, this.children);
  final int id;

  /// null for the invisible root (id 0).
  final TrayMenuItem? item;
  final List<_Node> children;
}

/// The tray's menu, exported over D-Bus as `com.canonical.dbusmenu` — the
/// protocol GNOME's AppIndicator extension, KDE and most other tray hosts
/// use to draw an app's menu themselves.
class DBusMenu extends DBusObject {
  DBusMenu({required this.onClicked}) : super(DBusObjectPath(dbusMenuPath)) {
    _root = _Node(0, null, const []);
    _byId[0] = _root;
  }

  /// Called with the row the user clicked.
  final void Function(TrayMenuItem item) onClicked;

  late _Node _root;
  final _byId = <int, _Node>{};

  // Ids stay bound to a row's key for the whole session, so a click on a
  // menu drawn before an update cannot land on a different row.
  final _idsByKey = <String, int>{};
  int _nextId = 1;
  int _revision = 0;

  int get revision => _revision;

  /// Replaces the menu and tells the host to fetch it again.
  Future<void> setItems(List<TrayMenuItem> items) async {
    _byId.clear();
    final seen = <String, int>{};
    List<_Node> build(List<TrayMenuItem> items, String parentKey) => [
          for (final item in items)
            () {
              var key = '$parentKey/${item.key}';
              final n = seen[key] = (seen[key] ?? 0) + 1;
              if (n > 1) key = '$key#$n';
              final id = _idsByKey.putIfAbsent(key, () => _nextId++);
              final node = _Node(id, item, build(item.children, key));
              _byId[id] = node;
              return node;
            }(),
        ];
    _root = _Node(0, null, build(items, ''));
    _byId[0] = _root;
    _revision++;
    if (client != null) {
      await emitSignal(dbusMenuInterface, 'LayoutUpdated',
          [DBusUint32(_revision), DBusInt32(0)]);
    }
  }

  static Map<String, DBusValue> _properties(_Node node) {
    final item = node.item;
    if (item == null) return {'children-display': const DBusString('submenu')};
    if (item.isSeparator) return {'type': const DBusString('separator')};
    return {
      // dbusmenu reads "_" as a mnemonic marker; a literal one is doubled.
      'label': DBusString(item.label.replaceAll('_', '__')),
      'enabled': DBusBoolean(item.enabled),
      if (item.children.isNotEmpty)
        'children-display': const DBusString('submenu'),
    };
  }

  static Map<String, DBusValue> _filtered(_Node node, List<String> names) {
    final all = _properties(node);
    if (names.isEmpty) return all;
    return {
      for (final e in all.entries)
        if (names.contains(e.key)) e.key: e.value,
    };
  }

  DBusStruct _layout(_Node node, int depth, List<String> names) =>
      DBusStruct([
        DBusInt32(node.id),
        DBusDict.stringVariant(_filtered(node, names)),
        DBusArray.variant([
          if (depth != 0)
            for (final child in node.children)
              _layout(child, depth < 0 ? -1 : depth - 1, names),
        ]),
      ]);

  /// The layout as GetLayout returns it, for tests and debugging.
  DBusStruct layout({int parentId = 0, int depth = -1}) =>
      _layout(_byId[parentId] ?? _root, depth, const []);

  void _click(int id) {
    final item = _byId[id]?.item;
    if (item == null || !item.enabled || item.command == null) return;
    // Answer the D-Bus call first; the command may hide or quit the app.
    Future(() => onClicked(item));
  }

  @override
  Future<DBusMethodResponse> handleMethodCall(DBusMethodCall methodCall) async {
    if (methodCall.interface != dbusMenuInterface) {
      return DBusMethodErrorResponse.unknownInterface();
    }
    final v = methodCall.values;
    switch (methodCall.name) {
      case 'GetLayout':
        final node = _byId[v[0].asInt32()];
        if (node == null) {
          return DBusMethodErrorResponse.invalidArgs('unknown menu item');
        }
        return DBusMethodSuccessResponse([
          DBusUint32(_revision),
          _layout(node, v[1].asInt32(), v[2].asStringArray().toList()),
        ]);
      case 'GetGroupProperties':
        final ids = v[0].asInt32Array();
        final names = v[1].asStringArray().toList();
        return DBusMethodSuccessResponse([
          DBusArray(DBusSignature('(ia{sv})'), [
            for (final id in ids)
              if (_byId[id] case final node?)
                DBusStruct([
                  DBusInt32(id),
                  DBusDict.stringVariant(_filtered(node, names)),
                ]),
          ]),
        ]);
      case 'GetProperty':
        final node = _byId[v[0].asInt32()];
        final value = node == null ? null : _properties(node)[v[1].asString()];
        if (value == null) return DBusMethodErrorResponse.invalidArgs();
        return DBusMethodSuccessResponse([DBusVariant(value)]);
      case 'Event':
        if (v[1].asString() == 'clicked') _click(v[0].asInt32());
        return DBusMethodSuccessResponse();
      case 'EventGroup':
        final unknown = <int>[];
        for (final event in v[0].asArray()) {
          final fields = event.asStruct();
          final id = fields[0].asInt32();
          if (!_byId.containsKey(id)) {
            unknown.add(id);
          } else if (fields[1].asString() == 'clicked') {
            _click(id);
          }
        }
        return DBusMethodSuccessResponse([DBusArray.int32(unknown)]);
      case 'AboutToShow':
        return DBusMethodSuccessResponse([const DBusBoolean(false)]);
      case 'AboutToShowGroup':
        return DBusMethodSuccessResponse(
            [DBusArray.int32(const []), DBusArray.int32(const [])]);
    }
    return DBusMethodErrorResponse.unknownMethod();
  }

  Map<String, DBusValue> get _objectProperties => {
        'Version': const DBusUint32(3),
        'TextDirection': const DBusString('ltr'),
        'Status': const DBusString('normal'),
        'IconThemePath': DBusArray.string(const []),
      };

  @override
  Future<DBusMethodResponse> getProperty(String interface, String name) async {
    final value =
        interface == dbusMenuInterface ? _objectProperties[name] : null;
    return value == null
        ? DBusMethodErrorResponse.unknownProperty()
        : DBusGetPropertyResponse(value);
  }

  @override
  Future<DBusMethodResponse> getAllProperties(String interface) async =>
      DBusGetAllPropertiesResponse(
          interface == dbusMenuInterface ? _objectProperties : {});

  @override
  List<DBusIntrospectInterface> introspect() {
    DBusIntrospectArgument inArg(String type, String name) =>
        DBusIntrospectArgument(DBusSignature(type), DBusArgumentDirection.in_,
            name: name);
    DBusIntrospectArgument outArg(String type, String name) =>
        DBusIntrospectArgument(DBusSignature(type), DBusArgumentDirection.out,
            name: name);
    return [
      DBusIntrospectInterface(dbusMenuInterface, methods: [
        DBusIntrospectMethod('GetLayout', args: [
          inArg('i', 'parentId'),
          inArg('i', 'recursionDepth'),
          inArg('as', 'propertyNames'),
          outArg('u', 'revision'),
          outArg('(ia{sv}av)', 'layout'),
        ]),
        DBusIntrospectMethod('GetGroupProperties', args: [
          inArg('ai', 'ids'),
          inArg('as', 'propertyNames'),
          outArg('a(ia{sv})', 'properties'),
        ]),
        DBusIntrospectMethod('GetProperty', args: [
          inArg('i', 'id'),
          inArg('s', 'name'),
          outArg('v', 'value'),
        ]),
        DBusIntrospectMethod('Event', args: [
          inArg('i', 'id'),
          inArg('s', 'eventId'),
          inArg('v', 'data'),
          inArg('u', 'timestamp'),
        ]),
        DBusIntrospectMethod('EventGroup', args: [
          inArg('a(isvu)', 'events'),
          outArg('ai', 'idErrors'),
        ]),
        DBusIntrospectMethod('AboutToShow', args: [
          inArg('i', 'id'),
          outArg('b', 'needUpdate'),
        ]),
        DBusIntrospectMethod('AboutToShowGroup', args: [
          inArg('ai', 'ids'),
          outArg('ai', 'updatesNeeded'),
          outArg('ai', 'idErrors'),
        ]),
      ], signals: [
        DBusIntrospectSignal('ItemsPropertiesUpdated', args: [
          outArg('a(ia{sv})', 'updatedProps'),
          outArg('a(ias)', 'removedProps'),
        ]),
        DBusIntrospectSignal('LayoutUpdated', args: [
          outArg('u', 'revision'),
          outArg('i', 'parent'),
        ]),
        DBusIntrospectSignal('ItemActivationRequested', args: [
          outArg('i', 'id'),
          outArg('u', 'timestamp'),
        ]),
      ], properties: [
        for (final p in const {
          'Version': 'u',
          'TextDirection': 's',
          'Status': 's',
          'IconThemePath': 'as',
        }.entries)
          DBusIntrospectProperty(p.key, DBusSignature(p.value),
              access: DBusPropertyAccess.read),
      ]),
    ];
  }
}

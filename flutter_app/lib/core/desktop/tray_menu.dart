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

/// What a tray menu row does when clicked.
enum TrayCommand { open, search, newEntry, generator, openEntry, lock, quit }

/// One row of the tray menu: a command, a separator, a disabled status line
/// or a submenu.
class TrayMenuItem {
  const TrayMenuItem(this.label, this.command, {this.entryId})
      : enabled = true,
        isSeparator = false,
        children = const [];

  const TrayMenuItem.status(this.label)
      : command = null,
        entryId = null,
        enabled = false,
        isSeparator = false,
        children = const [];

  const TrayMenuItem.submenu(this.label, this.children)
      : command = null,
        entryId = null,
        enabled = true,
        isSeparator = false;

  const TrayMenuItem.separator()
      : label = '',
        command = null,
        entryId = null,
        enabled = false,
        isSeparator = true,
        children = const [];

  final String label;
  final TrayCommand? command;

  /// The entry a [TrayCommand.openEntry] row opens.
  final String? entryId;
  final bool enabled;
  final bool isSeparator;
  final List<TrayMenuItem> children;

  /// Stays the same while the row means the same thing, so a click on a menu
  /// that was built before the vault locked still reaches the right command.
  String get key => isSeparator
      ? 'separator'
      : '${command?.name ?? 'label'}:${entryId ?? ''}:${command == null ? label : ''}';

  @override
  String toString() => isSeparator ? '---' : label;
}

/// A favourite shown in the tray's Favorites submenu.
typedef TrayFavorite = ({String id, String name});

/// Most favourites the submenu lists; the rest stay a click away in the app.
const maxTrayFavorites = 15;

/// The tray menu for the current vault state. Rows that need an open vault
/// still appear while it is locked: the router puts the unlock screen in
/// front and returns to the row's page afterwards. Favourite names only show
/// while the vault is unlocked.
List<TrayMenuItem> buildTrayMenu({
  required bool loggedIn,
  required bool unlocked,
  List<TrayFavorite> favorites = const [],
}) {
  final items = <TrayMenuItem>[
    const TrayMenuItem('Open Passbubble', TrayCommand.open),
  ];
  if (loggedIn) {
    items.addAll(const [
      TrayMenuItem('Search vault…', TrayCommand.search),
      TrayMenuItem('New entry…', TrayCommand.newEntry),
      TrayMenuItem('Password generator…', TrayCommand.generator),
    ]);
    if (unlocked && favorites.isNotEmpty) {
      final sorted = [...favorites]..sort(
          (a, b) => a.name.toLowerCase().compareTo(b.name.toLowerCase()));
      items
        ..add(const TrayMenuItem.separator())
        ..add(TrayMenuItem.submenu('Favorites', [
          for (final f in sorted.take(maxTrayFavorites))
            TrayMenuItem(f.name, TrayCommand.openEntry, entryId: f.id),
        ]));
    }
    items
      ..add(const TrayMenuItem.separator())
      ..add(unlocked
          ? const TrayMenuItem('Lock vault', TrayCommand.lock)
          : const TrayMenuItem.status('Vault locked'));
  }
  items
    ..add(const TrayMenuItem.separator())
    ..add(const TrayMenuItem('Quit Passbubble', TrayCommand.quit));
  return items;
}

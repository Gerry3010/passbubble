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

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'tray_menu.dart';
import 'tray_platform.dart';

/// A click on a menu row. [activationToken] lets the window take focus.
typedef TraySelectHandler = void Function(
    TrayMenuItem item, String? activationToken);

/// A click on the icon itself, where the host passes it on.
typedef TrayActivateHandler = void Function(String? activationToken);

/// The icon and menu in the desktop's tray.
abstract class TrayHost {
  /// Whether a tray host shows the icon right now. On GNOME that needs the
  /// AppIndicator extension; without it the icon has nowhere to go.
  ValueListenable<bool> get available;

  Future<void> update({
    required List<TrayMenuItem> menu,
    required bool unlocked,
    required String toolTip,
  });

  Future<void> close();
}

typedef TrayHostFactory = Future<TrayHost?> Function({
  required TraySelectHandler onSelected,
  required TrayActivateHandler onActivate,
});

/// Starts the platform's tray; returns null where there is none.
final trayHostFactoryProvider =
    Provider<TrayHostFactory>((_) => startPlatformTray);

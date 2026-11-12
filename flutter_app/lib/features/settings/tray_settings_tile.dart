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

import '../../core/desktop/tray_settings.dart';

/// Turns the tray icon on or off (desktop only). The subtitle says what
/// closing the window does, and why there may be no icon.
class TraySettingsTile extends ConsumerWidget {
  const TraySettingsTile({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final enabled = ref.watch(trayEnabledProvider);
    final available = ref.watch(trayAvailableProvider);
    return SwitchListTile(
      secondary: const Icon(Icons.push_pin_outlined),
      title: const Text('Tray icon'),
      subtitle: Text(!enabled
          ? 'Closing the window quits Passbubble'
          : available
              ? 'Closing the window keeps Passbubble running in the tray'
              : 'No tray found. On GNOME, turn on the AppIndicator extension'),
      value: enabled,
      onChanged: (v) => ref.read(trayEnabledProvider.notifier).set(v),
    );
  }
}

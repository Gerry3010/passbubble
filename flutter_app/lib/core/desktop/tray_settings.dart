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
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

const _kTrayEnabledKey = 'tray_enabled';

/// Platforms with a tray implementation. Linux only for now; macOS and
/// Windows would need their own.
bool get traySupported =>
    !kIsWeb && defaultTargetPlatform == TargetPlatform.linux;

/// Whether the app shows a tray icon and keeps running in the tray when its
/// window is closed. On by default; stored per device.
final trayEnabledProvider =
    StateNotifierProvider<TrayEnabledNotifier, bool>(
        (ref) => TrayEnabledNotifier());

class TrayEnabledNotifier extends StateNotifier<bool> {
  TrayEnabledNotifier() : super(true) {
    ready = _load();
  }

  final _storage = const FlutterSecureStorage();

  /// Completes once the stored choice is loaded, so the tray does not flash
  /// up for someone who turned it off.
  late final Future<void> ready;

  Future<void> _load() async {
    try {
      final raw = await _storage.read(key: _kTrayEnabledKey);
      if (raw != null && mounted) state = raw != 'false';
    } catch (_) {
      // Keyring unavailable: keep the default.
    }
  }

  Future<void> set(bool enabled) async {
    state = enabled;
    await _storage.write(key: _kTrayEnabledKey, value: enabled.toString());
  }
}

/// Whether a tray host shows Passbubble's icon right now (set by TrayScope).
final trayAvailableProvider = StateProvider<bool>((_) => false);

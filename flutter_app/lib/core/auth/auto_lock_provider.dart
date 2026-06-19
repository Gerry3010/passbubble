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

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

const _kAutoLockKey = 'auto_lock_minutes';

/// Default idle timeout (minutes) before the vault auto-locks.
const defaultAutoLockMinutes = 10;

/// Selectable auto-lock presets in minutes. 0 means "off" (disabled).
const autoLockPresets = <int>[0, 1, 5, 10, 15, 30, 60];

/// Holds the auto-lock idle timeout in minutes (0 = disabled). Persisted in
/// secure storage so the choice survives restarts.
final autoLockProvider =
    StateNotifierProvider<AutoLockNotifier, int>((ref) => AutoLockNotifier());

class AutoLockNotifier extends StateNotifier<int> {
  AutoLockNotifier() : super(defaultAutoLockMinutes) {
    _load();
  }

  final _storage = const FlutterSecureStorage(
    aOptions: AndroidOptions(encryptedSharedPreferences: true),
  );

  Future<void> _load() async {
    final raw = await _storage.read(key: _kAutoLockKey);
    final v = int.tryParse(raw ?? '');
    if (v != null && v >= 0) state = v;
  }

  /// Sets the timeout (minutes; 0 disables auto-lock) and persists it.
  Future<void> set(int minutes) async {
    state = minutes;
    await _storage.write(key: _kAutoLockKey, value: minutes.toString());
  }
}

/// Human-readable label for an auto-lock interval.
String autoLockLabel(int minutes) {
  if (minutes <= 0) return 'Off';
  if (minutes == 1) return '1 minute';
  return '$minutes minutes';
}

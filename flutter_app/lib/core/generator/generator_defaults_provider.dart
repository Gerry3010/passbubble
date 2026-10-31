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

import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'password_generator.dart';

const _kGeneratorDefaultsKey = 'generator_defaults';

/// The generator defaults of this device (not synced). Every generator starts
/// from them; only Settings changes them. Persisted as JSON in secure storage,
/// which the logout wipe leaves alone (app configuration, not session data).
final generatorDefaultsProvider =
    StateNotifierProvider<GeneratorDefaultsNotifier, GeneratorOptions>(
        (ref) => GeneratorDefaultsNotifier());

class GeneratorDefaultsNotifier extends StateNotifier<GeneratorOptions> {
  GeneratorDefaultsNotifier() : super(defaultGeneratorOptions) {
    loaded = _load();
  }

  final _storage = const FlutterSecureStorage(
    aOptions: AndroidOptions(encryptedSharedPreferences: true),
  );

  /// Completes once the stored defaults are read, so a generator opened right
  /// after start does not use the factory defaults by mistake.
  late final Future<void> loaded;

  /// Set by [save] / [reset]; a slow initial read must not undo them.
  bool _changed = false;

  Future<void> _load() async {
    try {
      final raw = await _storage.read(key: _kGeneratorDefaultsKey);
      if (raw == null || _changed) return;
      final json = jsonDecode(raw);
      if (json is Map<String, dynamic> && mounted) {
        state = GeneratorOptions.fromJson(json);
      }
    } catch (_) {
      // Unreadable or damaged value: keep the factory defaults.
    }
  }

  /// Makes [options] the defaults, with the symbol set cleaned so Settings
  /// shows what the generator uses.
  Future<void> save(GeneratorOptions options) async {
    _changed = true;
    final cleaned =
        options.copyWith(symbolSet: cleanSymbolSet(options.symbolSet));
    state = cleaned;
    await _storage.write(
        key: _kGeneratorDefaultsKey, value: jsonEncode(cleaned.toJson()));
  }

  /// Back to the factory defaults. Removes the stored value, so later changes
  /// to the factory defaults reach this device too.
  Future<void> reset() async {
    _changed = true;
    state = defaultGeneratorOptions;
    await _storage.delete(key: _kGeneratorDefaultsKey);
  }
}

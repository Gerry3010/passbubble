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
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/generator/generator_defaults_provider.dart';
import 'package:passbubble/core/generator/password_generator.dart';

const _key = 'generator_defaults';

void main() {
  late Map<String, String> store;

  /// A fresh container (like an app start) with its defaults loaded.
  Future<ProviderContainer> start() async {
    final c = ProviderContainer();
    addTearDown(c.dispose);
    await c.read(generatorDefaultsProvider.notifier).loaded;
    return c;
  }

  setUp(() {
    store = {};
    FlutterSecureStorage.setMockInitialValues(store);
  });

  test('factory defaults when nothing is stored', () async {
    final c = await start();
    expect(c.read(generatorDefaultsProvider), defaultGeneratorOptions);
  });

  test('loads stored defaults', () async {
    store[_key] = jsonEncode({'length': 32, 'symbols': false, 'symbol_set': '-_'});
    final c = await start();
    expect(c.read(generatorDefaultsProvider),
        defaultGeneratorOptions.copyWith(length: 32, symbols: false, symbolSet: '-_'));
  });

  test('save stores the cleaned options; the next start loads them', () async {
    final c = await start();
    await c.read(generatorDefaultsProvider.notifier).save(
        defaultGeneratorOptions.copyWith(length: 40, symbolSet: ' -_ - ', avoidAmbiguous: true));

    final saved = defaultGeneratorOptions.copyWith(
        length: 40, symbolSet: '-_', avoidAmbiguous: true);
    expect(c.read(generatorDefaultsProvider), saved);
    expect(jsonDecode(store[_key]!), saved.toJson());

    final next = await start();
    expect(next.read(generatorDefaultsProvider), saved);
  });

  test('reset goes back to the factory defaults and removes the stored value',
      () async {
    store[_key] = jsonEncode({'length': 50});
    final c = await start();
    expect(c.read(generatorDefaultsProvider).length, 50);

    await c.read(generatorDefaultsProvider.notifier).reset();
    expect(c.read(generatorDefaultsProvider), defaultGeneratorOptions);
    expect(store.containsKey(_key), isFalse);
    expect((await start()).read(generatorDefaultsProvider), defaultGeneratorOptions);
  });

  test('a damaged stored value falls back to the factory defaults', () async {
    for (final raw in ['{not json', '[1,2]', '"x"']) {
      store[_key] = raw;
      final c = await start();
      expect(c.read(generatorDefaultsProvider), defaultGeneratorOptions, reason: raw);
    }
  });

  test('a save before the initial read finished is not overwritten', () async {
    store[_key] = jsonEncode({'length': 50});
    final c = ProviderContainer();
    addTearDown(c.dispose);
    final notifier = c.read(generatorDefaultsProvider.notifier);
    final save = notifier.save(defaultGeneratorOptions.copyWith(length: 9));
    await notifier.loaded;
    await save;
    expect(c.read(generatorDefaultsProvider).length, 9);
  });
}

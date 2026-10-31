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

// The shared generator cases (same list in shared-ts and the CLI), plus the
// app's JSON and the unbiased index.

import 'dart:math';

import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/generator/password_generator.dart';

/// Hands out scripted values, one per nextInt call.
class _ScriptedRandom implements Random {
  final List<int> values;
  int calls = 0;
  _ScriptedRandom(this.values);

  @override
  int nextInt(int max) => values[calls++];
  @override
  bool nextBool() => throw UnimplementedError();
  @override
  double nextDouble() => throw UnimplementedError();
}

Set<int> _cps(String s) => s.runes.toSet();

void main() {
  const onlySymbols = GeneratorOptions(
      lowercase: false, uppercase: false, digits: false, symbols: true);

  test('defaults follow the shared spec', () {
    const d = defaultGeneratorOptions;
    expect(d.length, 20);
    expect([d.lowercase, d.uppercase, d.digits, d.symbols], everyElement(isTrue));
    expect(d.symbolSet, r'!@#$%^&*()_+-=[]{}|;:,.<>?');
    expect(d.avoidAmbiguous, isFalse);
  });

  test('length is right', () {
    for (final length in [4, 8, 20, 64, 128]) {
      final pw = generatePassword(defaultGeneratorOptions.copyWith(length: length));
      expect(pw.runes.length, length);
    }
  });

  test('only allowed characters, every active class present', () {
    final allowed = _cps(lowercaseChars + uppercaseChars + digitChars + defaultSymbolSet);
    for (var i = 0; i < 300; i++) {
      // Shortest possible: one character per class, nothing to fill.
      final pw = generatePassword(defaultGeneratorOptions.copyWith(length: 4 + i % 20));
      expect(_cps(pw).difference(allowed), isEmpty, reason: pw);
      for (final cls in [lowercaseChars, uppercaseChars, digitChars, defaultSymbolSet]) {
        expect(_cps(pw).intersection(_cps(cls)), isNotEmpty, reason: '$pw lacks $cls');
      }
    }

    // A class that is off never shows up.
    const lettersOnly = GeneratorOptions(digits: false, symbols: false);
    for (var i = 0; i < 100; i++) {
      final pw = generatePassword(lettersOnly);
      expect(_cps(pw).difference(_cps(lowercaseChars + uppercaseChars)), isEmpty);
    }
  });

  test('a custom symbol set is kept', () {
    final options = defaultGeneratorOptions.copyWith(symbolSet: '-_', length: 40);
    final allowed = _cps('$lowercaseChars$uppercaseChars$digitChars-_');
    for (var i = 0; i < 100; i++) {
      final pw = generatePassword(options);
      expect(_cps(pw).difference(allowed), isEmpty, reason: pw);
      expect(_cps(pw).intersection(_cps('-_')), isNotEmpty);
    }
    final symbols = generatePassword(onlySymbols.copyWith(symbolSet: '-_', length: 30));
    expect(_cps(symbols), _cps('-_'));
  });

  test('Unicode symbols stay whole (code points, not UTF-16 units)', () {
    final options = onlySymbols.copyWith(symbolSet: '😀€ß', length: 30);
    for (var i = 0; i < 50; i++) {
      final pw = generatePassword(options);
      expect(pw.runes.length, 30);
      expect(_cps(pw).difference(_cps('😀€ß')), isEmpty,
          reason: 'no broken surrogate halves: $pw');
    }
  });

  test('the symbol set is cleaned: whitespace, controls and repeats go', () {
    expect(cleanSymbolSet(' -\t_\n-\u0000 　_\u007f\u0085!'), '-_!');
    expect(cleanSymbolSet('😀 😀€'), '😀€');
    final pw = generatePassword(onlySymbols.copyWith(symbolSet: ' - \t- ', length: 12));
    expect(pw, '-' * 12);
  });

  test('avoidAmbiguous removes 0 O o 1 l I | from every class, symbols too', () {
    final options = defaultGeneratorOptions.copyWith(avoidAmbiguous: true, length: 64);
    for (var i = 0; i < 200; i++) {
      final pw = generatePassword(options);
      expect(_cps(pw).intersection(_cps(ambiguousChars)), isEmpty, reason: pw);
    }
    final symbols = generatePassword(
        onlySymbols.copyWith(symbolSet: '|-', avoidAmbiguous: true, length: 20));
    expect(symbols, '-' * 20);
  });

  group('errors', () {
    test('no class active', () {
      const none = GeneratorOptions(
          lowercase: false, uppercase: false, digits: false, symbols: false);
      expect(validateGeneratorOptions(none), isNotNull);
      expect(() => generatePassword(none), throwsA(isA<GeneratorException>()));
    });

    test('an active class is empty after cleaning', () {
      for (final set in ['', '  \t\n']) {
        final o = defaultGeneratorOptions.copyWith(symbolSet: set);
        expect(validateGeneratorOptions(o), isNotNull, reason: '"$set"');
        expect(() => generatePassword(o), throwsA(isA<GeneratorException>()));
      }
      // Only ambiguous symbols left.
      final o = defaultGeneratorOptions.copyWith(symbolSet: '|', avoidAmbiguous: true);
      expect(validateGeneratorOptions(o), isNotNull);
      expect(() => generatePassword(o), throwsA(isA<GeneratorException>()));
      // With symbols off the empty set does not matter.
      expect(validateGeneratorOptions(o.copyWith(symbols: false)), isNull);
    });

    test('length below the number of active classes', () {
      final o = defaultGeneratorOptions.copyWith(length: 3);
      expect(validateGeneratorOptions(o), isNotNull);
      expect(() => generatePassword(o), throwsA(isA<GeneratorException>()));
      expect(generatePassword(o.copyWith(length: 4)).runes.length, 4);
      expect(validateGeneratorOptions(o.copyWith(symbols: false)), isNull);
    });
  });

  test('over many runs every character of the set shows up', () {
    final seen = <int>{};
    final rng = Random(7); // seeded, so the test is deterministic
    for (var i = 0; i < 400; i++) {
      seen.addAll(generatePassword(defaultGeneratorOptions, random: rng).runes);
    }
    expect(seen, _cps(lowercaseChars + uppercaseChars + digitChars + defaultSymbolSet));

    final custom = <int>{};
    for (var i = 0; i < 100; i++) {
      custom.addAll(generatePassword(onlySymbols.copyWith(symbolSet: '😀€ß-'), random: rng).runes);
    }
    expect(custom, _cps('😀€ß-'));
  });

  test('randomIndex rejects the biased top of the 32-bit range', () {
    // 2^32 % 3 == 1, so 2^32 - 1 (0xFFFF:0xFFFF) must be drawn again.
    final rng = _ScriptedRandom([0xFFFF, 0xFFFF, 0x0000, 0x0005]);
    expect(randomIndex(rng, 3), 5 % 3);
    expect(rng.calls, 4);

    // Just below the limit is accepted.
    final ok = _ScriptedRandom([0xFFFF, 0xFFFE]);
    expect(randomIndex(ok, 3), 0xFFFFFFFE % 3);
    expect(ok.calls, 2);

    expect(() => randomIndex(Random(), 0), throwsArgumentError);
  });

  group('JSON', () {
    test('round-trips', () {
      const o = GeneratorOptions(
        length: 33,
        lowercase: false,
        uppercase: true,
        digits: false,
        symbols: true,
        symbolSet: '-_😀',
        avoidAmbiguous: true,
      );
      expect(GeneratorOptions.fromJson(o.toJson()), o);
      expect(o.toJson(), containsPair('symbol_set', '-_😀'));
      expect(o.toJson(), containsPair('avoid_ambiguous', true));
    });

    test('missing or mistyped keys fall back, the length is clamped', () {
      expect(GeneratorOptions.fromJson({}), defaultGeneratorOptions);
      final o = GeneratorOptions.fromJson({
        'length': '30',
        'lowercase': 'no',
        'digits': false,
        'symbol_set': 7,
      });
      expect(o, defaultGeneratorOptions.copyWith(digits: false));
      expect(GeneratorOptions.fromJson({'length': 2}).length, minGeneratorLength);
      expect(GeneratorOptions.fromJson({'length': 999}).length, maxGeneratorLength);
    });
  });

  test('describeGeneratorOptions', () {
    expect(describeGeneratorOptions(defaultGeneratorOptions),
        '20 characters · a-z A-Z 0-9 symbols');
    expect(
        describeGeneratorOptions(const GeneratorOptions(
            length: 12, uppercase: false, symbols: false, avoidAmbiguous: true)),
        '12 characters · a-z 0-9 · no ambiguous');
  });
}

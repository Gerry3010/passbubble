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

// Local password generator. Passwords are made on the device, so the server
// never sees them. The extension (packages/shared-ts/src/generator) and the
// CLI (cli/pkg/generator) follow the same rules: same defaults, symbol set
// cleaned and drawn in Unicode code points, the same ambiguous characters and
// the same three errors. Keep them in step.

import 'dart:math';

import 'package:flutter/foundation.dart';

const lowercaseChars = 'abcdefghijklmnopqrstuvwxyz';
const uppercaseChars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
const digitChars = '0123456789';

/// The symbol set backend and CLI have always used.
const defaultSymbolSet = r'!@#$%^&*()_+-=[]{}|;:,.<>?';

/// Removed from every class (symbols included) when avoiding ambiguous
/// characters.
const ambiguousChars = '0Oo1lI|';

/// Length range the UI offers.
const minGeneratorLength = 8;
const maxGeneratorLength = 128;

/// What the generator makes. Immutable; the UI edits copies.
@immutable
class GeneratorOptions {
  final int length;
  final bool lowercase;
  final bool uppercase;
  final bool digits;
  final bool symbols;

  /// The symbols to draw from, as the user typed it. Cleaned when generating
  /// (see [cleanSymbolSet]).
  final String symbolSet;
  final bool avoidAmbiguous;

  const GeneratorOptions({
    this.length = 20,
    this.lowercase = true,
    this.uppercase = true,
    this.digits = true,
    this.symbols = true,
    this.symbolSet = defaultSymbolSet,
    this.avoidAmbiguous = false,
  });

  GeneratorOptions copyWith({
    int? length,
    bool? lowercase,
    bool? uppercase,
    bool? digits,
    bool? symbols,
    String? symbolSet,
    bool? avoidAmbiguous,
  }) =>
      GeneratorOptions(
        length: length ?? this.length,
        lowercase: lowercase ?? this.lowercase,
        uppercase: uppercase ?? this.uppercase,
        digits: digits ?? this.digits,
        symbols: symbols ?? this.symbols,
        symbolSet: symbolSet ?? this.symbolSet,
        avoidAmbiguous: avoidAmbiguous ?? this.avoidAmbiguous,
      );

  Map<String, dynamic> toJson() => {
        'length': length,
        'lowercase': lowercase,
        'uppercase': uppercase,
        'digits': digits,
        'symbols': symbols,
        'symbol_set': symbolSet,
        'avoid_ambiguous': avoidAmbiguous,
      };

  /// Reads stored options. A missing or mistyped key falls back to its
  /// default, and the length is clamped to the UI range, so a damaged value
  /// never breaks the form.
  factory GeneratorOptions.fromJson(Map<String, dynamic> j) {
    const d = defaultGeneratorOptions;
    bool flag(String key, bool fallback) {
      final v = j[key];
      return v is bool ? v : fallback;
    }

    final length = j['length'];
    final symbolSet = j['symbol_set'];
    return GeneratorOptions(
      length: length is int
          ? min(max(length, minGeneratorLength), maxGeneratorLength)
          : d.length,
      lowercase: flag('lowercase', d.lowercase),
      uppercase: flag('uppercase', d.uppercase),
      digits: flag('digits', d.digits),
      symbols: flag('symbols', d.symbols),
      symbolSet: symbolSet is String ? symbolSet : d.symbolSet,
      avoidAmbiguous: flag('avoid_ambiguous', d.avoidAmbiguous),
    );
  }

  @override
  bool operator ==(Object other) =>
      other is GeneratorOptions &&
      other.length == length &&
      other.lowercase == lowercase &&
      other.uppercase == uppercase &&
      other.digits == digits &&
      other.symbols == symbols &&
      other.symbolSet == symbolSet &&
      other.avoidAmbiguous == avoidAmbiguous;

  @override
  int get hashCode => Object.hash(
      length, lowercase, uppercase, digits, symbols, symbolSet, avoidAmbiguous);
}

/// The factory defaults.
const defaultGeneratorOptions = GeneratorOptions();

/// Thrown by [generatePassword] for options that cannot produce a password;
/// [message] is meant for the user.
class GeneratorException implements Exception {
  final String message;
  const GeneratorException(this.message);

  @override
  String toString() => message;
}

final _whitespaceOrControl = RegExp(r'[\p{White_Space}\p{Cc}]', unicode: true);

/// [raw] without whitespace, control characters and repeats, in the order
/// typed. Works on code points, so characters outside the BMP stay whole.
String cleanSymbolSet(String raw) {
  final seen = <int>{};
  final kept = <int>[];
  for (final cp in raw.runes) {
    if (_whitespaceOrControl.hasMatch(String.fromCharCode(cp))) continue;
    if (seen.add(cp)) kept.add(cp);
  }
  return String.fromCharCodes(kept);
}

/// The active classes as code-point lists, or the reason there are none
/// usable.
({List<List<int>> classes, String? error}) _activeClasses(GeneratorOptions o) {
  final ambiguous = ambiguousChars.runes.toSet();
  List<int> usable(String chars) => [
        for (final cp in chars.runes)
          if (!o.avoidAmbiguous || !ambiguous.contains(cp)) cp,
      ];

  final classes = <List<int>>[];
  if (o.lowercase) classes.add(usable(lowercaseChars));
  if (o.uppercase) classes.add(usable(uppercaseChars));
  if (o.digits) classes.add(usable(digitChars));
  if (o.symbols) {
    final cleaned = cleanSymbolSet(o.symbolSet);
    if (cleaned.isEmpty) {
      return (classes: const [], error: 'Enter at least one symbol, or turn symbols off');
    }
    final symbols = usable(cleaned);
    if (symbols.isEmpty) {
      return (classes: const [], error: 'Every symbol in the set is ambiguous');
    }
    classes.add(symbols);
  }
  if (classes.isEmpty) {
    return (classes: const [], error: 'Select at least one character type');
  }
  if (o.length < classes.length) {
    return (
      classes: const [],
      error: 'Length must be at least ${classes.length} for the selected character types',
    );
  }
  return (classes: classes, error: null);
}

/// Why [options] cannot produce a password, or null when they can.
String? validateGeneratorOptions(GeneratorOptions options) =>
    _activeClasses(options).error;

/// A uniformly distributed index in `[0, n)`. Draws 32 random bits (as two
/// 16-bit halves, which also works for web ints) and rejects the top values
/// that would favour small indexes.
@visibleForTesting
int randomIndex(Random random, int n) {
  if (n <= 0) throw ArgumentError.value(n, 'n', 'must be positive');
  const range = 0x100000000; // 2^32
  final limit = range - range % n; // largest multiple of n not above 2^32
  while (true) {
    final v = random.nextInt(0x10000) * 0x10000 + random.nextInt(0x10000);
    if (v < limit) return v % n;
  }
}

/// A new password for [options]: one character from each active class, the
/// rest from all of them together, then shuffled (Fisher-Yates). [random]
/// defaults to [Random.secure]; tests pass a seeded one. Throws
/// [GeneratorException] for invalid options (see [validateGeneratorOptions]).
String generatePassword(GeneratorOptions options, {Random? random}) {
  final (:classes, :error) = _activeClasses(options);
  if (error != null) throw GeneratorException(error);
  final rng = random ?? Random.secure();

  final pool = <int>{for (final c in classes) ...c}.toList();
  final chars = <int>[
    for (final c in classes) c[randomIndex(rng, c.length)],
    for (var i = classes.length; i < options.length; i++)
      pool[randomIndex(rng, pool.length)],
  ];
  for (var i = chars.length - 1; i > 0; i--) {
    final j = randomIndex(rng, i + 1);
    final t = chars[i];
    chars[i] = chars[j];
    chars[j] = t;
  }
  return String.fromCharCodes(chars);
}

/// One line describing [o], e.g. "20 characters · a-z A-Z 0-9 symbols".
String describeGeneratorOptions(GeneratorOptions o) {
  final kinds = [
    if (o.lowercase) 'a-z',
    if (o.uppercase) 'A-Z',
    if (o.digits) '0-9',
    if (o.symbols) 'symbols',
  ];
  return [
    '${o.length} characters',
    if (kinds.isNotEmpty) kinds.join(' '),
    if (o.avoidAmbiguous) 'no ambiguous',
  ].join(' · ');
}

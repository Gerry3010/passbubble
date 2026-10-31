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

// Local password generator. The same spec is implemented in the app
// (flutter_app/lib/core/generator/password_generator.dart) and the CLI
// (cli/pkg/generator) — keep defaults, cleaning rules and error cases in sync.
// Passwords are generated on the client only; the server never sees them.

export interface GeneratorOptions {
  /** Length in Unicode code points. */
  length: number;
  lowercase: boolean;
  uppercase: boolean;
  digits: boolean;
  symbols: boolean;
  /** The symbols to draw from (cleaned before use, see cleanSymbolSet). */
  symbolSet: string;
  /** Leave out characters that are easy to mix up (AMBIGUOUS_CHARS). */
  avoidAmbiguous: boolean;
}

export const DEFAULT_SYMBOL_SET = '!@#$%^&*()_+-=[]{}|;:,.<>?';

export const DEFAULT_GENERATOR_OPTIONS: Readonly<GeneratorOptions> = Object.freeze({
  length: 20,
  lowercase: true,
  uppercase: true,
  digits: true,
  symbols: true,
  symbolSet: DEFAULT_SYMBOL_SET,
  avoidAmbiguous: false,
});

/** Length bounds the settings UIs offer. The generator itself only needs room
 * for one character per active class. */
export const GENERATOR_MIN_LENGTH = 8;
export const GENERATOR_MAX_LENGTH = 128;

/** Removed from every class (symbols included) when avoidAmbiguous is set. */
export const AMBIGUOUS_CHARS = '0Oo1lI|';

const LOWERCASE = 'abcdefghijklmnopqrstuvwxyz';
const UPPERCASE = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
const DIGITS = '0123456789';

// Whitespace and control characters never make it into a password. Unicode
// White_Space, not `\s` (which also matches U+FEFF), to match the app and CLI.
const UNUSABLE = /[\p{White_Space}\p{Cc}]/u;

/** Returns the symbol set without whitespace, control characters and
 * duplicates, keeping the first occurrence of each character. Works on code
 * points, so multi-byte characters such as `€` or emoji stay whole. */
export function cleanSymbolSet(symbolSet: string): string {
  const seen = new Set<string>();
  for (const ch of symbolSet) {
    if (!UNUSABLE.test(ch)) seen.add(ch);
  }
  return [...seen].join('');
}

interface CharClass {
  label: string;
  chars: string[];
}

// The active classes as code-point arrays, after cleaning and the ambiguity filter.
function activeClasses(options: GeneratorOptions): CharClass[] {
  const keep = (set: string) =>
    Array.from(set).filter((ch) => !options.avoidAmbiguous || !AMBIGUOUS_CHARS.includes(ch));
  const classes: CharClass[] = [];
  if (options.lowercase) classes.push({ label: 'lowercase letters', chars: keep(LOWERCASE) });
  if (options.uppercase) classes.push({ label: 'uppercase letters', chars: keep(UPPERCASE) });
  if (options.digits) classes.push({ label: 'digits', chars: keep(DIGITS) });
  if (options.symbols) classes.push({ label: 'symbols', chars: keep(cleanSymbolSet(options.symbolSet)) });
  return classes;
}

/** Returns why the options cannot produce a password, or null when they can. */
export function validateGeneratorOptions(options: GeneratorOptions): string | null {
  const classes = activeClasses(options);
  if (classes.length === 0) return 'Select at least one character type';
  const empty = classes.find((c) => c.chars.length === 0);
  if (empty) {
    return empty.label === 'symbols'
      ? 'The symbol set is empty — enter at least one symbol or turn symbols off'
      : `No ${empty.label} left to choose from`;
  }
  if (!Number.isInteger(options.length) || options.length < classes.length) {
    return `Length must be at least ${classes.length} (one character per selected type)`;
  }
  return null;
}

/** A uniformly distributed integer in [0, max), from crypto.getRandomValues.
 * Rejection sampling drops the top remainder of the 32-bit range so no value
 * is favoured by the modulo. */
export function secureRandomInt(max: number): number {
  if (!Number.isInteger(max) || max <= 0 || max > 0x100000000) {
    throw new RangeError('max must be an integer in 1..2^32');
  }
  const range = 0x100000000;
  const limit = range - (range % max);
  const buf = new Uint32Array(1);
  do {
    crypto.getRandomValues(buf);
  } while (buf[0] >= limit);
  return buf[0] % max;
}

/**
 * Generates a password: one character from every active class, the rest from
 * their union, then a Fisher-Yates shuffle. Throws an Error with the
 * validateGeneratorOptions message when the options are unusable.
 *
 * `randomInt(max)` must return an integer in [0, max); tests can inject a
 * deterministic source, everything else uses secureRandomInt.
 */
export function generatePassword(
  options: GeneratorOptions,
  randomInt: (max: number) => number = secureRandomInt,
): string {
  const error = validateGeneratorOptions(options);
  if (error) throw new Error(error);

  const classes = activeClasses(options);
  const pool = [...new Set(classes.flatMap((c) => c.chars))];

  const chars = classes.map((c) => c.chars[randomInt(c.chars.length)]);
  while (chars.length < options.length) chars.push(pool[randomInt(pool.length)]);

  for (let i = chars.length - 1; i > 0; i--) {
    const j = randomInt(i + 1);
    [chars[i], chars[j]] = [chars[j], chars[i]];
  }
  return chars.join('');
}

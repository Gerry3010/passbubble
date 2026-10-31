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

import { describe, it, expect, vi } from 'vitest';
import {
  AMBIGUOUS_CHARS,
  DEFAULT_GENERATOR_OPTIONS,
  DEFAULT_SYMBOL_SET,
  cleanSymbolSet,
  generatePassword,
  secureRandomInt,
  validateGeneratorOptions,
  type GeneratorOptions,
} from '../generator.js';

// Shared spec cases — the app and CLI generator tests cover the same list.

const opts = (o: Partial<GeneratorOptions> = {}): GeneratorOptions => ({ ...DEFAULT_GENERATOR_OPTIONS, ...o });
const codePoints = (s: string) => Array.from(s);

const LOWER = 'abcdefghijklmnopqrstuvwxyz';
const UPPER = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
const DIGITS = '0123456789';

describe('defaults', () => {
  it('match the shared spec', () => {
    expect(DEFAULT_GENERATOR_OPTIONS).toEqual({
      length: 20,
      lowercase: true,
      uppercase: true,
      digits: true,
      symbols: true,
      symbolSet: '!@#$%^&*()_+-=[]{}|;:,.<>?',
      avoidAmbiguous: false,
    });
    expect(DEFAULT_SYMBOL_SET).toBe('!@#$%^&*()_+-=[]{}|;:,.<>?');
    expect(AMBIGUOUS_CHARS).toBe('0Oo1lI|');
  });
});

describe('generatePassword', () => {
  it('has the requested length', () => {
    for (const length of [4, 8, 20, 64, 128]) {
      expect(codePoints(generatePassword(opts({ length })))).toHaveLength(length);
    }
  });

  it('only uses characters from the active classes', () => {
    const allowed = new Set(codePoints(LOWER + DIGITS));
    for (let i = 0; i < 50; i++) {
      const pw = generatePassword(opts({ uppercase: false, symbols: false, length: 32 }));
      for (const ch of codePoints(pw)) expect(allowed.has(ch)).toBe(true);
    }
  });

  it('includes every active class', () => {
    // Length 4 with four classes leaves no room for chance.
    for (let i = 0; i < 100; i++) {
      const pw = generatePassword(opts({ length: 4 }));
      expect(pw).toMatch(/[a-z]/);
      expect(pw).toMatch(/[A-Z]/);
      expect(pw).toMatch(/[0-9]/);
      expect(codePoints(pw).some((ch) => DEFAULT_SYMBOL_SET.includes(ch))).toBe(true);
    }
  });

  it('sticks to a custom symbol set', () => {
    for (let i = 0; i < 50; i++) {
      const pw = generatePassword(opts({ symbolSet: '-_', length: 30 }));
      const symbols = codePoints(pw).filter((ch) => !/[a-zA-Z0-9]/.test(ch));
      expect(symbols.length).toBeGreaterThan(0);
      for (const ch of symbols) expect('-_').toContain(ch);
    }
  });

  it('keeps multi-byte symbols whole', () => {
    const set = '€ß😀✓';
    const pw = generatePassword(opts({ lowercase: false, uppercase: false, digits: false, symbolSet: set, length: 40 }));
    expect(codePoints(pw)).toHaveLength(40);
    for (const ch of codePoints(pw)) expect(codePoints(set)).toContain(ch);
    // No lone surrogate halves from a split emoji.
    expect(pw).not.toMatch(/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/);
  });

  it('avoidAmbiguous also drops ambiguous symbols such as |', () => {
    for (let i = 0; i < 100; i++) {
      const pw = generatePassword(opts({ avoidAmbiguous: true, symbolSet: '|-', length: 40 }));
      for (const ch of AMBIGUOUS_CHARS) expect(pw).not.toContain(ch);
    }
  });

  it('eventually uses every character of the symbol set', () => {
    const set = codePoints(DEFAULT_SYMBOL_SET);
    const seen = new Set<string>();
    for (let i = 0; i < 300 && seen.size < set.length; i++) {
      const pw = generatePassword(opts({ lowercase: false, uppercase: false, digits: false, length: 32 }));
      for (const ch of codePoints(pw)) seen.add(ch);
    }
    expect([...seen].sort()).toEqual([...set].sort());
  });

  it('draws only from the injected random source', () => {
    const randomInt = vi.fn((max: number) => max - 1);
    const pw = generatePassword(opts({ uppercase: false, digits: false, symbols: false, length: 5 }), randomInt);
    expect(pw).toBe('zzzzz');
    // One pick per character plus one per Fisher-Yates step.
    expect(randomInt).toHaveBeenCalledTimes(5 + 4);
  });

  describe('errors', () => {
    it('no active class', () => {
      const o = opts({ lowercase: false, uppercase: false, digits: false, symbols: false });
      expect(validateGeneratorOptions(o)).toMatch(/at least one character type/i);
      expect(() => generatePassword(o)).toThrow(/at least one character type/i);
    });

    it('an active class that is empty after cleaning', () => {
      for (const o of [
        opts({ symbolSet: '' }),
        opts({ symbolSet: ' \t\n' }),
        opts({ symbolSet: '|', avoidAmbiguous: true }),
      ]) {
        expect(validateGeneratorOptions(o)).toMatch(/symbol set is empty/i);
        expect(() => generatePassword(o)).toThrow(/symbol set is empty/i);
      }
      // With symbols off, an empty set is fine.
      expect(validateGeneratorOptions(opts({ symbols: false, symbolSet: '' }))).toBeNull();
    });

    it('length below the number of active classes', () => {
      expect(validateGeneratorOptions(opts({ length: 3 }))).toMatch(/at least 4/);
      expect(() => generatePassword(opts({ length: 3 }))).toThrow(/at least 4/);
      expect(validateGeneratorOptions(opts({ length: 2, digits: false, symbols: false }))).toBeNull();
    });
  });
});

describe('cleanSymbolSet', () => {
  it('drops whitespace, control characters and duplicates, counting code points', () => {
    expect(cleanSymbolSet(' !\t!@\n\u0000@# €😀€😀 ')).toBe('!@#€😀');
  });
});

describe('secureRandomInt', () => {
  it('stays in range', () => {
    for (let i = 0; i < 1000; i++) {
      const n = secureRandomInt(7);
      expect(Number.isInteger(n) && n >= 0 && n < 7).toBe(true);
    }
  });

  it('rejects values from the biased top of the 32-bit range', () => {
    // 2^32 % 3 == 1, so 0xFFFFFFFF must be redrawn rather than folded to 0.
    const values = [0xffffffff, 5];
    const spy = vi.spyOn(crypto, 'getRandomValues').mockImplementation((buf) => {
      (buf as Uint32Array)[0] = values.shift()!;
      return buf;
    });
    expect(secureRandomInt(3)).toBe(2);
    expect(spy).toHaveBeenCalledTimes(2);
    spy.mockRestore();
  });

  it('rejects an invalid range', () => {
    expect(() => secureRandomInt(0)).toThrow(RangeError);
    expect(() => secureRandomInt(1.5)).toThrow(RangeError);
  });
});

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

// Password generator options (length, character types, symbol set, ambiguous
// characters). Shared by the popup, the options page and the fill box, so it
// stays narrow enough for the 320px fill iframe.

import { useEffect, useState } from 'react';
import {
  AMBIGUOUS_CHARS,
  GENERATOR_MAX_LENGTH,
  GENERATOR_MIN_LENGTH,
  type GeneratorOptions,
} from '@passbubble/shared-ts';
import { term, input } from './theme.js';

const CLASSES: { key: 'lowercase' | 'uppercase' | 'digits' | 'symbols'; label: string }[] = [
  { key: 'lowercase', label: 'Lowercase (a-z)' },
  { key: 'uppercase', label: 'Uppercase (A-Z)' },
  { key: 'digits', label: 'Digits (0-9)' },
  { key: 'symbols', label: 'Symbols' },
];

const labelStyle = {
  display: 'flex',
  alignItems: 'center',
  gap: '6px',
  fontSize: '12px',
  color: term.muted,
  fontFamily: term.font,
  cursor: 'pointer',
} as const;

const fieldStyle = { ...input, padding: '4px 6px', fontSize: '12px' };

export function GeneratorOptionsForm({
  value,
  onChange,
}: {
  value: GeneratorOptions;
  onChange: (next: GeneratorOptions) => void;
}) {
  // The number field keeps its own text while typing ("1" on the way to "16"
  // is not a valid length yet); only lengths in range reach onChange.
  const [lengthText, setLengthText] = useState(String(value.length));
  useEffect(() => setLengthText(String(value.length)), [value.length]);

  const set = <K extends keyof GeneratorOptions>(key: K, v: GeneratorOptions[K]) => onChange({ ...value, [key]: v });

  function typeLength(text: string) {
    setLengthText(text);
    const n = Number(text);
    if (Number.isInteger(n) && n >= GENERATOR_MIN_LENGTH && n <= GENERATOR_MAX_LENGTH) set('length', n);
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <span style={{ ...labelStyle, cursor: 'default' }}>Length</span>
        <input
          type="range"
          aria-label="Length"
          min={GENERATOR_MIN_LENGTH}
          max={GENERATOR_MAX_LENGTH}
          value={value.length}
          onChange={(e) => set('length', Number(e.target.value))}
          style={{ flex: 1, minWidth: 0, accentColor: term.green }}
        />
        <input
          type="number"
          aria-label="Length in characters"
          min={GENERATOR_MIN_LENGTH}
          max={GENERATOR_MAX_LENGTH}
          value={lengthText}
          onChange={(e) => typeLength(e.target.value)}
          onBlur={() => setLengthText(String(value.length))}
          style={{ ...fieldStyle, width: '56px' }}
        />
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '4px 8px' }}>
        {CLASSES.map((c) => (
          <label key={c.key} style={labelStyle}>
            <input
              type="checkbox"
              checked={value[c.key]}
              onChange={(e) => set(c.key, e.target.checked)}
              style={{ accentColor: term.green }}
            />
            {c.label}
          </label>
        ))}
      </div>

      <input
        type="text"
        aria-label="Symbol set"
        placeholder="Symbols to use"
        value={value.symbolSet}
        disabled={!value.symbols}
        onChange={(e) => set('symbolSet', e.target.value)}
        spellCheck={false}
        autoComplete="off"
        style={{ ...fieldStyle, opacity: value.symbols ? 1 : 0.5 }}
      />

      <label style={labelStyle}>
        <input
          type="checkbox"
          checked={value.avoidAmbiguous}
          onChange={(e) => set('avoidAmbiguous', e.target.checked)}
          style={{ accentColor: term.green }}
        />
        Avoid ambiguous ({AMBIGUOUS_CHARS.split('').join(' ')})
      </label>
    </div>
  );
}

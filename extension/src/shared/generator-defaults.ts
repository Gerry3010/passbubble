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

// The password generator defaults, stored in storage.sync and edited only on
// the options page. Every generator (popup, create dialog, fill box,
// background GENERATE) starts from them; changes made in a dialog are not
// written back.

import browser from 'webextension-polyfill';
import {
  DEFAULT_GENERATOR_OPTIONS,
  GENERATOR_MAX_LENGTH,
  GENERATOR_MIN_LENGTH,
  type GeneratorOptions,
} from '@passbubble/shared-ts';
import { STORAGE_KEYS } from './constants.js';

const FLAGS = ['lowercase', 'uppercase', 'digits', 'symbols', 'avoidAmbiguous'] as const;

/** Clamps a length to the range the UI offers. */
export function clampGeneratorLength(n: number): number {
  return Math.min(GENERATOR_MAX_LENGTH, Math.max(GENERATOR_MIN_LENGTH, Math.round(n)));
}

/** Overlays the well-typed fields of an untrusted object (a stored value or a
 * message payload) on `base`; anything missing or malformed keeps base's value. */
export function mergeGeneratorOptions(base: Readonly<GeneratorOptions>, raw: unknown): GeneratorOptions {
  const out: GeneratorOptions = { ...base };
  if (!raw || typeof raw !== 'object') return out;
  const r = raw as Record<string, unknown>;
  if (typeof r.length === 'number' && Number.isFinite(r.length)) out.length = clampGeneratorLength(r.length);
  for (const key of FLAGS) {
    const value = r[key];
    if (typeof value === 'boolean') out[key] = value;
  }
  if (typeof r.symbolSet === 'string') out.symbolSet = r.symbolSet;
  return out;
}

/** The stored generator defaults. Falls back to the factory defaults if unset
 * or unreadable, and per field if the stored value is malformed. */
export async function readGeneratorDefaults(): Promise<GeneratorOptions> {
  try {
    const data = await browser.storage.sync.get(STORAGE_KEYS.GENERATOR_DEFAULTS);
    return mergeGeneratorOptions(DEFAULT_GENERATOR_OPTIONS, data[STORAGE_KEYS.GENERATOR_DEFAULTS]);
  } catch {
    return { ...DEFAULT_GENERATOR_OPTIONS };
  }
}

export async function saveGeneratorDefaults(options: GeneratorOptions): Promise<void> {
  await browser.storage.sync.set({ [STORAGE_KEYS.GENERATOR_DEFAULTS]: { ...options } });
}

/** Drops the stored defaults, so readers get the factory defaults again. */
export async function resetGeneratorDefaults(): Promise<void> {
  await browser.storage.sync.remove(STORAGE_KEYS.GENERATOR_DEFAULTS);
}

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

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { DEFAULT_GENERATOR_OPTIONS, DEFAULT_SYMBOL_SET } from '@passbubble/shared-ts';

const syncGet = vi.hoisted(() => vi.fn());
const syncSet = vi.hoisted(() => vi.fn());
const syncRemove = vi.hoisted(() => vi.fn());
vi.mock('webextension-polyfill', () => ({
  default: { storage: { sync: { get: syncGet, set: syncSet, remove: syncRemove } } },
}));

import { GeneratorSection } from '../components/GeneratorSection.js';
import { readGeneratorDefaults } from '../../shared/generator-defaults.js';
import { STORAGE_KEYS } from '../../shared/constants.js';

const KEY = STORAGE_KEYS.GENERATOR_DEFAULTS;

const lengthSlider = () => screen.getByRole('slider', { name: 'Length' }) as HTMLInputElement;
const symbolSet = () => screen.getByRole('textbox', { name: 'Symbol set' }) as HTMLInputElement;
const saveButton = () => screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement;

async function renderSection() {
  render(<GeneratorSection />);
  await screen.findByRole('slider', { name: 'Length' });
}

describe('GeneratorSection', () => {
  beforeEach(() => {
    syncGet.mockResolvedValue({});
    syncSet.mockResolvedValue(undefined);
    syncRemove.mockResolvedValue(undefined);
  });

  it('shows the stored defaults', async () => {
    syncGet.mockResolvedValue({ [KEY]: { length: 40, symbolSet: '-_', avoidAmbiguous: true } });
    await renderSection();
    expect(lengthSlider().value).toBe('40');
    expect(symbolSet().value).toBe('-_');
    expect((screen.getByRole('checkbox', { name: /avoid ambiguous/i }) as HTMLInputElement).checked).toBe(true);
    expect(screen.getByRole('status', { name: 'Generated password' }).textContent).toMatch(/^[^0Oo1lI|]{40}$/);
  });

  it('saves the edited defaults to storage.sync', async () => {
    await renderSection();
    expect(saveButton().disabled).toBe(true);

    fireEvent.change(lengthSlider(), { target: { value: '32' } });
    fireEvent.change(symbolSet(), { target: { value: '!?' } });
    fireEvent.click(screen.getByRole('checkbox', { name: /digits/i }));
    expect(saveButton().disabled).toBe(false);

    await act(async () => fireEvent.click(saveButton()));

    expect(syncSet).toHaveBeenCalledWith({
      [KEY]: { ...DEFAULT_GENERATOR_OPTIONS, length: 32, symbolSet: '!?', digits: false },
    });
    expect(screen.getByText('Saved.')).toBeDefined();
    expect(saveButton().disabled).toBe(true);
  });

  it('round-trips: what is saved is what readGeneratorDefaults returns', async () => {
    await renderSection();
    fireEvent.change(lengthSlider(), { target: { value: '50' } });
    await act(async () => fireEvent.click(saveButton()));

    const stored = syncSet.mock.calls[0][0];
    syncGet.mockResolvedValue(stored);
    expect(await readGeneratorDefaults()).toEqual({ ...DEFAULT_GENERATOR_OPTIONS, length: 50 });
  });

  it('refuses to save options that cannot produce a password', async () => {
    await renderSection();
    fireEvent.change(symbolSet(), { target: { value: '' } });
    expect(screen.getByRole('status', { name: 'Generated password' }).textContent).toMatch(/symbol set is empty/i);
    expect(saveButton().disabled).toBe(true);
  });

  it('Reset to defaults drops the stored value and shows the factory defaults', async () => {
    syncGet.mockResolvedValue({ [KEY]: { length: 64, symbols: false, symbolSet: '-' } });
    await renderSection();

    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Reset to defaults' })));

    expect(syncRemove).toHaveBeenCalledWith(KEY);
    expect(lengthSlider().value).toBe('20');
    expect(symbolSet().value).toBe(DEFAULT_SYMBOL_SET);
    expect((screen.getByRole('checkbox', { name: /symbols/i }) as HTMLInputElement).checked).toBe(true);
    expect(saveButton().disabled).toBe(true);
  });

  it('shows why saving failed', async () => {
    syncSet.mockRejectedValue(new Error('QUOTA_BYTES_PER_ITEM quota exceeded'));
    await renderSection();
    fireEvent.change(lengthSlider(), { target: { value: '30' } });
    await act(async () => fireEvent.click(saveButton()));
    expect(screen.getByText(/could not save: .*quota/i)).toBeDefined();
  });
});

describe('readGeneratorDefaults', () => {
  it('falls back to the factory defaults per field and clamps the length', async () => {
    syncGet.mockResolvedValue({ [KEY]: { length: 999, digits: 'no', symbolSet: 42, uppercase: false } });
    expect(await readGeneratorDefaults()).toEqual({ ...DEFAULT_GENERATOR_OPTIONS, length: 128, uppercase: false });
  });

  it('falls back to the factory defaults when storage is unavailable', async () => {
    syncGet.mockRejectedValue(new Error('no storage'));
    expect(await readGeneratorDefaults()).toEqual(DEFAULT_GENERATOR_OPTIONS);
  });
});

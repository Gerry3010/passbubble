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

const syncGet = vi.hoisted(() => vi.fn());
const syncSet = vi.hoisted(() => vi.fn());
const sendMessage = vi.hoisted(() => vi.fn());
vi.mock('webextension-polyfill', () => ({
  default: {
    runtime: { sendMessage, openOptionsPage: vi.fn() },
    storage: { sync: { get: syncGet, set: syncSet } },
  },
}));

import { GeneratorPanel } from '../components/GeneratorPanel.js';
import { STORAGE_KEYS } from '../../shared/constants.js';

const codePoints = (s: string) => Array.from(s);

async function renderPanel() {
  render(<GeneratorPanel />);
  return screen.findByRole('status', { name: 'Generated password' });
}

function shownPassword(): string {
  return screen.getByRole('status', { name: 'Generated password' }).textContent ?? '';
}

describe('GeneratorPanel', () => {
  beforeEach(() => {
    syncGet.mockResolvedValue({});
    sendMessage.mockRejectedValue(new Error('the generator must not ask the background'));
  });

  it('generates locally on open, with the factory defaults', async () => {
    await renderPanel();
    const pw = shownPassword();
    expect(codePoints(pw)).toHaveLength(20);
    expect(pw).toMatch(/[a-z]/);
    expect(pw).toMatch(/[A-Z]/);
    expect(pw).toMatch(/[0-9]/);
    expect(sendMessage).not.toHaveBeenCalled();
    expect(screen.getByText(/strong|weak|moderate/i)).toBeDefined();
  });

  it('starts from the stored defaults', async () => {
    syncGet.mockResolvedValue({
      [STORAGE_KEYS.GENERATOR_DEFAULTS]: { length: 32, uppercase: false, symbols: false },
    });
    await renderPanel();
    const pw = shownPassword();
    expect(pw).toMatch(/^[a-z0-9]{32}$/);
    expect((screen.getByRole('slider', { name: 'Length' }) as HTMLInputElement).value).toBe('32');
    expect((screen.getByRole('checkbox', { name: /uppercase/i }) as HTMLInputElement).checked).toBe(false);
  });

  it('Regenerate makes a new password', async () => {
    await renderPanel();
    const first = shownPassword();
    fireEvent.click(screen.getByRole('button', { name: /regenerate/i }));
    expect(shownPassword()).not.toBe(first);
  });

  it('regenerates on every option change and follows the options', async () => {
    await renderPanel();
    fireEvent.change(screen.getByRole('slider', { name: 'Length' }), { target: { value: '40' } });
    expect(codePoints(shownPassword())).toHaveLength(40);

    fireEvent.click(screen.getByRole('checkbox', { name: /symbols/i }));
    fireEvent.click(screen.getByRole('checkbox', { name: /uppercase/i }));
    expect(shownPassword()).toMatch(/^[a-z0-9]{40}$/);
  });

  it('uses only the symbols from the symbol set', async () => {
    await renderPanel();
    fireEvent.change(screen.getByRole('textbox', { name: 'Symbol set' }), { target: { value: '-_' } });
    const symbols = codePoints(shownPassword()).filter((ch) => !/[a-zA-Z0-9]/.test(ch));
    expect(symbols.length).toBeGreaterThan(0);
    for (const ch of symbols) expect('-_').toContain(ch);
  });

  it('says so when the symbol set is empty, and offers nothing to copy', async () => {
    await renderPanel();
    fireEvent.change(screen.getByRole('textbox', { name: 'Symbol set' }), { target: { value: '  ' } });
    expect(shownPassword()).toMatch(/symbol set is empty/i);
    expect((screen.getByRole('button', { name: /copy to clipboard/i }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('does not save option changes as defaults', async () => {
    await renderPanel();
    fireEvent.change(screen.getByRole('slider', { name: 'Length' }), { target: { value: '64' } });
    expect(syncSet).not.toHaveBeenCalled();
  });

  it('copies the shown password and confirms', async () => {
    await renderPanel();
    const pw = shownPassword();
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /copy to clipboard/i }));
    });
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(pw);
    expect(screen.getByRole('button', { name: /copied!/i })).toBeDefined();
  });
});

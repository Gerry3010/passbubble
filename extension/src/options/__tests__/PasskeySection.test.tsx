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

const local = vi.hoisted(() => new Map<string, unknown>());
const platform = vi.hoisted(() => ({ os: 'linux', base: 'chrome-extension://ext/' }));
vi.mock('webextension-polyfill', () => ({
  default: {
    storage: {
      local: {
        get: vi.fn(async (keys: string[]) => Object.fromEntries(keys.filter((k) => local.has(k)).map((k) => [k, local.get(k)]))),
        set: vi.fn(async (obj: Record<string, unknown>) => {
          for (const [k, v] of Object.entries(obj)) local.set(k, v);
        }),
      },
    },
    runtime: { getPlatformInfo: vi.fn(async () => ({ os: platform.os })), getURL: (p: string) => platform.base + p },
  },
}));

import { PasskeySection } from '../components/PasskeySection.js';
import { STORAGE_KEYS } from '../../shared/constants.js';
import { normalizeSiteEntry, siteListed } from '../../shared/passkey-settings.js';

const toggle = () => screen.getByRole('checkbox', { name: /use passbubble for passkeys/i }) as HTMLInputElement;

beforeEach(() => {
  local.clear();
  platform.os = 'linux';
  platform.base = 'chrome-extension://ext/';
});

describe('PasskeySection', () => {
  it('is on by default and can be turned off', async () => {
    render(<PasskeySection />);
    expect((await screen.findByRole('checkbox')).getAttribute('type')).toBe('checkbox');
    expect(toggle().checked).toBe(true);
    await act(async () => fireEvent.click(toggle()));
    expect(local.get(STORAGE_KEYS.PASSKEYS_ENABLED)).toBe(false);
    expect(toggle().checked).toBe(false);
  });

  it('shows the stored state', async () => {
    local.set(STORAGE_KEYS.PASSKEYS_ENABLED, false);
    local.set(STORAGE_KEYS.PASSKEY_BLOCKLIST, ['bank.example']);
    render(<PasskeySection />);
    await screen.findByRole('checkbox');
    expect(toggle().checked).toBe(false);
    expect(screen.getByText('bank.example')).toBeDefined();
  });

  it('adds sites as hosts and removes them again', async () => {
    render(<PasskeySection />);
    const input = await screen.findByRole('textbox', { name: /site to exclude/i });
    fireEvent.change(input, { target: { value: 'https://www.Bank.example/login' } });
    await act(async () => fireEvent.submit(input.closest('form')!));
    expect(local.get(STORAGE_KEYS.PASSKEY_BLOCKLIST)).toEqual(['bank.example']);
    fireEvent.change(input, { target: { value: 'bank.example' } });
    await act(async () => fireEvent.submit(input.closest('form')!));
    expect(local.get(STORAGE_KEYS.PASSKEY_BLOCKLIST)).toEqual(['bank.example']);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Remove bank.example' })));
    expect(local.get(STORAGE_KEYS.PASSKEY_BLOCKLIST)).toEqual([]);
  });

  it('refuses input that is not a host', async () => {
    render(<PasskeySection />);
    const input = await screen.findByRole('textbox', { name: /site to exclude/i });
    fireEvent.change(input, { target: { value: '   ' } });
    await act(async () => fireEvent.submit(input.closest('form')!));
    expect(screen.getByText(/enter a host or domain/i)).toBeDefined();
    expect(local.has(STORAGE_KEYS.PASSKEY_BLOCKLIST)).toBe(false);
  });

  it('says the feature is unavailable on Firefox for Android', async () => {
    platform.os = 'android';
    render(<PasskeySection />);
    expect(await screen.findByText(/not available in firefox for android/i)).toBeDefined();
    expect(screen.queryByRole('checkbox')).toBeNull();
  });

  it('says the feature is unavailable in Safari', async () => {
    platform.base = 'safari-web-extension://ABCD/';
    render(<PasskeySection />);
    expect(await screen.findByText(/not available in safari/i)).toBeDefined();
    expect(screen.queryByRole('checkbox')).toBeNull();
  });
});

describe('site list matching', () => {
  it('covers the site and its subdomains, ignoring www', () => {
    expect(normalizeSiteEntry(' https://WWW.Example.com:8443/x ')).toBe('example.com');
    expect(siteListed('login.example.com', ['example.com'])).toBe(true);
    expect(siteListed('www.example.com', ['example.com'])).toBe(true);
    expect(siteListed('example.com', ['login.example.com'])).toBe(false);
    expect(siteListed('notexample.com', ['example.com'])).toBe(false);
  });
});

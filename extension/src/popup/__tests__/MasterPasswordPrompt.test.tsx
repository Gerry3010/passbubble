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
import { MasterPasswordPrompt } from '../components/MasterPasswordPrompt.js';

vi.mock('../store/session.js', () => ({
  useSessionStore: vi.fn(),
}));

import { useSessionStore } from '../store/session.js';

const mockUseStore = useSessionStore as ReturnType<typeof vi.fn>;

function defaultStore(overrides = {}) {
  return {
    unlock: vi.fn().mockResolvedValue(undefined),
    unlockWithPin: vi.fn().mockResolvedValue(undefined),
    logout: vi.fn().mockResolvedValue(undefined),
    pinAvailable: false,
    isLoading: false,
    error: null,
    ...overrides,
  };
}

describe('MasterPasswordPrompt', () => {
  beforeEach(() => {
    mockUseStore.mockReturnValue(defaultStore());
  });

  it('renders the master password input', () => {
    render(<MasterPasswordPrompt />);
    expect(screen.getByPlaceholderText('Master password')).toBeDefined();
  });

  it('renders the Unlock button', () => {
    render(<MasterPasswordPrompt />);
    expect(screen.getByRole('button', { name: /unlock/i })).toBeDefined();
  });

  it('calls unlock with the entered master password on submit', async () => {
    const unlockMock = vi.fn().mockResolvedValue(undefined);
    mockUseStore.mockReturnValue(defaultStore({ unlock: unlockMock }));

    render(<MasterPasswordPrompt />);

    fireEvent.change(screen.getByPlaceholderText('Master password'), {
      target: { value: 'mysecretpassphrase' },
    });

    await act(async () => {
      fireEvent.submit(
        screen.getByRole('button', { name: /unlock/i }).closest('form')!,
      );
    });

    expect(unlockMock).toHaveBeenCalledWith('mysecretpassphrase');
  });

  it('clears the password field after submit', async () => {
    render(<MasterPasswordPrompt />);

    const input = screen.getByPlaceholderText('Master password') as HTMLInputElement;
    fireEvent.change(input, { target: { value: 'mysecretpassphrase' } });
    expect(input.value).toBe('mysecretpassphrase');

    await act(async () => {
      fireEvent.submit(input.closest('form')!);
    });

    expect(input.value).toBe('');
  });

  it('shows error message when error is set', () => {
    mockUseStore.mockReturnValue(defaultStore({ error: 'Wrong master password' }));
    render(<MasterPasswordPrompt />);
    expect(screen.getByText('Wrong master password')).toBeDefined();
  });

  it('disables the button and shows loading text while loading', () => {
    mockUseStore.mockReturnValue(defaultStore({ isLoading: true }));
    render(<MasterPasswordPrompt />);
    const btn = screen.getByRole('button', { name: /unlock/i }) as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    expect(btn.textContent).toMatch(/unlocking/i);
  });

  it('renders a log out button that calls logout', () => {
    const logoutMock = vi.fn().mockResolvedValue(undefined);
    mockUseStore.mockReturnValue(defaultStore({ logout: logoutMock }));
    render(<MasterPasswordPrompt />);
    fireEvent.click(screen.getByRole('button', { name: /log out/i }));
    expect(logoutMock).toHaveBeenCalled();
  });
});

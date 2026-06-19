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
import { LoginForm } from '../components/LoginForm.js';

// Mock the Zustand store — LoginForm only needs login, isLoading, error
vi.mock('../store/session.js', () => ({
  useSessionStore: vi.fn(),
}));

// LoginForm persists/restores its draft via browser.storage.session.
vi.mock('webextension-polyfill', () => ({
  default: {
    storage: {
      session: {
        get: vi.fn().mockResolvedValue({}),
        set: vi.fn().mockResolvedValue(undefined),
        remove: vi.fn().mockResolvedValue(undefined),
      },
    },
  },
}));

import { useSessionStore } from '../store/session.js';

const mockUseStore = useSessionStore as ReturnType<typeof vi.fn>;

function defaultStore(overrides = {}) {
  return {
    login: vi.fn().mockResolvedValue(undefined),
    isLoading: false,
    error: null,
    ...overrides,
  };
}

describe('LoginForm', () => {
  beforeEach(() => {
    mockUseStore.mockReturnValue(defaultStore());
  });

  it('renders email and password fields', () => {
    render(<LoginForm />);
    expect(screen.getByPlaceholderText('Email')).toBeDefined();
    expect(screen.getByPlaceholderText('Password')).toBeDefined();
  });

  it('renders the Sign in button', () => {
    render(<LoginForm />);
    expect(screen.getByRole('button', { name: /sign in/i })).toBeDefined();
  });

  it('calls login with email and password on submit', async () => {
    const loginMock = vi.fn().mockResolvedValue(undefined);
    mockUseStore.mockReturnValue(defaultStore({ login: loginMock }));

    render(<LoginForm />);

    fireEvent.change(screen.getByPlaceholderText('Email'), {
      target: { value: 'alice@example.com' },
    });
    fireEvent.change(screen.getByPlaceholderText('Password'), {
      target: { value: 'hunter2' },
    });

    await act(async () => {
      fireEvent.submit(screen.getByRole('button', { name: /sign in/i }).closest('form')!);
    });

    expect(loginMock).toHaveBeenCalledWith('alice@example.com', 'hunter2');
  });

  it('shows error message when error is set', () => {
    mockUseStore.mockReturnValue(defaultStore({ error: 'Invalid credentials' }));
    render(<LoginForm />);
    expect(screen.getByText('Invalid credentials')).toBeDefined();
  });

  it('disables the button and shows loading text while loading', () => {
    mockUseStore.mockReturnValue(defaultStore({ isLoading: true }));
    render(<LoginForm />);
    const btn = screen.getByRole('button') as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    expect(btn.textContent).toMatch(/signing in/i);
  });
});

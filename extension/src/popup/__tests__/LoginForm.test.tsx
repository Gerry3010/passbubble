import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { LoginForm } from '../components/LoginForm.js';

// Mock the Zustand store — LoginForm only needs login, isLoading, error
vi.mock('../store/session.js', () => ({
  useSessionStore: vi.fn(),
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

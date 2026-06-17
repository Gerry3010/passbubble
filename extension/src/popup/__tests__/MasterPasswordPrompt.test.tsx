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
    const btn = screen.getByRole('button') as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    expect(btn.textContent).toMatch(/unlocking/i);
  });
});

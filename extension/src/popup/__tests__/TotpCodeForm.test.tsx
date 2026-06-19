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
import { TotpCodeForm } from '../components/TotpCodeForm.js';

vi.mock('../store/session.js', () => ({
  useSessionStore: vi.fn(),
}));

import { useSessionStore } from '../store/session.js';

const mockUseStore = useSessionStore as ReturnType<typeof vi.fn>;

function defaultStore(overrides = {}) {
  return {
    verifyTotp: vi.fn().mockResolvedValue(undefined),
    cancelTotp: vi.fn(),
    isLoading: false,
    error: null,
    ...overrides,
  };
}

describe('TotpCodeForm', () => {
  beforeEach(() => {
    mockUseStore.mockReturnValue(defaultStore());
  });

  it('calls verifyTotp with the trimmed code on submit', async () => {
    const verify = vi.fn().mockResolvedValue(undefined);
    mockUseStore.mockReturnValue(defaultStore({ verifyTotp: verify }));

    render(<TotpCodeForm />);
    fireEvent.change(screen.getByPlaceholderText('123456'), {
      target: { value: ' 123456 ' },
    });
    await act(async () => {
      fireEvent.submit(screen.getByRole('button', { name: /verify/i }).closest('form')!);
    });
    expect(verify).toHaveBeenCalledWith('123456');
  });

  it('calls cancelTotp when "Back to sign in" is clicked', () => {
    const cancel = vi.fn();
    mockUseStore.mockReturnValue(defaultStore({ cancelTotp: cancel }));
    render(<TotpCodeForm />);
    fireEvent.click(screen.getByRole('button', { name: /back to sign in/i }));
    expect(cancel).toHaveBeenCalled();
  });

  it('shows an error when verification fails', () => {
    mockUseStore.mockReturnValue(defaultStore({ error: 'invalid code' }));
    render(<TotpCodeForm />);
    expect(screen.getByText('invalid code')).toBeDefined();
  });
});

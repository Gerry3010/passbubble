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
import { GeneratorPanel } from '../components/GeneratorPanel.js';

vi.mock('webextension-polyfill', () => ({
  default: {
    runtime: { sendMessage: vi.fn() },
  },
}));

import browser from 'webextension-polyfill';

const mockSendMessage = browser.runtime.sendMessage as ReturnType<typeof vi.fn>;

describe('GeneratorPanel', () => {
  beforeEach(() => {
    mockSendMessage.mockResolvedValue({
      passwords: [{ password: 'Xk!2@mN9#p', strength: 88 }],
    });
  });

  it('renders the Generate button', () => {
    render(<GeneratorPanel />);
    expect(screen.getByRole('button', { name: /generate/i })).toBeDefined();
  });

  it('sends a GENERATE message to the SW when Generate is clicked', async () => {
    render(<GeneratorPanel />);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /generate/i }));
    });

    expect(mockSendMessage).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'GENERATE' }),
    );
  });

  it('displays the generated password after clicking Generate', async () => {
    render(<GeneratorPanel />);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /generate/i }));
    });
    expect(screen.getByText('Xk!2@mN9#p')).toBeDefined();
  });

  it('renders a strength meter with the returned strength value', async () => {
    render(<GeneratorPanel />);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /generate/i }));
    });
    expect(screen.getByText('88/100')).toBeDefined();
  });

  it('copies the generated password to clipboard when Copy is clicked', async () => {
    render(<GeneratorPanel />);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /generate/i }));
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /copy to clipboard/i }));
    });
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('Xk!2@mN9#p');
  });

  it('shows Copied! text briefly after copying', async () => {
    render(<GeneratorPanel />);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /generate/i }));
    });
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /copy to clipboard/i }));
    });
    expect(screen.getByRole('button', { name: /copied!/i })).toBeDefined();
  });

  it('disables the Generate button and shows loading text while waiting', async () => {
    let resolve: (v: unknown) => void;
    mockSendMessage.mockReturnValue(new Promise((r) => { resolve = r; }));

    render(<GeneratorPanel />);
    fireEvent.click(screen.getByRole('button', { name: /generate/i }));

    const btn = screen.getByRole('button') as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    expect(btn.textContent).toMatch(/generating/i);

    await act(async () => {
      resolve!({ passwords: [{ password: 'abc', strength: 60 }] });
    });
  });

  it('includes the length and symbols settings in the GENERATE payload', async () => {
    render(<GeneratorPanel />);

    // Change length slider to 32
    fireEvent.change(screen.getByRole('slider'), { target: { value: '32' } });

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /generate/i }));
    });

    expect(mockSendMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        payload: expect.objectContaining({ length: 32 }),
      }),
    );
  });
});

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
import { render, screen, fireEvent, act, waitFor } from '@testing-library/react';

const sendMessage = vi.hoisted(() => vi.fn());
vi.mock('webextension-polyfill', () => ({ default: { runtime: { sendMessage } } }));

import { CreateEntryForm } from '../components/CreateEntryForm.js';
import { MessageType } from '../../shared/constants.js';

describe('CreateEntryForm', () => {
  beforeEach(() => sendMessage.mockReset());

  it('requires a name', async () => {
    const onCreated = vi.fn();
    render(<CreateEntryForm onCreated={onCreated} onCancel={() => {}} />);
    await act(async () => {
      fireEvent.submit(screen.getByRole('button', { name: /save entry/i }).closest('form')!);
    });
    expect(screen.getByText(/name is required/i)).toBeDefined();
    expect(onCreated).not.toHaveBeenCalled();
  });

  it('sends CREATE_ENTRY and calls onCreated on success', async () => {
    sendMessage.mockResolvedValue({ id: 'e1' });
    const onCreated = vi.fn();
    render(<CreateEntryForm onCreated={onCreated} onCancel={() => {}} />);

    fireEvent.change(screen.getByPlaceholderText('Name'), { target: { value: 'GitHub' } });
    fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'octocat' } });
    fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'pw' } });

    await act(async () => {
      fireEvent.submit(screen.getByRole('button', { name: /save entry/i }).closest('form')!);
    });

    expect(sendMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        type: MessageType.CREATE_ENTRY,
        payload: expect.objectContaining({
          name: 'GitHub',
          type: 'password',
          data: { username: 'octocat', password: 'pw' },
        }),
      }),
    );
    await waitFor(() => expect(onCreated).toHaveBeenCalled());
  });

  it('fills the password from the generator', async () => {
    sendMessage.mockResolvedValue({ passwords: [{ password: 'Gen3rated!', strength: 90 }] });
    render(<CreateEntryForm onCreated={() => {}} onCancel={() => {}} />);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /generate/i }));
    });
    await waitFor(() =>
      expect((screen.getByPlaceholderText('Password') as HTMLInputElement).value).toBe('Gen3rated!'),
    );
  });
});

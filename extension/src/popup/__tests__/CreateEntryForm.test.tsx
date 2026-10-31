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
const tabsQuery = vi.hoisted(() => vi.fn().mockResolvedValue([]));
vi.mock('webextension-polyfill', () => ({
  default: { runtime: { sendMessage }, tabs: { query: tabsQuery } },
}));

import { CreateEntryForm } from '../components/CreateEntryForm.js';
import { MessageType } from '../../shared/constants.js';

describe('CreateEntryForm', () => {
  beforeEach(() => {
    sendMessage.mockReset();
    tabsQuery.mockReset().mockResolvedValue([]);
  });

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

  it('pre-fills URL (origin) and name (title) from the active web tab', async () => {
    tabsQuery.mockResolvedValue([{ url: 'https://www.example.com/login?x=1', title: 'Example Login' }]);
    await act(async () => {
      render(<CreateEntryForm onCreated={() => {}} onCancel={() => {}} />);
    });
    await waitFor(() =>
      expect((screen.getByPlaceholderText('URL') as HTMLInputElement).value).toBe('https://www.example.com'),
    );
    expect((screen.getByPlaceholderText('Name') as HTMLInputElement).value).toBe('Example Login');
  });

  it('does not pre-fill on non-web tabs (chrome://)', async () => {
    tabsQuery.mockResolvedValue([{ url: 'chrome://extensions', title: 'Extensions' }]);
    await act(async () => {
      render(<CreateEntryForm onCreated={() => {}} onCancel={() => {}} />);
    });
    await waitFor(() => expect(tabsQuery).toHaveBeenCalled());
    expect((screen.getByPlaceholderText('URL') as HTMLInputElement).value).toBe('');
    expect((screen.getByPlaceholderText('Name') as HTMLInputElement).value).toBe('');
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

  describe('tags', () => {
    async function submitWithTags(value: string) {
      fireEvent.change(screen.getByPlaceholderText('Name'), { target: { value: 'GitHub' } });
      fireEvent.change(screen.getByRole('textbox', { name: 'Tags' }), { target: { value } });
      await act(async () => {
        fireEvent.submit(screen.getByRole('button', { name: /save entry/i }).closest('form')!);
      });
    }

    it('sends the comma-separated tags normalised', async () => {
      sendMessage.mockResolvedValue({ id: 'e1' });
      render(<CreateEntryForm onCreated={() => {}} onCancel={() => {}} />);
      await submitWithTags(' #Work, client,, work ,  my   team ');

      expect(sendMessage).toHaveBeenCalledWith(
        expect.objectContaining({
          type: MessageType.CREATE_ENTRY,
          payload: expect.objectContaining({ data: { tags: ['Work', 'client', 'my team'] } }),
        }),
      );
    });

    it('sends tags for non-login types too', async () => {
      sendMessage.mockResolvedValue({ id: 'e1' });
      render(<CreateEntryForm onCreated={() => {}} onCancel={() => {}} />);
      fireEvent.click(screen.getByRole('button', { name: 'Note' }));
      fireEvent.change(screen.getByPlaceholderText('Notes'), { target: { value: 'hello' } });
      await submitWithTags('private');

      expect(sendMessage).toHaveBeenCalledWith(
        expect.objectContaining({
          payload: expect.objectContaining({ type: 'note', data: { notes: 'hello', tags: ['private'] } }),
        }),
      );
    });

    it('refuses a tag longer than 40 characters with a message instead of dropping it', async () => {
      const onCreated = vi.fn();
      render(<CreateEntryForm onCreated={onCreated} onCancel={() => {}} />);
      await submitWithTags(`work, ${'x'.repeat(41)}`);

      // Shown live under the input and again as the submit error.
      expect(screen.getAllByText(/tags can be at most 40 characters/i).length).toBeGreaterThan(0);
      expect(sendMessage).not.toHaveBeenCalledWith(expect.objectContaining({ type: MessageType.CREATE_ENTRY }));
      expect(onCreated).not.toHaveBeenCalled();
    });

    it('offers vault tags matching the tag being typed and completes on click', () => {
      render(
        <CreateEntryForm onCreated={() => {}} onCancel={() => {}} vaultTags={['client', 'home', 'Work', 'workshop']} />,
      );
      const box = screen.getByRole('textbox', { name: 'Tags' }) as HTMLInputElement;

      fireEvent.change(box, { target: { value: 'Work, wo' } });
      // "Work" is already entered, so only "workshop" is offered.
      expect(screen.queryByRole('button', { name: '#Work' })).toBeNull();
      fireEvent.click(screen.getByRole('button', { name: '#workshop' }));
      expect(box.value).toBe('Work, workshop, ');
    });
  });
});

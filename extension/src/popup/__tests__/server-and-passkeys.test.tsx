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

// The popup's "server" entry type and the passkeys section of the detail view.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act, waitFor, within } from '@testing-library/react';
import type { EntryData, EntryResponse } from '@passbubble/shared-ts';

const sendMessage = vi.hoisted(() => vi.fn());
vi.mock('webextension-polyfill', () => ({
  default: {
    runtime: { sendMessage },
    tabs: { query: vi.fn().mockResolvedValue([]) },
    storage: {
      sync: { get: vi.fn().mockResolvedValue({}) },
      session: { get: vi.fn().mockResolvedValue({}), set: vi.fn(), remove: vi.fn() },
    },
  },
}));
vi.mock('../store/entries.js', () => ({ useEntriesStore: vi.fn() }));

import { CreateEntryForm } from '../components/CreateEntryForm.js';
import { EditEntryForm } from '../components/EditEntryForm.js';
import { EntryDetail, sshCommand } from '../components/EntryDetail.js';
import { EntryList } from '../components/EntryList.js';
import { clearEditDraft } from '../store/edit-draft.js';
import { useEntriesStore } from '../store/entries.js';
import { MessageType } from '../../shared/constants.js';

const SERVER: EntryResponse = { id: 's1', name: 'Build box', type: 'server', owner_id: 'u1', created_at: '', updated_at: '' };
const LOGIN: EntryResponse = { id: 'e1', name: 'Example', url: 'https://example.com', type: 'password', owner_id: 'u1', created_at: '', updated_at: '' };
const KEY = '-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXk=\n-----END OPENSSH PRIVATE KEY-----\n';

function serve(data: EntryData | Record<string, unknown>, entry: EntryResponse) {
  sendMessage.mockImplementation(async (msg: { type: string }) => {
    if (msg.type === MessageType.GET_ENTRY) return { entry, data };
    if (msg.type === MessageType.EDIT_ENTRY) return { ok: true };
    if (msg.type === MessageType.PASSKEY_DELETE) return { ok: true };
    return { id: 'new' };
  });
}

const payloadsOf = (type: string) =>
  sendMessage.mock.calls.map(([m]) => m as { type: string; payload: Record<string, unknown> }).filter((m) => m.type === type).map((m) => m.payload);

beforeEach(() => {
  sendMessage.mockReset();
  clearEditDraft();
});

describe('server entries: create', () => {
  async function fillAndSave(fields: Record<string, string>) {
    serve({}, SERVER);
    render(<CreateEntryForm onCreated={() => {}} onCancel={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: 'Server' }));
    fireEvent.change(screen.getByPlaceholderText('Name'), { target: { value: 'Build box' } });
    for (const [label, value] of Object.entries(fields)) {
      fireEvent.change(screen.getByLabelText(label), { target: { value } });
    }
    await act(async () => fireEvent.submit(screen.getByRole('button', { name: /save entry/i }).closest('form')!));
  }

  it('needs a host', async () => {
    await fillAndSave({ Username: 'deploy' });
    expect(screen.getByText(/host is required/i)).toBeDefined();
    expect(payloadsOf(MessageType.CREATE_ENTRY)).toEqual([]);
  });

  it('stores the fields, the private key exactly as pasted', async () => {
    await fillAndSave({
      Host: ' build.example.org ',
      Port: '2222',
      Username: 'deploy',
      Password: 'pw',
      'Private key': KEY,
      'Key passphrase': 'phrase',
      Notes: 'rack 4',
    });
    expect(payloadsOf(MessageType.CREATE_ENTRY)).toEqual([
      {
        name: 'Build box',
        type: 'server',
        url: '',
        data: {
          host: 'build.example.org',
          port: '2222',
          username: 'deploy',
          password: 'pw',
          private_key: KEY,
          key_passphrase: 'phrase',
          notes: 'rack 4',
        },
      },
    ]);
  });
});

describe('server entries: edit', () => {
  it('shows the server fields without a URL and keeps the key verbatim', async () => {
    serve({ host: 'build.example.org', port: 22, username: 'deploy' }, SERVER);
    render(<EditEntryForm entry={SERVER} onCancel={() => {}} onSaved={() => {}} />);
    await screen.findByLabelText('Name');
    expect(screen.queryByLabelText('URL')).toBeNull();
    expect((screen.getByLabelText('Port') as HTMLInputElement).value).toBe('22');
    expect((screen.getByLabelText('User') as HTMLInputElement).value).toBe('deploy');
    fireEvent.change(screen.getByLabelText('Private key'), { target: { value: KEY } });
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Save changes' })));
    // The port (a number from another client) was not touched, so it is not sent.
    expect(payloadsOf(MessageType.EDIT_ENTRY)).toEqual([{ entryId: 's1', fields: { private_key: KEY } }]);
  });
});

describe('server entries: detail', () => {
  it('shows where to connect and an ssh command, secrets masked', async () => {
    serve({ host: 'build.example.org', port: '2222', username: 'deploy', password: 'pw', private_key: KEY, key_passphrase: 'phrase' }, SERVER);
    render(<EntryDetail entry={SERVER} onBack={() => {}} />);
    await screen.findByText('build.example.org');
    expect(screen.getByText('2222')).toBeDefined();
    expect(screen.getByText('User')).toBeDefined();
    expect(screen.getByText('ssh -p 2222 deploy@build.example.org')).toBeDefined();
    expect(screen.queryByText(/BEGIN OPENSSH/)).toBeNull();
    expect(screen.queryByText('phrase')).toBeNull();
    expect(screen.getByText('Private key')).toBeDefined();
    expect(screen.getByText('Passphrase')).toBeDefined();
  });

  it('builds the ssh command', () => {
    expect(sshCommand({ host: 'h.example' })).toBe('ssh h.example');
    expect(sshCommand({ host: 'h.example', port: '22', username: 'root' })).toBe('ssh root@h.example');
    expect(sshCommand({ host: 'h.example', port: '' })).toBe('ssh h.example');
    expect(sshCommand({ host: 'h.example', port: 2200 as unknown as string })).toBe('ssh -p 2200 h.example');
    expect(sshCommand({ username: 'x' })).toBe('');
    // A shared entry cannot smuggle a second command into the paste.
    expect(sshCommand({ host: 'h.example', username: 'x; rm -rf ~' })).toBe(`ssh 'x; rm -rf ~@h.example'`);
    expect(sshCommand({ host: "it's", port: '1$(id)' })).toBe(`ssh -p '1$(id)' 'it'\\''s'`);
  });
});

describe('server entries: list', () => {
  it('marks servers and offers username and password copies', () => {
    (useEntriesStore as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      entries: [SERVER],
      folders: [],
      usernames: {},
      currentHost: '',
      isLoading: false,
      error: null,
      load: vi.fn().mockResolvedValue(undefined),
      copyField: vi.fn().mockResolvedValue(undefined),
      toggleSite: vi.fn(),
      removeMatch: vi.fn(),
    });
    render(<EntryList />);
    expect(screen.getByText(/🖥/)).toBeDefined();
    expect(screen.getByRole('button', { name: 'Username' })).toBeDefined();
    expect(screen.getByRole('button', { name: 'Password' })).toBeDefined();
  });
});

describe('passkeys in the detail view', () => {
  const passkey = (id: string, rp: string, user: string) => ({
    credential_id: id,
    rp_id: rp,
    user_handle: 'dXNlcg',
    user_name: user,
    key_algorithm: -7,
    private_key: 'PRIVATE-KEY-MATERIAL',
    prf_secret: 'PRF-SECRET',
    sign_count: 0,
    discoverable: true,
    created_at: '2026-11-14T03:00:00Z',
  });

  it('lists site, user and date, never key material', async () => {
    serve({ username: 'alice', password: '', passkeys: [passkey('c1', 'example.com', 'alice@example.com')] }, LOGIN);
    render(<EntryDetail entry={LOGIN} onBack={() => {}} />);
    const section = await screen.findByRole('region', { name: 'Passkeys' });
    expect(within(section).getByText('example.com')).toBeDefined();
    expect(within(section).getByText('alice@example.com')).toBeDefined();
    expect(within(section).getByText(/2026-11-14/)).toBeDefined();
    expect(document.body.textContent).not.toMatch(/PRIVATE-KEY-MATERIAL|PRF-SECRET/);
    // A passkey-only entry has no password to show.
    expect(screen.queryByText('Password')).toBeNull();
  });

  it('deletes a passkey after confirming', async () => {
    serve(
      { username: 'alice', passkeys: [passkey('c1', 'example.com', 'alice'), passkey('c2', 'other.com', 'alice')] },
      LOGIN,
    );
    render(<EntryDetail entry={LOGIN} onBack={() => {}} />);
    await screen.findByRole('region', { name: 'Passkeys' });
    fireEvent.click(screen.getByRole('button', { name: 'Delete passkey for example.com' }));
    expect(payloadsOf(MessageType.PASSKEY_DELETE)).toEqual([]);
    expect(screen.getByText(/delete this passkey\?/i)).toBeDefined();
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Delete' })));
    expect(payloadsOf(MessageType.PASSKEY_DELETE)).toEqual([{ entryId: 'e1', credentialId: 'c1' }]);
    await waitFor(() => expect(screen.getAllByTestId('passkey')).toHaveLength(1));
    expect(screen.queryByText('example.com')).toBeNull();
  });

  it('keeps the passkey and says why when deleting fails', async () => {
    serve({ username: 'alice', passkeys: [passkey('c1', 'example.com', 'alice')] }, LOGIN);
    render(<EntryDetail entry={LOGIN} onBack={() => {}} />);
    await screen.findByRole('region', { name: 'Passkeys' });
    sendMessage.mockResolvedValueOnce({ ok: false, error: 'You only have read access to this entry — nothing was changed.' });
    fireEvent.click(screen.getByRole('button', { name: 'Delete passkey for example.com' }));
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Delete' })));
    expect(screen.getByRole('alert').textContent).toMatch(/read access/);
    expect(screen.getAllByTestId('passkey')).toHaveLength(1);
  });
});

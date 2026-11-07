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
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { generateTotp } from '@passbubble/shared-ts';
import type { EntryResponse } from '@passbubble/shared-ts';

const sendMessage = vi.hoisted(() => vi.fn());
vi.mock('webextension-polyfill', () => ({ default: { runtime: { sendMessage } } }));

import { EntryDetail } from '../components/EntryDetail.js';

const entry = {
  id: 'e1',
  name: 'GitHub',
  url: 'https://github.com',
  type: 'password',
  owner_id: 'u1',
  created_at: '',
  updated_at: '',
} as EntryResponse;

describe('EntryDetail', () => {
  beforeEach(() => sendMessage.mockReset());

  it('loads and shows the decrypted fields with a masked password', async () => {
    sendMessage.mockResolvedValue({ data: { username: 'octocat', password: 's3cret', notes: 'hi' } });
    render(<EntryDetail entry={entry} onBack={() => {}} />);

    await waitFor(() => expect(screen.getByText('octocat')).toBeDefined());
    expect(screen.getByText('••••••••')).toBeDefined();
    expect(screen.queryByText('s3cret')).toBeNull();
  });

  it('reveals the password on Show', async () => {
    sendMessage.mockResolvedValue({ data: { username: 'octocat', password: 's3cret' } });
    render(<EntryDetail entry={entry} onBack={() => {}} />);

    await waitFor(() => expect(screen.getByText('octocat')).toBeDefined());
    fireEvent.click(screen.getByRole('button', { name: /show/i }));
    expect(screen.getByText('s3cret')).toBeDefined();
  });

  it('masks secret custom fields and reveals each field on its own', async () => {
    sendMessage.mockResolvedValue({
      data: {
        username: 'octocat',
        password: 's3cret',
        custom_fields: [
          { label: 'Recovery', value: 'ask the cat', type: 'text' },
          { label: 'PIN', value: '4711', type: 'password' },
          { label: 'Backup', value: 'backup-code', type: 'password' },
          { label: 'Deploy key', value: 'ssh-ed25519 AAAA', type: 'ssh' },
        ],
      },
    });
    render(<EntryDetail entry={entry} onBack={() => {}} />);

    await waitFor(() => expect(screen.getByText('octocat')).toBeDefined());
    expect(screen.getByText('ask the cat')).toBeDefined();
    for (const secret of ['s3cret', '4711', 'backup-code', 'ssh-ed25519 AAAA']) {
      expect(screen.queryByText(secret)).toBeNull();
    }

    // Show buttons in field order: password, PIN, Backup, Deploy key.
    fireEvent.click(screen.getAllByRole('button', { name: 'Show' })[1]);
    expect(screen.getByText('4711')).toBeDefined();
    expect(screen.queryByText('s3cret')).toBeNull();
    expect(screen.queryByText('backup-code')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Hide' }));
    expect(screen.queryByText('4711')).toBeNull();
  });

  it('masks a custom TOTP field until revealed and still copies its value', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    sendMessage.mockResolvedValue({
      data: { custom_fields: [{ label: 'Backup 2FA', value: 'JBSWY3DPEHPK3PXP', type: 'totp' }] },
    });
    render(<EntryDetail entry={entry} onBack={() => {}} />);

    await screen.findByText('Backup 2FA');
    expect(screen.queryByText('JBSWY3DPEHPK3PXP')).toBeNull();

    // Copying does not need the field to be revealed.
    fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[0]);
    expect(writeText).toHaveBeenCalledWith('JBSWY3DPEHPK3PXP');
    expect(screen.queryByText('JBSWY3DPEHPK3PXP')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Show' }));
    expect(screen.getByText('JBSWY3DPEHPK3PXP')).toBeDefined();
  });

  it('masks the private key of an ssh-key entry with a reveal toggle', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    const key = '-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----\n';
    sendMessage.mockResolvedValue({ data: { username: 'deploy', password: key } });
    render(<EntryDetail entry={{ ...entry, type: 'ssh-key' }} onBack={() => {}} />);

    await screen.findByText('Private key');
    expect(screen.queryByText('Password')).toBeNull();
    expect(screen.getByText('••••••••')).toBeDefined();
    expect(document.body.textContent).not.toContain('BEGIN OPENSSH');

    fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[1]);
    expect(writeText).toHaveBeenCalledWith(key);
    expect(document.body.textContent).not.toContain('BEGIN OPENSSH');

    fireEvent.click(screen.getByRole('button', { name: 'Show' }));
    expect(document.body.textContent).toContain('BEGIN OPENSSH');
    fireEvent.click(screen.getByRole('button', { name: 'Hide' }));
    expect(document.body.textContent).not.toContain('BEGIN OPENSSH');
  });

  describe('TOTP codes', () => {
    const SECRET = 'JBSWY3DPEHPK3PXP';
    // An hour-long period, so the expected code stays valid during the test.
    const STORED = { period: 3600, digits: 8, algorithm: 'SHA256' };

    it("uses the entry's stored period, digits and algorithm", async () => {
      sendMessage.mockResolvedValue({ data: { username: 'octocat', totp_secret: SECRET, ...STORED } });
      render(<EntryDetail entry={entry} onBack={() => {}} />);

      const { code } = await generateTotp(SECRET, STORED);
      expect(code).toHaveLength(8);
      expect(await screen.findByText(code)).toBeDefined();
      expect(await screen.findByText(/^TOTP \(\d+s\)$/)).toBeDefined();
    });

    it('shows a code for a custom totp field, with the entry parameters', async () => {
      sendMessage.mockResolvedValue({
        data: { ...STORED, custom_fields: [{ label: 'Backup', value: SECRET, type: 'totp' }] },
      });
      render(<EntryDetail entry={entry} onBack={() => {}} />);

      const { code } = await generateTotp(SECRET, STORED);
      expect(await screen.findByText(code)).toBeDefined();
      expect(screen.getByText(/^Backup code \(\d+s\)$/)).toBeDefined();
      // The secret itself stays masked.
      expect(screen.queryByText(SECRET)).toBeNull();
    });

    it('takes the parameters of an otpauth URI in a custom totp field', async () => {
      const uri = `otpauth://totp/x?secret=${SECRET}&period=3600&digits=7`;
      sendMessage.mockResolvedValue({ data: { custom_fields: [{ label: 'Backup', value: uri, type: 'totp' }] } });
      render(<EntryDetail entry={entry} onBack={() => {}} />);

      const { code } = await generateTotp(SECRET, { period: 3600, digits: 7 });
      expect(await screen.findByText(code)).toBeDefined();
    });

    it('shows no code for a custom field it cannot read', async () => {
      sendMessage.mockResolvedValue({
        data: { username: 'octocat', custom_fields: [{ label: 'Odd', value: 'not base32 !!', type: 'totp' }] },
      });
      render(<EntryDetail entry={entry} onBack={() => {}} />);

      await screen.findByText('Odd');
      await new Promise((r) => setTimeout(r, 30));
      expect(screen.queryByText(/code \(\d+s\)/)).toBeNull();
    });

    it('shows no code for a copy of the password in totp_secret', async () => {
      // Valid base32, so it would produce a bogus code.
      sendMessage.mockResolvedValue({ data: { username: 'octocat', password: SECRET, totp_secret: SECRET } });
      render(<EntryDetail entry={entry} onBack={() => {}} />);

      await screen.findByText('octocat');
      await new Promise((r) => setTimeout(r, 30));
      expect(screen.queryByText(/^TOTP/)).toBeNull();
    });

    it('still shows the code of a real secret next to a password', async () => {
      sendMessage.mockResolvedValue({ data: { password: 'hunter2', totp_secret: SECRET } });
      render(<EntryDetail entry={entry} onBack={() => {}} />);

      expect(await screen.findByText(/^TOTP \(\d+s\)$/)).toBeDefined();
    });
  });

  it('offers Edit once the entry has loaded', async () => {
    const onEdit = vi.fn();
    sendMessage.mockResolvedValue({ data: { username: 'octocat' } });
    render(<EntryDetail entry={entry} onBack={() => {}} onEdit={onEdit} />);

    expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull();
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
    expect(onEdit).toHaveBeenCalledTimes(1);
  });

  it("shows the entry's tags, normalised", async () => {
    sendMessage.mockResolvedValue({ data: { username: 'octocat', tags: ['#Work ', 'work', 'client', 42] } });
    render(<EntryDetail entry={entry} onBack={() => {}} />);

    const list = await screen.findByRole('list', { name: 'Tags' });
    expect(Array.from(list.querySelectorAll('li')).map((li) => li.textContent)).toEqual(['#Work', '#client']);
  });

  it('shows no tag list for an entry without tags', async () => {
    sendMessage.mockResolvedValue({ data: { username: 'octocat' } });
    render(<EntryDetail entry={entry} onBack={() => {}} />);
    await waitFor(() => expect(screen.getByText('octocat')).toBeDefined());
    expect(screen.queryByRole('list', { name: 'Tags' })).toBeNull();
  });

  it('shows a lock message when the vault is locked', async () => {
    sendMessage.mockResolvedValue({ locked: true });
    render(<EntryDetail entry={entry} onBack={() => {}} />);
    await waitFor(() => expect(screen.getByText(/vault is locked/i)).toBeDefined());
  });

  it('shows Copied! feedback immediately after clicking copy', async () => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } });
    sendMessage.mockResolvedValue({ data: { username: 'octocat', password: 's3cret' } });
    render(<EntryDetail entry={entry} onBack={() => {}} />);

    await waitFor(() => expect(screen.getByText('octocat')).toBeDefined());
    const copyBtns = screen.getAllByRole('button', { name: /copy/i });
    fireEvent.click(copyBtns[0]);

    expect(screen.getByText('Copied!')).toBeDefined();
  });
});

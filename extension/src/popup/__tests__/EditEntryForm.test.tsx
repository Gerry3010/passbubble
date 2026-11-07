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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act, waitFor, cleanup, within } from '@testing-library/react';
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

import { EditEntryForm } from '../components/EditEntryForm.js';
import { VaultPanel } from '../components/VaultPanel.js';
import { clearEditDraft } from '../store/edit-draft.js';
import { useEntriesStore } from '../store/entries.js';
import { useSessionStore } from '../store/session.js';
import { MessageType } from '../../shared/constants.js';
import type { EntryEdit } from '../../shared/entry-edit.js';

const META: EntryResponse = {
  id: 'e1',
  name: 'GitHub',
  url: 'https://github.com',
  type: 'password',
  owner_id: 'u1',
  created_at: '',
  updated_at: '',
};

type EditPayload = EntryEdit & { entryId: string };

// Serves GET_ENTRY with `data` (under `entry`'s metadata) and accepts EDIT_ENTRY.
function serve(data: EntryData | Record<string, unknown>, entry: EntryResponse = META) {
  sendMessage.mockImplementation(async (msg: { type: string }) => {
    if (msg.type === MessageType.GET_ENTRY) return { entry, data };
    if (msg.type === MessageType.EDIT_ENTRY) return { ok: true };
    if (msg.type === MessageType.GET_SESSION) {
      return { isLoggedIn: true, isUnlocked: useSessionStore.getState().isUnlocked, serverUrl: 'https://srv' };
    }
    return {};
  });
}

function editPayloads(): EditPayload[] {
  return sendMessage.mock.calls
    .map(([msg]) => msg as { type: string; payload: EditPayload })
    .filter((msg) => msg.type === MessageType.EDIT_ENTRY)
    .map((msg) => msg.payload);
}

async function renderForm(
  data: EntryData | Record<string, unknown>,
  { entry = META, vaultTags }: { entry?: EntryResponse; vaultTags?: string[] } = {},
) {
  serve(data, entry);
  const onCancel = vi.fn();
  const onSaved = vi.fn();
  render(<EditEntryForm entry={entry} vaultTags={vaultTags} onCancel={onCancel} onSaved={onSaved} />);
  await screen.findByLabelText('Name');
  return { onCancel, onSaved };
}

const value = (label: string) => (screen.getByLabelText(label) as HTMLInputElement | HTMLTextAreaElement).value;
const change = (label: string, next: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value: next } });

async function save() {
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
  });
}

describe('EditEntryForm', () => {
  beforeEach(() => {
    sendMessage.mockReset();
    clearEditDraft();
    useSessionStore.setState({ isLoggedIn: true, isUnlocked: true, userEmail: 'alice@example.com', error: null });
  });

  afterEach(() => cleanup());

  describe('loading', () => {
    it('fills the form from the entry, with secrets masked', async () => {
      await renderForm({ username: 'octocat', password: 's3cret', notes: 'hello', tags: ['work', 'client'] });

      expect(value('Name')).toBe('GitHub');
      expect(value('URL')).toBe('https://github.com');
      expect(value('Username')).toBe('octocat');
      expect(value('Password')).toBe('s3cret');
      expect((screen.getByLabelText('Password') as HTMLInputElement).type).toBe('password');
      expect(value('Notes')).toBe('hello');
      expect(value('Tags')).toBe('work, client, ');
    });

    it('reveals and hides a secret with its own toggle', async () => {
      await renderForm({ password: 's3cret', totp_secret: 'JBSWY3DP' });
      const password = () => screen.getByLabelText('Password') as HTMLInputElement;

      fireEvent.click(screen.getByRole('button', { name: 'Show Password' }));
      expect(password().type).toBe('text');
      expect((screen.getByLabelText(/^TOTP secret/) as HTMLInputElement).type).toBe('password');

      fireEvent.click(screen.getByRole('button', { name: 'Hide Password' }));
      expect(password().type).toBe('password');
    });

    it('shows the failure when the entry cannot be loaded', async () => {
      sendMessage.mockRejectedValue(new Error('boom'));
      render(<EditEntryForm entry={META} onCancel={() => {}} onSaved={() => {}} />);
      expect(await screen.findByText(/failed to load entry: boom/i)).toBeDefined();
    });
  });

  describe('entry types', () => {
    // Every type with every key the app's edit screen offers for it.
    const TYPES: Array<[type: string, Record<string, string>, hasUrl: boolean]> = [
      ['password', { Username: 'username', Password: 'password', 'TOTP secret (base32 or otpauth:// URI)': 'totp_secret', Notes: 'notes' }, true],
      ['totp', { 'Account / Username': 'username', 'TOTP secret (base32 or otpauth:// URI)': 'totp_secret', Notes: 'notes' }, true],
      ['note', { Content: 'notes' }, true],
      ['api-key', { 'Key ID / Client ID': 'username', 'API key / Secret': 'password', Notes: 'notes' }, true],
      ['ssh-key', { User: 'username', 'Notes / Passphrase': 'notes' }, true],
      [
        'credit-card',
        {
          'Card number': 'card_number',
          'Cardholder name': 'holder_name',
          'Expiry month (MM)': 'expiry_month',
          'Expiry year (YYYY)': 'expiry_year',
          CVV: 'cvv',
          Notes: 'notes',
        },
        true,
      ],
      [
        'bank-account',
        { 'Bank name': 'bank_name', IBAN: 'iban', 'BIC / SWIFT': 'bic', 'Account number': 'account_number', Notes: 'notes' },
        false,
      ],
      [
        'identity',
        {
          'First name': 'first_name',
          'Last name': 'last_name',
          Company: 'company',
          Email: 'email',
          Phone: 'phone',
          Street: 'street',
          City: 'city',
          State: 'state',
          'Postal code': 'postal_code',
          Country: 'country',
          Notes: 'notes',
        },
        false,
      ],
      [
        'license',
        {
          'Product name': 'product_name',
          'License key': 'license_key',
          'Purchase email': 'purchase_email',
          'Purchase date (YYYY-MM-DD)': 'purchase_date',
          'Expires at (YYYY-MM-DD)': 'expires_at',
          Notes: 'notes',
        },
        true,
      ],
    ];

    it.each(TYPES)('%s: loads and saves each field under its JSON key', async (type, fields, hasUrl) => {
      const stored = Object.fromEntries(Object.values(fields).map((key) => [key, `old-${key}`]));
      await renderForm(stored, { entry: { ...META, type } });

      expect(screen.queryByLabelText('URL') !== null).toBe(hasUrl);
      const expected: Record<string, string> = {};
      for (const [label, key] of Object.entries(fields)) {
        expect(value(label)).toBe(`old-${key}`);
        // A valid base32 value, since this one is validated.
        const next = key === 'totp_secret' ? 'MFRGGZDF' : `new-${key}`;
        change(label, next);
        expected[key] = next;
      }
      await save();

      expect(editPayloads()[0].fields).toEqual(expected);
    });

    it('edits the drop-downs with the values the app uses', async () => {
      await renderForm({ account_type: 'checking' }, { entry: { ...META, type: 'bank-account' } });
      change('Account type', 'savings');
      await save();
      expect(editPayloads()[0].fields).toEqual({ account_type: 'savings' });
      cleanup();

      sendMessage.mockClear();
      await renderForm({ title: 'Mr' }, { entry: { ...META, type: 'identity' } });
      change('Title', 'Dr');
      await save();
      expect(editPayloads()[0].fields).toEqual({ title: 'Dr' });
      cleanup();

      sendMessage.mockClear();
      await renderForm({ sign_in_with: 'google' });
      change('Sign in with (SSO)', 'github');
      await save();
      expect(editPayloads()[0].fields).toEqual({ sign_in_with: 'github' });
    });

    it('keeps a stored drop-down value the list does not have', async () => {
      const { onSaved } = await renderForm({ title: 'Prof' }, { entry: { ...META, type: 'identity' } });
      expect(value('Title')).toBe('Prof');
      await save();
      expect(editPayloads()).toEqual([]);
      expect(onSaved).toHaveBeenCalled();
    });

    it('edits an unknown type as a login, like the app', async () => {
      await renderForm({ username: 'x' }, { entry: { ...META, type: 'from-the-future' } });
      expect(value('Username')).toBe('x');
    });
  });

  describe('saving', () => {
    it('sends only what changed', async () => {
      const { onSaved } = await renderForm({ username: 'octocat', password: 's3cret', notes: 'hello', tags: ['work'] });
      change('Password', 'new-pass');
      await save();

      expect(editPayloads()).toEqual([{ entryId: 'e1', fields: { password: 'new-pass' } }]);
      expect(onSaved).toHaveBeenCalledWith({ name: 'GitHub', url: 'https://github.com' });
    });

    it('writes nothing when nothing changed', async () => {
      const { onSaved } = await renderForm({ username: 'octocat', custom_fields: [{ label: 'a', value: 'b' }], tags: ['x'] });
      await save();
      expect(editPayloads()).toEqual([]);
      expect(onSaved).toHaveBeenCalledTimes(1);
    });

    it('sends a blank for a value the user removed', async () => {
      await renderForm({ username: 'octocat', notes: 'hello' });
      change('Notes', '   ');
      await save();
      expect(editPayloads()[0].fields).toEqual({ notes: '' });
    });

    it('trims like the app, but keeps a password as typed', async () => {
      await renderForm({ username: 'octocat', password: 's3cret' });
      change('Username', '  neo  ');
      change('Password', ' spaced ');
      await save();
      expect(editPayloads()[0].fields).toEqual({ username: 'neo', password: ' spaced ' });
    });

    it('requires a name and sends nothing without one', async () => {
      const { onSaved } = await renderForm({ username: 'octocat' });
      change('Name', '  ');
      await save();
      expect(screen.getByRole('alert').textContent).toMatch(/name is required/i);
      expect(editPayloads()).toEqual([]);
      expect(onSaved).not.toHaveBeenCalled();
    });

    it('sends an empty URL when the user empties it', async () => {
      const { onSaved } = await renderForm({ username: 'octocat' });
      change('URL', '   ');
      await save();
      expect(editPayloads()).toEqual([{ entryId: 'e1', url: '', fields: {} }]);
      expect(onSaved).toHaveBeenCalledWith({ name: 'GitHub', url: '' });
    });

    it('sends a changed name and URL', async () => {
      const { onSaved } = await renderForm({ username: 'octocat' });
      change('Name', ' Renamed ');
      change('URL', ' https://new.example ');
      await save();
      expect(editPayloads()).toEqual([{ entryId: 'e1', name: 'Renamed', url: 'https://new.example', fields: {} }]);
      expect(onSaved).toHaveBeenCalledWith({ name: 'Renamed', url: 'https://new.example' });
    });

    it('never sends a URL for a type without the field', async () => {
      await renderForm({ first_name: 'A' }, { entry: { ...META, type: 'identity', url: 'https://kept.example' } });
      change('First name', 'B');
      await save();
      expect(editPayloads()[0]).not.toHaveProperty('url');
    });

    it('shows a save error from the background and keeps the input', async () => {
      const { onSaved } = await renderForm({ password: 's3cret' });
      sendMessage.mockImplementation(async () => ({ ok: false, error: 'You only have read access to this entry' }));
      change('Password', 'new-pass');
      await save();

      expect(screen.getByRole('alert').textContent).toMatch(/read access/);
      expect(value('Password')).toBe('new-pass');
      expect(onSaved).not.toHaveBeenCalled();
    });

    it('Cancel changes nothing', async () => {
      const { onCancel, onSaved } = await renderForm({ password: 's3cret' });
      change('Password', 'new-pass');
      fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

      expect(onCancel).toHaveBeenCalledTimes(1);
      expect(onSaved).not.toHaveBeenCalled();
      expect(editPayloads()).toEqual([]);
    });
  });

  describe('2FA secret', () => {
    it('refuses a changed secret that is not base32 or an otpauth URI', async () => {
      await renderForm({ totp_secret: 'JBSWY3DP' });
      change('TOTP secret (base32 or otpauth:// URI)', 'not base32 !!');
      await save();
      expect(screen.getByRole('alert').textContent).toMatch(/not a valid base32/);
      expect(editPayloads()).toEqual([]);
    });

    it('leaves an unchanged secret alone, whatever it looks like', async () => {
      const { onSaved } = await renderForm({ totp_secret: 'not base32 !!' });
      await save();
      expect(editPayloads()).toEqual([]);
      expect(onSaved).toHaveBeenCalled();
    });

    it('shows a copy of the password as no secret, and does not send it back', async () => {
      await renderForm({ password: 'JBSWY3DP', totp_secret: 'JBSWY3DP', notes: 'n' });
      expect(value('TOTP secret (base32 or otpauth:// URI)')).toBe('');
      change('Notes', 'changed');
      await save();
      expect(editPayloads()[0].fields).toEqual({ notes: 'changed' });
    });

    it('writes nothing for a stray secret when nothing else changed', async () => {
      const { onSaved } = await renderForm({ password: 'JBSWY3DP', totp_secret: 'JBSWY3DP' });
      await save();
      expect(editPayloads()).toEqual([]);
      expect(onSaved).toHaveBeenCalled();
    });

    it('shows a real secret and does not send it when untouched', async () => {
      await renderForm({ password: 'hunter2', totp_secret: 'JBSWY3DP' });
      expect(value('TOTP secret (base32 or otpauth:// URI)')).toBe('JBSWY3DP');
      change('Password', 'new-pass');
      await save();
      expect(editPayloads()[0].fields).toEqual({ password: 'new-pass' });
    });

    it('takes secret and parameters from a pasted otpauth URI', async () => {
      await renderForm({ totp_secret: 'JBSWY3DP', period: 60 });
      change('TOTP secret (base32 or otpauth:// URI)', 'otpauth://totp/ACME:me?secret=MFRGGZDF&digits=8');
      await save();
      expect(editPayloads()[0].fields).toEqual({
        totp_secret: 'MFRGGZDF',
        issuer: 'ACME',
        digits: 8,
        period: null,
        algorithm: null,
      });
    });
  });

  describe('multi-line secrets', () => {
    const KEY = '-----BEGIN KEY-----\nabc\ndef\n-----END KEY-----\n';
    const SSH_ENTRY = { ...META, type: 'ssh-key' };

    it('hides the private key entirely until revealed, then edits all of it', async () => {
      await renderForm({ username: 'deploy', password: KEY }, { entry: SSH_ENTRY });

      const masked = screen.getByRole('textbox', { name: 'Private key' });
      expect(masked.getAttribute('aria-readonly')).toBe('true');
      expect(document.body.textContent).not.toContain('BEGIN KEY');

      fireEvent.click(screen.getByRole('button', { name: 'Show Private key' }));
      expect(value('Private key')).toBe(KEY);
    });

    it('keeps the line breaks and the final newline of a changed key', async () => {
      await renderForm({ password: KEY }, { entry: SSH_ENTRY });
      fireEvent.click(screen.getByRole('button', { name: 'Show Private key' }));
      const edited = KEY.replace('abc', 'xyz');
      change('Private key', edited);
      await save();
      expect(editPayloads()[0].fields).toEqual({ password: edited });
    });

    it('keeps showing a secret typed into an empty field', async () => {
      await renderForm({}, { entry: SSH_ENTRY });
      change('Private key', 'line1\nline2');
      expect(value('Private key')).toBe('line1\nline2');
    });
  });

  describe('generator', () => {
    const shownPassword = () => screen.getByRole('status', { name: 'Generated password' }).textContent ?? '';

    async function openGeneratorFor(button: string) {
      fireEvent.click(screen.getByRole('button', { name: button }));
      await screen.findByRole('status', { name: 'Generated password' });
    }

    it('OK writes the password into the field', async () => {
      await renderForm({ password: 'typed-before' });
      await openGeneratorFor('Generate Password');
      const chosen = shownPassword();
      fireEvent.click(screen.getByRole('button', { name: 'OK' }));

      expect(screen.queryByRole('dialog')).toBeNull();
      expect(value('Password')).toBe(chosen);
      await save();
      expect(editPayloads()[0].fields).toEqual({ password: chosen });
    });

    it('Cancel leaves the field alone', async () => {
      await renderForm({ password: 'typed-before' });
      await openGeneratorFor('Generate Password');
      fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));

      expect(screen.queryByRole('dialog')).toBeNull();
      expect(value('Password')).toBe('typed-before');
    });

    it('is offered on password fields only, like the app', async () => {
      await renderForm({ password: 'k', notes: 'n' }, { entry: { ...META, type: 'api-key' } });
      expect(screen.queryByRole('button', { name: /^Generate/ })).toBeNull();
    });

    it('writes into a custom password field; Cancel leaves it', async () => {
      await renderForm({ custom_fields: [{ label: 'PIN', value: 'old', type: 'password' }] });

      await openGeneratorFor('Generate Value of custom field 1');
      fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
      expect(value('Value of custom field 1')).toBe('old');

      await openGeneratorFor('Generate Value of custom field 1');
      const chosen = shownPassword();
      fireEvent.click(screen.getByRole('button', { name: 'OK' }));
      expect(value('Value of custom field 1')).toBe(chosen);
      await save();
      expect(editPayloads()[0].customFields).toEqual([{ label: 'PIN', value: chosen, type: 'password' }]);
    });

    it('is not offered on other custom field types', async () => {
      await renderForm({ custom_fields: [{ label: 'a', value: 'b', type: 'ssh' }] });
      expect(screen.queryByRole('button', { name: 'Generate Value of custom field 1' })).toBeNull();
    });
  });

  describe('tags', () => {
    it('sends the normalised list when it changed', async () => {
      await renderForm({ tags: ['work'] });
      change('Tags', 'work, #Home  , ');
      await save();
      expect(editPayloads()[0].tags).toEqual(['work', 'Home']);
      expect(editPayloads()[0]).not.toHaveProperty('customFields');
    });

    it('sends an empty list when all tags were removed', async () => {
      await renderForm({ tags: ['work'] });
      change('Tags', '');
      await save();
      expect(editPayloads()[0].tags).toEqual([]);
    });

    it('offers vault tags as completions', async () => {
      await renderForm({ tags: ['work'] }, { vaultTags: ['client', 'work', 'workshop'] });
      change('Tags', 'work, wor');
      fireEvent.click(screen.getByRole('button', { name: '#workshop' }));
      expect(value('Tags')).toBe('work, workshop, ');
      expect(screen.queryByRole('button', { name: '#work' })).toBeNull();
    });

    it('refuses a tag that is too long', async () => {
      await renderForm({});
      change('Tags', 'x'.repeat(41));
      await save();
      expect(screen.getByRole('alert').textContent).toMatch(/at most 40/);
      expect(editPayloads()).toEqual([]);
    });
  });

  describe('custom fields', () => {
    const file = { label: 'Contract', value: 'Y29udHJhY3Q=', type: 'file', filename: 'contract.pdf', mime_type: 'application/pdf' };
    const a = { label: 'A', value: 'va', type: 'text' };
    const b = { label: 'B', value: 'vb', type: 'password' };
    const c = { label: 'C', value: 'vc', type: 'text', written_by_a_newer_app: { nested: true } };

    it('masks secret types with a toggle and leaves text visible', async () => {
      await renderForm({
        custom_fields: [a, b, { label: 'D', value: 'vd', type: 'totp' }, { label: 'E', value: 'l1\nl2', type: 'ssh' }],
      });
      const type = (n: number) => (screen.getByLabelText(`Value of custom field ${n}`) as HTMLInputElement).type;
      expect(type(1)).toBe('text');
      expect(type(2)).toBe('password');
      expect(type(3)).toBe('password');
      expect(screen.getByRole('textbox', { name: 'Value of custom field 4' }).getAttribute('aria-readonly')).toBe('true');

      fireEvent.click(screen.getByRole('button', { name: 'Show Value of custom field 2' }));
      expect(type(2)).toBe('text');
    });

    it('edits label, value and type, keeping keys it does not know', async () => {
      await renderForm({ custom_fields: [c] });
      change('Label of custom field 1', 'Renamed');
      change('Value of custom field 1', 'changed');
      fireEvent.change(screen.getByRole('combobox', { name: 'Type of custom field 1' }), { target: { value: 'totp' } });
      await save();

      expect(editPayloads()[0].customFields).toEqual([
        { label: 'Renamed', value: 'changed', type: 'totp', written_by_a_newer_app: { nested: true } },
      ]);
    });

    it('offers text, password, ssh and totp, and keeps another stored type', async () => {
      await renderForm({ custom_fields: [{ label: 'Site', value: 'https://x', type: 'url' }, a] });
      const options = (n: number) =>
        within(screen.getByRole('combobox', { name: `Type of custom field ${n}` }))
          .getAllByRole('option')
          .map((o) => (o as HTMLOptionElement).value);
      expect(options(2)).toEqual(['text', 'password', 'ssh', 'totp']);
      expect(options(1)).toEqual(['text', 'password', 'ssh', 'totp', 'url']);

      change('Value of custom field 1', 'https://y');
      await save();
      expect(editPayloads()[0].customFields?.[0]).toEqual({ label: 'Site', value: 'https://y', type: 'url' });
    });

    it('adds a field at the end, and removes one', async () => {
      await renderForm({ custom_fields: [a, b] });
      fireEvent.click(screen.getByRole('button', { name: 'Remove custom field 1' }));
      fireEvent.click(screen.getByRole('button', { name: '+ Add field' }));
      change('Label of custom field 2', 'New');
      change('Value of custom field 2', 'nv');
      await save();

      expect(editPayloads()[0].customFields).toEqual([b, { label: 'New', value: 'nv', type: 'text' }]);
    });

    it('sends an empty list when the last field was removed', async () => {
      await renderForm({ custom_fields: [a] });
      fireEvent.click(screen.getByRole('button', { name: 'Remove custom field 1' }));
      await save();
      expect(editPayloads()[0].customFields).toEqual([]);
    });

    it('drops an added row left empty, without writing anything', async () => {
      const { onSaved } = await renderForm({ custom_fields: [a] });
      fireEvent.click(screen.getByRole('button', { name: '+ Add field' }));
      await save();
      expect(editPayloads()).toEqual([]);
      expect(onSaved).toHaveBeenCalled();
    });

    it('refuses to drop a value that has no label', async () => {
      await renderForm({ custom_fields: [a] });
      fireEvent.click(screen.getByRole('button', { name: '+ Add field' }));
      change('Value of custom field 2', 'orphan');
      await save();
      expect(screen.getByRole('alert').textContent).toMatch(/needs a label/);
      expect(editPayloads()).toEqual([]);
      expect(value('Value of custom field 2')).toBe('orphan');
    });

    it('reorders with the up and down buttons', async () => {
      await renderForm({ custom_fields: [a, b, c] });
      expect((screen.getByRole('button', { name: 'Move custom field 1 up' }) as HTMLButtonElement).disabled).toBe(true);
      expect((screen.getByRole('button', { name: 'Move custom field 3 down' }) as HTMLButtonElement).disabled).toBe(true);

      fireEvent.click(screen.getByRole('button', { name: 'Move custom field 3 up' }));
      fireEvent.click(screen.getByRole('button', { name: 'Move custom field 1 down' }));
      await save();

      expect(editPayloads()[0].customFields).toEqual([c, a, b]);
    });

    it('sends a file field back untouched, at its position, with no controls', async () => {
      await renderForm({ custom_fields: [a, file, b] });

      const rows = screen.getAllByTestId('custom-field');
      expect(rows[1].textContent).toContain('Contract');
      expect(rows[1].textContent).toContain('contract.pdf');
      expect(rows[1].textContent).toContain('read-only');
      expect(rows[1].textContent).not.toContain('Y29udHJhY3Q=');
      expect(within(rows[1]).queryAllByRole('button')).toEqual([]);
      expect(within(rows[1]).queryAllByRole('textbox')).toEqual([]);

      // Moving a field past the file field jumps over it.
      fireEvent.click(screen.getByRole('button', { name: 'Move custom field 1 down' }));
      await save();
      expect(editPayloads()[0].customFields).toEqual([b, file, a]);
    });

    it('keeps a file field untouched when another field is edited', async () => {
      await renderForm({ custom_fields: [file, a] });
      change('Value of custom field 2', 'changed');
      await save();
      expect(editPayloads()[0].customFields).toEqual([file, { ...a, value: 'changed' }]);
    });

    it('keeps a list item it cannot read', async () => {
      await renderForm({ custom_fields: ['odd', a] });
      change('Value of custom field 2', 'changed');
      await save();
      expect(editPayloads()[0].customFields).toEqual(['odd', { ...a, value: 'changed' }]);
    });
  });

  describe('a locked vault during save', () => {
    function lockedOnSave() {
      sendMessage.mockImplementation(async (msg: { type: string }) => {
        if (msg.type === MessageType.EDIT_ENTRY) return { locked: true };
        if (msg.type === MessageType.GET_SESSION) return { isLoggedIn: true, isUnlocked: false, serverUrl: 'https://srv' };
        return {};
      });
    }

    it('says nothing was saved and switches to the unlock screen', async () => {
      const { onSaved } = await renderForm({ password: 's3cret' });
      lockedOnSave();
      change('Password', 'typed-new');
      await save();

      expect(screen.getByRole('alert').textContent).toMatch(/locked — nothing was saved/);
      expect(onSaved).not.toHaveBeenCalled();
      await waitFor(() => expect(useSessionStore.getState().isUnlocked).toBe(false));
    });

    it('brings the input back after unlocking, and saves it from there', async () => {
      await renderForm({ username: 'octocat', password: 's3cret', custom_fields: [{ label: 'a', value: 'b' }] });
      lockedOnSave();
      change('Password', 'typed-new');
      change('Value of custom field 1', 'edited');
      await save();
      cleanup(); // App replaces the panel with the unlock screen.

      useSessionStore.setState({ isUnlocked: true });
      serve({ username: 'octocat', password: 's3cret' });
      const onSaved = vi.fn();
      render(<EditEntryForm entry={META} onCancel={() => {}} onSaved={onSaved} />);

      expect(screen.getByText(/unsaved changes were restored/)).toBeDefined();
      expect(value('Password')).toBe('typed-new');
      expect(value('Value of custom field 1')).toBe('edited');

      await save();
      expect(onSaved).toHaveBeenCalledTimes(1);
      const [last] = editPayloads().slice(-1);
      expect(last.fields).toEqual({ password: 'typed-new' });
      expect(last.customFields).toEqual([{ label: 'a', value: 'edited', type: 'text' }]);

      // Saved, so the next open starts from the entry again.
      cleanup();
      serve({ username: 'octocat', password: 'typed-new' });
      render(<EditEntryForm entry={META} onCancel={() => {}} onSaved={() => {}} />);
      await screen.findByLabelText('Name');
      expect(value('Password')).toBe('typed-new');
      expect(screen.queryByText(/unsaved changes were restored/)).toBeNull();
    });

    it('keeps no draft after Cancel', async () => {
      await renderForm({ password: 's3cret' });
      change('Password', 'typed-new');
      fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
      cleanup();

      await renderForm({ password: 's3cret' });
      expect(value('Password')).toBe('s3cret');
    });

    it('does not offer one account the draft of another', async () => {
      await renderForm({ password: 's3cret' });
      change('Password', 'typed-new');
      cleanup();

      useSessionStore.setState({ userEmail: 'bob@example.com' });
      await renderForm({ password: 'bobs' });
      expect(value('Password')).toBe('bobs');
    });
  });

  describe('in the vault panel', () => {
    const ENTRY = { ...META, name: 'GitHub' };

    function servePanel(data: EntryData) {
      useEntriesStore.setState({
        entries: [ENTRY],
        folders: [],
        usernames: {},
        entryTags: {},
        currentHost: '',
        isLoading: false,
        error: null,
        load: vi.fn(async () => {}),
      });
      serve(data, ENTRY);
    }

    async function openEditForm() {
      fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
      await screen.findByLabelText('Name');
    }

    it('goes from the detail view to the form and back with the new name', async () => {
      servePanel({ username: 'octocat', password: 's3cret' });
      render(<VaultPanel />);
      fireEvent.click(await screen.findByText('GitHub'));
      await openEditForm();

      change('Name', 'GitHub work');
      await save();

      expect(await screen.findByRole('heading', { name: 'GitHub work' })).toBeDefined();
      expect(useEntriesStore.getState().load).toHaveBeenCalled();
      expect(editPayloads()[0].name).toBe('GitHub work');
    });

    it('Cancel returns to the detail view', async () => {
      servePanel({ username: 'octocat' });
      render(<VaultPanel />);
      fireEvent.click(await screen.findByText('GitHub'));
      await openEditForm();

      change('Name', 'never saved');
      fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

      expect(await screen.findByRole('heading', { name: 'GitHub' })).toBeDefined();
      expect(editPayloads()).toEqual([]);
    });

    it('reopens the form with the input after the panel was replaced', async () => {
      servePanel({ username: 'octocat', password: 's3cret' });
      render(<VaultPanel />);
      fireEvent.click(await screen.findByText('GitHub'));
      await openEditForm();
      change('Password', 'typed-new');
      cleanup(); // Vault locked or tab switched: the panel is gone.

      render(<VaultPanel />);
      expect(screen.getByText(/unsaved changes were restored/)).toBeDefined();
      expect(value('Password')).toBe('typed-new');
    });
  });
});

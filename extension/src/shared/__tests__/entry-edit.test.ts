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

import { describe, it, expect } from 'vitest';
import type { EntryData } from '@passbubble/shared-ts';
import { applyEntryEdit } from '../entry-edit.js';

describe('applyEntryEdit and passkeys', () => {
  const passkey = {
    credential_id: 'q2D0bQ2u3E7XoWlS1yZbVw',
    rp_id: 'example.com',
    user_handle: 'dXNlci0xMjM0',
    user_name: 'alice@example.com',
    key_algorithm: -7,
    private_key: 'MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0w',
    sign_count: 0,
    discoverable: true,
    created_at: '2026-11-14T03:00:00Z',
    written_by_a_newer_client: { x: 1 },
  };

  it('keeps the passkeys when other fields change', () => {
    const data: EntryData = { username: 'alice@example.com', password: '', passkeys: [passkey] };
    applyEntryEdit(data, { fields: { password: 'now with a password', notes: 'n' }, tags: ['work'] });
    expect(data.passkeys).toEqual([passkey]);
    expect(data.password).toBe('now with a password');
  });

  it('keeps the server fields it was not asked to change, private key verbatim', () => {
    const key = '-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----\n';
    const data: EntryData = { host: 'srv.example', port: '2222', private_key: key };
    applyEntryEdit(data, { fields: { port: '22' } });
    expect(data).toEqual({ host: 'srv.example', port: '22', private_key: key });
  });
});

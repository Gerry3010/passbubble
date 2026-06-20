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
import { parsePsono } from '../psono-import.js';

describe('parsePsono', () => {
  it('maps website_password incl. url filter and folders', () => {
    const json = JSON.stringify({
      items: [
        {
          type: 'website_password',
          name: 'Root Site',
          website_password_title: 'Root Site',
          website_password_url: 'https://example.com',
          website_password_username: 'me',
          website_password_password: 'pw',
          urlfilter: 'example.com, *.example.com',
        },
      ],
      folders: [
        {
          name: 'Work',
          items: [
            {
              type: 'website_password',
              name: 'A Trust',
              website_password_title: 'A Trust',
              website_password_url: 'http://www.a-trust.at',
              website_password_username: '+436649252412',
              website_password_password: 'secret',
              website_password_url_filter: 'www.a-trust.at',
            },
          ],
        },
      ],
    });

    const res = parsePsono(json);
    expect(res.records).toHaveLength(2);

    const root = res.records[0];
    expect(root.name).toBe('Root Site');
    expect(root.type).toBe('password');
    expect(root.data.username).toBe('me');
    expect(root.matchPatterns).toEqual(['example.com', '*.example.com']);

    const nested = res.records[1];
    expect(nested.name).toBe('A Trust');
    expect(nested.matchPatterns).toEqual(['www.a-trust.at']);
  });

  it('classifies a website_password with TOTP as a totp entry', () => {
    const res = parsePsono(
      JSON.stringify({
        items: [
          {
            type: 'website_password',
            name: 'X',
            website_password_title: 'X',
            website_password_totp_code: 'JBSWY3DPEHPK3PXP',
          },
        ],
      }),
    );
    expect(res.records[0].type).toBe('totp');
    expect(res.records[0].data.totp_secret).toBe('JBSWY3DPEHPK3PXP');
  });

  it('skips unknown types with a warning', () => {
    const res = parsePsono(JSON.stringify({ items: [{ type: 'gpg_key', name: 'k' }] }));
    expect(res.records).toHaveLength(0);
    expect(res.skipped).toBe(1);
    expect(res.warnings.length).toBe(1);
  });

  it('leaves matchPatterns undefined when no url filter present', () => {
    const res = parsePsono(
      JSON.stringify({
        items: [{ type: 'note', name: 'N', note_title: 'N', note_notes: 'hi' }],
      }),
    );
    expect(res.records[0].matchPatterns).toBeUndefined();
  });
});

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

import { useState, useEffect } from 'react';
import browser from 'webextension-polyfill';
import { PassbubbleClient } from '@passbubble/shared-ts';
import { STORAGE_KEYS } from '../../shared/constants.js';
import { term, input, buttonPrimary, buttonGhost, withDisabled } from '../../shared/theme.js';

export function ServerUrlForm() {
  const [url, setUrl] = useState('');
  const [status, setStatus] = useState<'idle' | 'testing' | 'ok' | 'error'>('idle');
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    browser.storage.sync.get(STORAGE_KEYS.SERVER_URL).then((data) => {
      const stored = data[STORAGE_KEYS.SERVER_URL] as string | undefined;
      if (stored) setUrl(stored);
    });
  }, []);

  async function testConnection() {
    setStatus('testing');
    const normalised = url.replace(/\/$/, '');
    const client = new PassbubbleClient(normalised);
    const ok = await client.healthCheck();
    setStatus(ok ? 'ok' : 'error');
  }

  async function save() {
    const normalised = url.replace(/\/$/, '');
    await browser.storage.sync.set({ [STORAGE_KEYS.SERVER_URL]: normalised });
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '32px' }}>
      <h2 style={{ fontSize: '18px', fontWeight: 700, color: term.green, fontFamily: term.font }}># server configuration</h2>
      <label style={{ display: 'flex', flexDirection: 'column', gap: '4px', fontSize: '14px', color: term.muted }}>
        Backend URL
        <input
          type="url"
          value={url}
          onChange={(e) => { setUrl(e.target.value); setStatus('idle'); }}
          placeholder="https://passbubble.yourdomain.com"
          style={{ ...input, fontSize: '14px' }}
        />
      </label>
      {status === 'ok' && <p style={{ color: term.green, fontSize: '13px' }}>✓ Connected successfully</p>}
      {status === 'error' && <p style={{ color: term.red, fontSize: '13px' }}>✗ Cannot reach server</p>}
      <div style={{ display: 'flex', gap: '8px' }}>
        <button
          onClick={testConnection}
          disabled={!url || status === 'testing'}
          style={withDisabled({ ...buttonGhost, padding: '8px 16px' }, !url || status === 'testing')}
        >
          {status === 'testing' ? 'Testing…' : 'Test Connection'}
        </button>
        <button
          onClick={save}
          disabled={!url}
          style={withDisabled({ ...buttonPrimary, padding: '8px 16px' }, !url)}
        >
          {saved ? 'Saved!' : 'Save'}
        </button>
      </div>
    </section>
  );
}

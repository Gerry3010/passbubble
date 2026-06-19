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

import { useState } from 'react';
import browser from 'webextension-polyfill';
import { MessageType } from '../../shared/constants.js';
import { term, input, buttonPrimary, buttonGhost, link, errorText, withDisabled } from '../../shared/theme.js';

export function CreateEntryForm({ onCreated, onCancel }: { onCreated: () => void; onCancel: () => void }) {
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function generate() {
    try {
      const resp = (await browser.runtime.sendMessage({
        type: MessageType.GENERATE,
        payload: { length: 20, type: 'strong', count: 1 },
      })) as { passwords?: { password: string }[] };
      const pw = resp.passwords?.[0]?.password;
      if (pw) setPassword(pw);
    } catch {
      // ignore — user can type one manually
    }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) {
      setError('Name is required');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const resp = (await browser.runtime.sendMessage({
        type: MessageType.CREATE_ENTRY,
        payload: { name: name.trim(), type: 'password', url, data: { username, password } },
      })) as { locked?: boolean };
      if (resp?.locked) {
        setError('Vault is locked');
        return;
      }
      onCreated();
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
      <button type="button" onClick={onCancel} style={{ ...link, alignSelf: 'flex-start' }}>
        ‹ Back
      </button>
      <h3 style={{ margin: 0, fontSize: '15px', color: term.green }}># new entry</h3>
      {error && <p style={errorText}>{error}</p>}
      <input style={input} placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
      <input style={input} placeholder="URL" value={url} onChange={(e) => setUrl(e.target.value)} />
      <input style={input} placeholder="Username" value={username} onChange={(e) => setUsername(e.target.value)} />
      <div style={{ display: 'flex', gap: '6px' }}>
        <input
          style={input}
          placeholder="Password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <button type="button" onClick={() => void generate()} style={{ ...buttonGhost, padding: '0 10px', fontSize: '12px', whiteSpace: 'nowrap' }}>
          Generate
        </button>
      </div>
      <button
        type="submit"
        disabled={busy}
        style={withDisabled(buttonPrimary, busy)}
      >
        {busy ? 'Saving…' : 'Save entry'}
      </button>
    </form>
  );
}

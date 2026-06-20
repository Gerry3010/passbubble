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

import { useEffect, useState } from 'react';
import browser from 'webextension-polyfill';
import { MessageType } from '../../shared/constants.js';
import { term, input, buttonPrimary, buttonGhost, muted } from '../../shared/theme.js';

export function BlocklistSection() {
  const [list, setList] = useState<string[]>([]);
  const [value, setValue] = useState('');

  async function refresh() {
    const resp = (await browser.runtime.sendMessage({
      type: MessageType.BLOCKLIST_GET,
      payload: {},
    })) as { list?: string[] };
    setList(resp?.list ?? []);
  }

  useEffect(() => {
    void refresh();
  }, []);

  async function add(e: React.FormEvent) {
    e.preventDefault();
    const host = value.trim();
    if (!host) return;
    await browser.runtime.sendMessage({ type: MessageType.BLOCKLIST_ADD, payload: { host } });
    setValue('');
    await refresh();
  }

  async function remove(host: string) {
    await browser.runtime.sendMessage({ type: MessageType.BLOCKLIST_REMOVE, payload: { host } });
    await refresh();
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '32px' }}>
      <h2 style={{ fontSize: '18px', fontWeight: 700, color: term.green, fontFamily: term.font }}># save blocklist</h2>
      <p style={{ ...muted, fontSize: '13px' }}>
        Hosts/domains here never get a <strong>“save password?”</strong> prompt. Autofill still works.
        A domain (e.g. <code>example.com</code>) also covers its subdomains.
      </p>

      <form onSubmit={(e) => void add(e)} style={{ display: 'flex', gap: '6px' }}>
        <input
          type="text"
          placeholder="example.com or login.example.com"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          style={{ ...input, flex: 1 }}
        />
        <button type="submit" style={{ ...buttonPrimary, padding: '8px 16px' }}>Add</button>
      </form>

      {list.length === 0 ? (
        <p style={{ ...muted, fontSize: '13px' }}>No blocked sites.</p>
      ) : (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: '4px' }}>
          {list.map((host) => (
            <li
              key={host}
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                gap: '8px',
                border: `1px solid ${term.border}`,
                background: term.surface,
                borderRadius: '4px',
                padding: '6px 10px',
                fontSize: '13px',
                fontFamily: term.font,
                color: term.green,
              }}
            >
              <span style={{ overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>{host}</span>
              <button
                onClick={() => void remove(host)}
                style={{ ...buttonGhost, padding: '2px 10px', fontSize: '12px', color: term.red, flexShrink: 0 }}
              >
                remove
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

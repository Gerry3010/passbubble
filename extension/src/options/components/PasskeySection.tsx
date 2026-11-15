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

// Passkey settings (device-local, storage.local): whether Passbubble answers
// passkey requests, and the sites where the browser always handles them.

import { useEffect, useState } from 'react';
import browser from 'webextension-polyfill';
import { STORAGE_KEYS } from '../../shared/constants.js';
import { normalizeSiteEntry, passkeysUnsupportedIn, readPasskeySettings } from '../../shared/passkey-settings.js';
import { term, input, buttonPrimary, buttonGhost, muted } from '../../shared/theme.js';

export function PasskeySection() {
  const [loaded, setLoaded] = useState(false);
  const [enabled, setEnabled] = useState(true);
  const [list, setList] = useState<string[]>([]);
  const [value, setValue] = useState('');
  const [unsupportedIn, setUnsupportedIn] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    void (async () => {
      const [settings, unsupported] = await Promise.all([readPasskeySettings(), passkeysUnsupportedIn()]);
      setEnabled(settings.enabled);
      setList(settings.blocklist);
      setUnsupportedIn(unsupported);
      setLoaded(true);
    })();
  }, []);

  async function toggle(next: boolean) {
    setEnabled(next);
    await browser.storage.local.set({ [STORAGE_KEYS.PASSKEYS_ENABLED]: next });
  }

  async function saveList(next: string[]) {
    setList(next);
    await browser.storage.local.set({ [STORAGE_KEYS.PASSKEY_BLOCKLIST]: next });
  }

  async function add(e: React.FormEvent) {
    e.preventDefault();
    const site = normalizeSiteEntry(value);
    if (!site) {
      setError('Enter a host or domain, e.g. example.com');
      return;
    }
    setError(null);
    setValue('');
    if (!list.includes(site)) await saveList([...list, site]);
  }

  if (!loaded) return null;

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '32px' }}>
      <h2 style={{ fontSize: '18px', fontWeight: 700, color: term.green, fontFamily: term.font }}># passkeys</h2>
      {unsupportedIn ? (
        <p style={{ ...muted, fontSize: '13px' }}>
          Passkeys are not available in {unsupportedIn}. The browser handles them.
        </p>
      ) : (
        <>
          <label style={{ display: 'flex', alignItems: 'center', gap: '8px', color: term.green, fontFamily: term.font, fontSize: '13px' }}>
            <input type="checkbox" checked={enabled} onChange={(e) => void toggle(e.target.checked)} />
            Use Passbubble for passkeys
          </label>
          <p style={{ ...muted, fontSize: '13px' }}>
            When a site creates or asks for a passkey, Passbubble offers to save it in your vault or to sign in with
            one stored there. You can always choose another device instead. Turned off, the browser handles every
            passkey request.
          </p>

          <h3 style={{ fontSize: '14px', fontWeight: 700, color: term.green, fontFamily: term.font, margin: 0 }}>
            Never use Passbubble passkeys on
          </h3>
          <form onSubmit={(e) => void add(e)} style={{ display: 'flex', gap: '6px' }}>
            <input
              type="text"
              aria-label="Site to exclude from passkeys"
              placeholder="example.com or login.example.com"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              style={{ ...input, flex: 1 }}
            />
            <button type="submit" style={{ ...buttonPrimary, padding: '8px 16px' }}>
              Add
            </button>
          </form>
          {error && <p style={{ color: term.red, fontSize: '12px', margin: 0 }}>{error}</p>}
          {list.length === 0 ? (
            <p style={{ ...muted, fontSize: '13px' }}>No sites excluded.</p>
          ) : (
            <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: '4px' }}>
              {list.map((site) => (
                <li
                  key={site}
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
                  <span style={{ overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>{site}</span>
                  <button
                    onClick={() => void saveList(list.filter((s) => s !== site))}
                    aria-label={`Remove ${site}`}
                    style={{ ...buttonGhost, padding: '2px 10px', fontSize: '12px', color: term.red, flexShrink: 0 }}
                  >
                    remove
                  </button>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </section>
  );
}

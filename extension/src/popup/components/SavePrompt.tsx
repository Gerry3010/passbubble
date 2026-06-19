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

interface PendingSave {
  name: string;
  url: string;
  username: string;
  password: string;
}

function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

/** Offers to save credentials captured from a form submission on a site with no
 * matching entry. Nothing is saved until the user explicitly confirms. */
export function SavePrompt({ onSaved }: { onSaved?: () => void }) {
  const [pending, setPending] = useState<PendingSave | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    browser.runtime
      .sendMessage({ type: MessageType.GET_PENDING_SAVE, payload: {} })
      .then((p) => setPending((p as PendingSave) ?? null))
      .catch(() => setPending(null));
  }, []);

  if (!pending) return null;

  async function save() {
    setBusy(true);
    try {
      await browser.runtime.sendMessage({ type: MessageType.CONFIRM_SAVE, payload: {} });
      setPending(null);
      onSaved?.();
    } finally {
      setBusy(false);
    }
  }

  async function dismiss() {
    setBusy(true);
    try {
      await browser.runtime.sendMessage({
        type: MessageType.DISMISS_SAVE,
        payload: { host: hostOf(pending!.url) },
      });
      setPending(null);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div
      style={{
        margin: '8px',
        padding: '10px 12px',
        border: '1px solid #4299e1',
        background: '#ebf8ff',
        borderRadius: '6px',
        fontSize: '12px',
      }}
    >
      <div style={{ fontWeight: 600, marginBottom: '4px' }}>Save this login?</div>
      <div style={{ color: '#2d3748' }}>
        {hostOf(pending.url)}
        {pending.username ? ` — ${pending.username}` : ''}
      </div>
      <div style={{ display: 'flex', gap: '8px', marginTop: '8px' }}>
        <button
          onClick={() => void save()}
          disabled={busy}
          style={{
            padding: '6px 12px',
            background: '#4299e1',
            color: '#fff',
            border: 'none',
            borderRadius: '4px',
            cursor: busy ? 'not-allowed' : 'pointer',
            fontSize: '12px',
          }}
        >
          Save
        </button>
        <button
          onClick={() => void dismiss()}
          disabled={busy}
          style={{
            padding: '6px 12px',
            background: 'none',
            color: '#718096',
            border: '1px solid #e2e8f0',
            borderRadius: '4px',
            cursor: busy ? 'not-allowed' : 'pointer',
            fontSize: '12px',
          }}
        >
          Not now
        </button>
      </div>
    </div>
  );
}

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

// The passkeys of an entry in the detail view: site, user name and creation
// date. Key material is never shown. Deleting one asks first and goes through
// the background (PASSKEY_DELETE), which rewrites the entry under its key.

import { useState } from 'react';
import browser from 'webextension-polyfill';
import type { PasskeyRecord } from '@passbubble/shared-ts';
import { MessageType } from '../../shared/constants.js';
import { handleLockedResponse } from '../store/session.js';
import { term, link, errorText } from '../../shared/theme.js';

function createdOn(value: unknown): string {
  if (typeof value !== 'string' || !value) return '';
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? '' : d.toISOString().slice(0, 10);
}

export function PasskeyList({
  entryId,
  passkeys,
  onDeleted,
}: {
  entryId: string;
  passkeys: PasskeyRecord[];
  onDeleted: (credentialId: string) => void;
}) {
  const [confirming, setConfirming] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (passkeys.length === 0) return null;

  async function remove(credentialId: string) {
    setBusy(true);
    setError(null);
    try {
      const resp = (await browser.runtime.sendMessage({
        type: MessageType.PASSKEY_DELETE,
        payload: { entryId, credentialId },
      })) as { ok?: boolean; error?: string; locked?: boolean };
      if (handleLockedResponse(resp)) {
        setError('Vault is locked');
        return;
      }
      if (!resp?.ok) {
        setError(resp?.error ?? 'Could not delete the passkey.');
        return;
      }
      setConfirming(null);
      onDeleted(credentialId);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section aria-label="Passkeys" style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
      <span style={{ fontSize: '11px', color: term.green }}># passkeys</span>
      {passkeys.map((p) => {
        const date = createdOn(p.created_at);
        return (
          <div
            key={p.credential_id}
            data-testid="passkey"
            style={{ border: `1px solid ${term.border}`, background: term.surface, borderRadius: '4px', padding: '6px 8px' }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: '8px', fontSize: '10px', color: term.muted }}>
              <span>🔑 passkey{date ? ` · ${date}` : ''}</span>
              {confirming === p.credential_id ? (
                <span style={{ display: 'flex', gap: '8px' }}>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => void remove(p.credential_id)}
                    style={{ ...link, fontSize: '10px', color: term.red }}
                  >
                    {busy ? 'Deleting…' : 'Delete'}
                  </button>
                  <button type="button" disabled={busy} onClick={() => setConfirming(null)} style={{ ...link, fontSize: '10px' }}>
                    Keep
                  </button>
                </span>
              ) : (
                <button
                  type="button"
                  aria-label={`Delete passkey for ${p.rp_id}`}
                  onClick={() => setConfirming(p.credential_id)}
                  style={{ ...link, fontSize: '10px', color: term.red }}
                >
                  Delete
                </button>
              )}
            </div>
            <div style={{ fontSize: '13px', color: term.green, wordBreak: 'break-all', fontFamily: term.font }}>{p.rp_id}</div>
            {p.user_name && <div style={{ fontSize: '11px', color: term.muted, wordBreak: 'break-all' }}>{p.user_name}</div>}
            {confirming === p.credential_id && (
              <p style={{ fontSize: '11px', color: term.amber, margin: '4px 0 0' }}>
                Delete this passkey? You can no longer sign in to {p.rp_id} with it.
              </p>
            )}
          </div>
        );
      })}
      {error && (
        <p role="alert" style={errorText}>
          {error}
        </p>
      )}
    </section>
  );
}

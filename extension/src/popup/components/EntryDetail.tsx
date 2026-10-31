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
import { generateTotp, tagsOf } from '@passbubble/shared-ts';
import type { EntryData, EntryResponse } from '@passbubble/shared-ts';
import { MessageType } from '../../shared/constants.js';
import { handleLockedResponse } from '../store/session.js';
import { isSsoProvider, SSO_PROVIDER_LABELS } from '../../shared/sso.js';
import { term, link, muted, errorText, tagChip } from '../../shared/theme.js';

export function EntryDetail({ entry, onBack }: { entry: EntryResponse; onBack: () => void }) {
  const [data, setData] = useState<EntryData | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Secret fields the user has revealed, by field id — each Show toggles only
  // its own field.
  const [revealed, setRevealed] = useState<ReadonlySet<string>>(new Set());
  const [totp, setTotp] = useState<{ code: string; remaining: number } | null>(null);

  useEffect(() => {
    browser.runtime
      .sendMessage({ type: MessageType.GET_ENTRY, payload: { id: entry.id } })
      .then((resp) => {
        const r = resp as { data?: EntryData; locked?: boolean };
        if (handleLockedResponse(r) || !r.data) {
          setError('Vault is locked');
          return;
        }
        setData(r.data);
      })
      .catch((e) => setError(`Failed to load entry: ${e instanceof Error ? e.message : String(e)}`));
  }, [entry.id]);

  // Live TOTP code (recomputed every second) when the entry has a secret.
  useEffect(() => {
    const secret = data?.totp_secret;
    if (!secret) return;
    let active = true;
    const tick = async () => {
      try {
        const r = await generateTotp(secret);
        if (active) setTotp({ code: r.code, remaining: r.secondsRemaining });
      } catch {
        if (active) setTotp(null);
      }
    };
    void tick();
    const id = setInterval(() => void tick(), 1000);
    return () => {
      active = false;
      clearInterval(id);
    };
  }, [data?.totp_secret]);

  const tags = tagsOf(data);

  function copy(value: string) {
    void navigator.clipboard.writeText(value);
  }

  function toggleReveal(id: string) {
    setRevealed((prev) => {
      const next = new Set(prev);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  }

  function revealButton(id: string) {
    return (
      <button onClick={() => toggleReveal(id)} style={link}>
        {revealed.has(id) ? 'Hide' : 'Show'}
      </button>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      <button onClick={onBack} style={link}>‹ Back</button>
      <h3 style={{ margin: 0, fontSize: '15px', color: term.green }}>{entry.name}</h3>
      {entry.url && <div style={{ color: term.muted, fontSize: '11px' }}>{entry.url}</div>}
      {tags.length > 0 && (
        <ul aria-label="Tags" style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexWrap: 'wrap', gap: '4px' }}>
          {tags.map((t) => (
            <li key={t} style={{ ...tagChip, cursor: 'default' }}>
              #{t}
            </li>
          ))}
        </ul>
      )}

      {error && <p style={errorText}>{error}</p>}
      {!data && !error && <p style={muted}>Loading…</p>}

      {data && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          {data.username && <Field label="Username" value={data.username} onCopy={() => copy(data.username!)} />}
          {isSsoProvider(data.sign_in_with) && (
            <div style={{ color: term.muted, fontSize: '12px' }}>
              → Signs in with <b style={{ color: term.text }}>{SSO_PROVIDER_LABELS[data.sign_in_with]}</b>
            </div>
          )}
          {data.password && (
            <Field
              label="Password"
              value={revealed.has('password') ? data.password : '••••••••'}
              onCopy={() => copy(data.password!)}
              extra={revealButton('password')}
            />
          )}
          {totp && (
            <Field
              label={`TOTP (${totp.remaining}s)`}
              value={totp.code}
              onCopy={() => copy(totp.code)}
            />
          )}
          {TYPED_FIELDS.map(([key, label, secret]) => {
            const value = data[key];
            if (typeof value !== 'string' || !value) return null;
            return (
              <Field
                key={key}
                label={label}
                value={secret && !revealed.has(key) ? mask(key, value) : value}
                onCopy={() => copy(value)}
                extra={secret ? revealButton(key) : undefined}
              />
            );
          })}
          {(data.custom_fields ?? []).map((cf, i) => {
            if (!cf.value) return null;
            const id = `cf-${i}`;
            const secret = SECRET_CUSTOM_TYPES.has(cf.type ?? '');
            return (
              <Field
                key={id}
                label={cf.label || 'Custom'}
                value={secret && !revealed.has(id) ? mask(id, cf.value) : cf.value}
                onCopy={() => copy(cf.value)}
                extra={secret ? revealButton(id) : undefined}
              />
            );
          })}
          {data.notes && <Field label="Notes" value={data.notes} onCopy={() => copy(data.notes!)} />}
        </div>
      )}
    </div>
  );
}

// Non-login fields rendered when present, in display order. `secret` fields
// are masked until revealed.
const TYPED_FIELDS: Array<[keyof EntryData & string, string, boolean?]> = [
  ['card_number', 'Card number', true],
  ['holder_name', 'Cardholder'],
  ['expiry_month', 'Expiry month'],
  ['expiry_year', 'Expiry year'],
  ['cvv', 'CVV', true],
  ['bank_name', 'Bank'],
  ['iban', 'IBAN'],
  ['bic', 'BIC'],
  ['account_number', 'Account number'],
  ['title', 'Title'],
  ['first_name', 'First name'],
  ['last_name', 'Last name'],
  ['company', 'Company'],
  ['email', 'Email'],
  ['phone', 'Phone'],
  ['street', 'Street'],
  ['postal_code', 'Postal code'],
  ['city', 'City'],
  ['state', 'State'],
  ['country', 'Country'],
  ['product_name', 'Product'],
  ['license_key', 'License key', true],
  ['purchase_email', 'Purchase email'],
];

// Custom field types the app masks until revealed.
const SECRET_CUSTOM_TYPES = new Set(['password', 'ssh']);

function mask(key: string, value: string): string {
  if (key === 'card_number') {
    const digits = value.replace(/\D/g, '');
    return digits.length >= 4 ? `•••• ${digits.slice(-4)}` : '••••';
  }
  return '•'.repeat(Math.min(value.length, 8));
}

function Field({
  label,
  value,
  onCopy,
  extra,
}: {
  label: string;
  value: string;
  onCopy: () => void;
  extra?: React.ReactNode;
}) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    onCopy();
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <div style={{ border: `1px solid ${term.border}`, background: term.surface, borderRadius: '4px', padding: '6px 8px' }}>
      <div style={{ fontSize: '10px', color: term.muted, display: 'flex', justifyContent: 'space-between' }}>
        <span>{label}</span>
        <span style={{ display: 'flex', gap: '8px' }}>
          {extra}
          <button onClick={handleCopy} style={{ ...link, color: copied ? term.green : undefined }}>
            {copied ? 'Copied!' : 'Copy'}
          </button>
        </span>
      </div>
      <div style={{ fontSize: '13px', wordBreak: 'break-all', fontFamily: term.font, color: term.green }}>{value}</div>
    </div>
  );
}

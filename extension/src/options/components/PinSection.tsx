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
import { MessageType, STORAGE_KEYS } from '../../shared/constants.js';
import { term, input, buttonPrimary, buttonGhost, muted, errorText } from '../../shared/theme.js';

interface PinStatus {
  enabled: boolean;
  expired?: boolean;
  intervalDays?: number;
}

const WARN_COLOR = '#d08a1e';

export function PinSection() {
  const [status, setStatus] = useState<PinStatus | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [masterPassword, setMasterPassword] = useState('');
  const [pin, setPin] = useState('');
  const [pin2, setPin2] = useState('');
  const [intervalDays, setIntervalDays] = useState('14');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function refresh() {
    const resp = (await browser.runtime.sendMessage({
      type: MessageType.GET_PIN_STATUS,
      payload: {},
    })) as PinStatus;
    setStatus(resp);
  }

  useEffect(() => {
    void refresh();
    // Keep in sync when the PIN state changes elsewhere (e.g. a logout in
    // AccountSection wipes the PIN, or a lockout clears it in the background).
    const onChanged = (
      changes: Record<string, browser.Storage.StorageChange>,
      area: string,
    ) => {
      if (area === 'local' && STORAGE_KEYS.PIN_ENABLED in changes) void refresh();
    };
    browser.storage.onChanged.addListener(onChanged);
    return () => browser.storage.onChanged.removeListener(onChanged);
  }, []);

  function resetForm() {
    setMasterPassword('');
    setPin('');
    setPin2('');
    setError(null);
    setShowForm(false);
  }

  async function enable(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (pin !== pin2) {
      setError('PINs do not match');
      return;
    }
    if (!/^\d{4,}$/.test(pin)) {
      setError('PIN must be at least 4 digits');
      return;
    }
    const days = Math.min(60, Math.max(1, parseInt(intervalDays, 10) || 14));
    setBusy(true);
    try {
      const resp = (await browser.runtime.sendMessage({
        type: MessageType.SET_PIN,
        payload: { masterPassword, pin, intervalDays: days },
      })) as { ok?: boolean };
      if (!resp?.ok) {
        setError('Could not set PIN');
        return;
      }
      resetForm();
      await refresh();
    } catch (err) {
      setError(String(err));
    } finally {
      setBusy(false);
    }
  }

  async function disable() {
    setBusy(true);
    try {
      await browser.runtime.sendMessage({ type: MessageType.DISABLE_PIN, payload: {} });
      await refresh();
    } finally {
      setBusy(false);
    }
  }

  const enabled = status?.enabled ?? false;

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '32px' }}>
      <h2 style={{ fontSize: '18px', fontWeight: 700, color: term.green, fontFamily: term.font }}># pin quick-unlock</h2>

      <p style={{ color: WARN_COLOR, fontSize: '13px', margin: 0 }}>
        ⚠ A PIN is <strong>less secure</strong> than your master password. It is stored on this
        device (browser storage on disk, not a hardware keystore), so a short PIN could be
        brute-forced by someone with access to this machine. Use it only on trusted devices.
      </p>

      {enabled ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          <p style={{ ...muted, fontSize: '13px', margin: 0 }}>
            PIN unlock is <strong style={{ color: term.green }}>on</strong> — master password
            required every <strong>{status?.intervalDays ?? 14}</strong> days
            {status?.expired ? ' (expired — master password required next unlock)' : ''}.
          </p>
          <div style={{ display: 'flex', gap: '6px' }}>
            <button onClick={() => setShowForm((v) => !v)} style={{ ...buttonGhost, padding: '8px 16px' }}>
              {showForm ? 'cancel' : 'change PIN'}
            </button>
            <button
              onClick={() => void disable()}
              disabled={busy}
              style={{ ...buttonGhost, padding: '8px 16px', color: term.red }}
            >
              disable PIN
            </button>
          </div>
        </div>
      ) : (
        !showForm && (
          <button onClick={() => setShowForm(true)} style={{ ...buttonPrimary, padding: '8px 16px', alignSelf: 'flex-start' }}>
            Set up PIN
          </button>
        )
      )}

      {showForm && (
        <form onSubmit={(e) => void enable(e)} style={{ display: 'flex', flexDirection: 'column', gap: '8px', maxWidth: '360px' }}>
          {error && <p style={errorText}>{error}</p>}
          <input
            type="password"
            placeholder="Master password"
            value={masterPassword}
            onChange={(e) => setMasterPassword(e.target.value)}
            required
            style={input}
          />
          <input
            type="password"
            inputMode="numeric"
            placeholder="New PIN (digits)"
            value={pin}
            onChange={(e) => setPin(e.target.value)}
            required
            style={input}
          />
          <input
            type="password"
            inputMode="numeric"
            placeholder="Confirm PIN"
            value={pin2}
            onChange={(e) => setPin2(e.target.value)}
            required
            style={input}
          />
          <label style={{ ...muted, fontSize: '12px', display: 'flex', alignItems: 'center', gap: '8px' }}>
            Require master password after
            <input
              type="number"
              min={1}
              max={60}
              value={intervalDays}
              onChange={(e) => setIntervalDays(e.target.value)}
              style={{ ...input, width: '64px' }}
            />
            days (1–60)
          </label>
          <button type="submit" disabled={busy} style={{ ...buttonPrimary, padding: '8px 16px', alignSelf: 'flex-start' }}>
            {busy ? 'Saving…' : 'Save PIN'}
          </button>
        </form>
      )}
    </section>
  );
}

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
import { STORAGE_KEYS, AUTO_LOCK_DEFAULT_MINUTES } from '../../shared/constants.js';
import { term, input, muted } from '../../shared/theme.js';

// 0 = never (lock only on browser close / service-worker eviction).
const OPTIONS: { value: number; label: string }[] = [
  { value: 1, label: '1 minute' },
  { value: 5, label: '5 minutes' },
  { value: 15, label: '15 minutes' },
  { value: 30, label: '30 minutes' },
  { value: 60, label: '1 hour' },
  { value: 0, label: 'Never (lock only on browser close)' },
];

export function AutoLockSection() {
  const [minutes, setMinutes] = useState<number>(AUTO_LOCK_DEFAULT_MINUTES);

  useEffect(() => {
    void (async () => {
      const data = await browser.storage.sync.get(STORAGE_KEYS.AUTO_LOCK_MINUTES);
      const raw = data[STORAGE_KEYS.AUTO_LOCK_MINUTES];
      const n = typeof raw === 'number' ? raw : Number(raw);
      setMinutes(Number.isFinite(n) && n >= 0 ? n : AUTO_LOCK_DEFAULT_MINUTES);
    })();
  }, []);

  async function change(value: number) {
    setMinutes(value);
    await browser.storage.sync.set({ [STORAGE_KEYS.AUTO_LOCK_MINUTES]: value });
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '32px' }}>
      <h2 style={{ fontSize: '18px', fontWeight: 700, color: term.green, fontFamily: term.font }}># auto-lock</h2>
      <p style={{ ...muted, fontSize: '13px' }}>
        Lock the vault automatically after a period of inactivity. You will need your master
        password or PIN to unlock again.
      </p>
      <label style={{ ...muted, fontSize: '13px', display: 'flex', alignItems: 'center', gap: '8px' }}>
        Lock after
        <select
          value={minutes}
          onChange={(e) => void change(Number(e.target.value))}
          style={{ ...input, width: 'auto' }}
        >
          {OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </label>
    </section>
  );
}

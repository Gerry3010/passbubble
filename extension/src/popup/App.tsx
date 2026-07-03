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
import { useSessionStore } from './store/session.js';
import { LoginForm } from './components/LoginForm.js';
import { TotpCodeForm } from './components/TotpCodeForm.js';
import { SavePrompt } from './components/SavePrompt.js';
import { MasterPasswordPrompt } from './components/MasterPasswordPrompt.js';
import { VaultPanel } from './components/VaultPanel.js';
import { GeneratorPanel } from './components/GeneratorPanel.js';
import { HealthPanel } from './components/HealthPanel.js';
import browser from 'webextension-polyfill';
import { STORAGE_KEYS } from '../shared/constants.js';
import { term, buttonPrimary } from '../shared/theme.js';

type Tab = 'vault' | 'generator' | 'health';

export function App() {
  const { isLoggedIn, isUnlocked, userName, userEmail, lock, checkSession, isLoading, totpRequired } =
    useSessionStore();
  const accountLabel = userName || userEmail;
  const [tab, setTab] = useState<Tab>('vault');
  const [hasServerUrl, setHasServerUrl] = useState<boolean | null>(null);

  useEffect(() => {
    // Check server URL config first
    browser.storage.sync.get(STORAGE_KEYS.SERVER_URL).then((data) => {
      const url = data[STORAGE_KEYS.SERVER_URL] as string | undefined;
      setHasServerUrl(!!url);
      if (url) void checkSession();
    });
  }, []);

  if (hasServerUrl === null || isLoading) {
    return (
      <div style={{ padding: '16px', color: term.muted, fontSize: '13px' }}>Loading…</div>
    );
  }

  if (!hasServerUrl) {
    return (
      <div style={{ padding: '16px', display: 'flex', flexDirection: 'column', gap: '10px' }}>
        <h2 style={{ fontSize: '16px', fontWeight: 700, margin: 0, color: term.green }}>Welcome to Passbubble</h2>
        <p style={{ color: term.muted, fontSize: '12px' }}>Configure your server URL to get started.</p>
        <button
          onClick={() => browser.runtime.openOptionsPage()}
          style={buttonPrimary}
        >
          Open Settings
        </button>
      </div>
    );
  }

  if (!isLoggedIn) {
    return (
      <div style={{ padding: '16px' }}>
        {totpRequired ? <TotpCodeForm /> : <LoginForm />}
      </div>
    );
  }

  if (!isUnlocked) {
    return (
      <div style={{ padding: '16px' }}>
        <MasterPasswordPrompt />
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Header */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '10px 14px',
          borderBottom: `1px solid ${term.border}`,
          background: term.surface,
        }}
      >
        <span style={{ fontWeight: 700, fontSize: '14px', color: term.green }}>
          <span style={{ color: term.muted }}>&gt;_</span> passbubble
        </span>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          {accountLabel && (
            <button
              onClick={() => browser.runtime.openOptionsPage()}
              title="Account settings"
              style={{ background: 'none', border: 'none', cursor: 'pointer', color: term.green, fontSize: '11px', fontFamily: term.font, padding: 0 }}
            >
              [{accountLabel}]
            </button>
          )}
          <button
            onClick={() => void lock()}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: term.muted, fontSize: '11px', fontFamily: term.font }}
          >
            [lock]
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', borderBottom: `1px solid ${term.border}` }}>
        {(['vault', 'generator', 'health'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              flex: 1,
              padding: '8px',
              background: tab === t ? term.bg : term.surface,
              border: 'none',
              borderBottom: tab === t ? `2px solid ${term.green}` : '2px solid transparent',
              cursor: 'pointer',
              fontFamily: term.font,
              fontSize: '12px',
              fontWeight: tab === t ? 700 : 400,
              color: tab === t ? term.green : term.muted,
            }}
          >
            {t === 'vault' ? './vault' : t === 'generator' ? './generator' : './health'}
          </button>
        ))}
      </div>

      {/* Save-credentials offer (only on the vault tab) */}
      {tab === 'vault' && <SavePrompt />}

      {/* Content */}
      <div style={{ padding: '12px', flex: 1, overflowY: 'auto' }}>
        {tab === 'vault' ? <VaultPanel /> : tab === 'generator' ? <GeneratorPanel /> : <HealthPanel />}
      </div>
    </div>
  );
}

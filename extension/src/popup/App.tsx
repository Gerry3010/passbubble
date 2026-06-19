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
import { EntryList } from './components/EntryList.js';
import { GeneratorPanel } from './components/GeneratorPanel.js';
import browser from 'webextension-polyfill';
import { STORAGE_KEYS } from '../shared/constants.js';

type Tab = 'vault' | 'generator';

export function App() {
  const { isLoggedIn, isUnlocked, userName, lock, checkSession, isLoading, totpRequired } =
    useSessionStore();
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
      <div style={{ padding: '16px', color: '#718096', fontSize: '13px' }}>Loading…</div>
    );
  }

  if (!hasServerUrl) {
    return (
      <div style={{ padding: '16px', display: 'flex', flexDirection: 'column', gap: '10px' }}>
        <h2 style={{ fontSize: '16px', fontWeight: 600, margin: 0 }}>Welcome to Passbubble</h2>
        <p style={{ color: '#718096', fontSize: '12px' }}>Configure your server URL to get started.</p>
        <button
          onClick={() => browser.runtime.openOptionsPage()}
          style={{
            padding: '8px',
            background: '#4299e1',
            color: '#fff',
            border: 'none',
            borderRadius: '4px',
            cursor: 'pointer',
            fontSize: '13px',
          }}
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
          borderBottom: '1px solid #e2e8f0',
          background: '#f7fafc',
        }}
      >
        <span style={{ fontWeight: 700, fontSize: '14px', color: '#2d3748' }}>🔐 Passbubble</span>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          {userName && <span style={{ fontSize: '11px', color: '#718096' }}>{userName}</span>}
          <button
            onClick={() => void lock()}
            style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#718096', fontSize: '11px' }}
          >
            Lock
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', borderBottom: '1px solid #e2e8f0' }}>
        {(['vault', 'generator'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              flex: 1,
              padding: '8px',
              background: tab === t ? '#fff' : '#f7fafc',
              border: 'none',
              borderBottom: tab === t ? '2px solid #4299e1' : '2px solid transparent',
              cursor: 'pointer',
              fontSize: '12px',
              fontWeight: tab === t ? 600 : 400,
              color: tab === t ? '#4299e1' : '#718096',
              textTransform: 'capitalize',
            }}
          >
            {t === 'vault' ? 'Vault' : 'Generator'}
          </button>
        ))}
      </div>

      {/* Save-credentials offer (only on the vault tab) */}
      {tab === 'vault' && <SavePrompt />}

      {/* Content */}
      <div style={{ padding: '12px', flex: 1, overflowY: 'auto' }}>
        {tab === 'vault' ? <EntryList /> : <GeneratorPanel />}
      </div>
    </div>
  );
}

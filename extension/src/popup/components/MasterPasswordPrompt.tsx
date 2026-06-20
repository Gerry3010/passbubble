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
import { useSessionStore } from '../store/session.js';
import { term, input, buttonPrimary, muted, errorText, withDisabled } from '../../shared/theme.js';

export function MasterPasswordPrompt() {
  const { unlock, unlockWithPin, logout, isLoading, error, pinAvailable } = useSessionStore();
  // Prefer the PIN when it is available; the user can switch to the master password.
  const [usePin, setUsePin] = useState(pinAvailable);
  const [masterPassword, setMasterPassword] = useState('');
  const [pin, setPin] = useState('');

  const pinMode = usePin && pinAvailable;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (pinMode) {
      await unlockWithPin(pin);
      setPin('');
    } else {
      await unlock(masterPassword);
      setMasterPassword('');
    }
  }

  return (
    <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      <h2 style={{ fontSize: '16px', fontWeight: 700, margin: 0, color: term.green }}>$ unlock vault</h2>
      <p style={muted}>{pinMode ? 'Enter your PIN to unlock' : 'Enter your master password to unlock'}</p>
      {error && <p style={errorText}>{error}</p>}
      {pinMode ? (
        <input
          type="password"
          inputMode="numeric"
          autoComplete="off"
          placeholder="PIN"
          value={pin}
          onChange={(e) => setPin(e.target.value)}
          required
          autoFocus
          style={input}
        />
      ) : (
        <input
          type="password"
          placeholder="Master password"
          value={masterPassword}
          onChange={(e) => setMasterPassword(e.target.value)}
          required
          autoFocus
          style={input}
        />
      )}
      <button type="submit" disabled={isLoading} style={withDisabled(buttonPrimary, isLoading)}>
        {isLoading ? 'Unlocking…' : 'Unlock'}
      </button>
      {pinAvailable && (
        <button
          type="button"
          onClick={() => {
            setUsePin((v) => !v);
            useSessionStore.getState().clearError();
          }}
          style={linkButton}
        >
          {pinMode ? '› use master password instead' : '› use PIN instead'}
        </button>
      )}
      <button type="button" onClick={() => void logout()} disabled={isLoading} style={linkButton}>
        › log out
      </button>
    </form>
  );
}

const linkButton: React.CSSProperties = {
  background: 'none',
  border: 'none',
  cursor: 'pointer',
  color: term.muted,
  fontSize: '11px',
  fontFamily: term.font,
  padding: 0,
  textAlign: 'left',
};

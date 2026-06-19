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
  const [masterPassword, setMasterPassword] = useState('');
  const { unlock, isLoading, error } = useSessionStore();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    await unlock(masterPassword);
    setMasterPassword(''); // Clear after submit — best effort
  }

  return (
    <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      <h2 style={{ fontSize: '16px', fontWeight: 700, margin: 0, color: term.green }}>$ unlock vault</h2>
      <p style={muted}>
        Enter your master password to unlock
      </p>
      {error && <p style={errorText}>{error}</p>}
      <input
        type="password"
        placeholder="Master password"
        value={masterPassword}
        onChange={(e) => setMasterPassword(e.target.value)}
        required
        autoFocus
        style={input}
      />
      <button
        type="submit"
        disabled={isLoading}
        style={withDisabled(buttonPrimary, isLoading)}
      >
        {isLoading ? 'Unlocking…' : 'Unlock'}
      </button>
    </form>
  );
}

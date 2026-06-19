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
import { term, input, buttonPrimary, link, muted, errorText, withDisabled } from '../../shared/theme.js';

/** Second step of a 2FA login: prompts for the 6-digit TOTP code. */
export function TotpCodeForm() {
  const [code, setCode] = useState('');
  const { verifyTotp, cancelTotp, isLoading, error } = useSessionStore();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    await verifyTotp(code.trim());
  }

  return (
    <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      <h2 style={{ fontSize: '16px', fontWeight: 700, margin: 0, color: term.green }}>$ 2fa code</h2>
      <p style={muted}>
        Enter the 6-digit code from your authenticator app.
      </p>
      {error && <p style={errorText}>{error}</p>}
      <input
        type="text"
        inputMode="numeric"
        placeholder="123456"
        value={code}
        onChange={(e) => setCode(e.target.value)}
        required
        autoFocus
        style={{ ...input, letterSpacing: '4px' }}
      />
      <button
        type="submit"
        disabled={isLoading}
        style={withDisabled(buttonPrimary, isLoading)}
      >
        {isLoading ? 'Verifying…' : 'Verify'}
      </button>
      <button
        type="button"
        onClick={() => cancelTotp()}
        style={{ ...link, color: term.muted }}
      >
        ‹ Back to sign in
      </button>
    </form>
  );
}

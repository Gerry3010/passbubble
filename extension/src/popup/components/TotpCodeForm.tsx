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
      <h2 style={{ fontSize: '16px', fontWeight: 600, margin: 0 }}>Two-factor code</h2>
      <p style={{ color: '#718096', fontSize: '12px', margin: 0 }}>
        Enter the 6-digit code from your authenticator app.
      </p>
      {error && <p style={{ color: '#e53e3e', fontSize: '12px', margin: 0 }}>{error}</p>}
      <input
        type="text"
        inputMode="numeric"
        placeholder="123456"
        value={code}
        onChange={(e) => setCode(e.target.value)}
        required
        autoFocus
        style={{ padding: '8px', borderRadius: '4px', border: '1px solid #e2e8f0', fontSize: '13px', letterSpacing: '4px' }}
      />
      <button
        type="submit"
        disabled={isLoading}
        style={{
          padding: '8px',
          background: '#4299e1',
          color: '#fff',
          border: 'none',
          borderRadius: '4px',
          cursor: isLoading ? 'not-allowed' : 'pointer',
          fontSize: '13px',
          fontWeight: 500,
        }}
      >
        {isLoading ? 'Verifying…' : 'Verify'}
      </button>
      <button
        type="button"
        onClick={() => cancelTotp()}
        style={{ background: 'none', border: 'none', color: '#718096', fontSize: '12px', cursor: 'pointer' }}
      >
        Back to sign in
      </button>
    </form>
  );
}

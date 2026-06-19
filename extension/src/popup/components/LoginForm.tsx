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
import { term, input, buttonPrimary, errorText, withDisabled } from '../../shared/theme.js';

export function LoginForm() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const { login, isLoading, error } = useSessionStore();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    await login(email, password);
  }

  return (
    <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      <h2 style={{ fontSize: '16px', fontWeight: 700, margin: 0, color: term.green }}>$ login</h2>
      {error && <p style={errorText}>{error}</p>}
      <input
        type="email"
        placeholder="Email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        required
        autoFocus
        style={input}
      />
      <input
        type="password"
        placeholder="Password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        required
        style={input}
      />
      <button
        type="submit"
        disabled={isLoading}
        style={withDisabled(buttonPrimary, isLoading)}
      >
        {isLoading ? 'Signing in…' : 'Sign in'}
      </button>
    </form>
  );
}

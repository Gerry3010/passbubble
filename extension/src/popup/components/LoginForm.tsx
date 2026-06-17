import { useState } from 'react';
import { useSessionStore } from '../store/session.js';

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
      <h2 style={{ fontSize: '16px', fontWeight: 600, margin: 0 }}>Sign in to Passbubble</h2>
      {error && <p style={{ color: '#e53e3e', fontSize: '12px', margin: 0 }}>{error}</p>}
      <input
        type="email"
        placeholder="Email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        required
        autoFocus
        style={{ padding: '8px', borderRadius: '4px', border: '1px solid #e2e8f0', fontSize: '13px' }}
      />
      <input
        type="password"
        placeholder="Password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        required
        style={{ padding: '8px', borderRadius: '4px', border: '1px solid #e2e8f0', fontSize: '13px' }}
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
        {isLoading ? 'Signing in…' : 'Sign in'}
      </button>
    </form>
  );
}

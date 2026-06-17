import { useState } from 'react';
import { useSessionStore } from '../store/session.js';

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
      <h2 style={{ fontSize: '16px', fontWeight: 600, margin: 0 }}>Unlock Vault</h2>
      <p style={{ color: '#718096', fontSize: '12px', margin: 0 }}>
        Enter your master password to unlock
      </p>
      {error && <p style={{ color: '#e53e3e', fontSize: '12px', margin: 0 }}>{error}</p>}
      <input
        type="password"
        placeholder="Master password"
        value={masterPassword}
        onChange={(e) => setMasterPassword(e.target.value)}
        required
        autoFocus
        style={{ padding: '8px', borderRadius: '4px', border: '1px solid #e2e8f0', fontSize: '13px' }}
      />
      <button
        type="submit"
        disabled={isLoading}
        style={{
          padding: '8px',
          background: '#48bb78',
          color: '#fff',
          border: 'none',
          borderRadius: '4px',
          cursor: isLoading ? 'not-allowed' : 'pointer',
          fontSize: '13px',
          fontWeight: 500,
        }}
      >
        {isLoading ? 'Unlocking…' : 'Unlock'}
      </button>
    </form>
  );
}

import { useState, useEffect } from 'react';
import browser from 'webextension-polyfill';
import { PassbubbleClient } from '@passbubble/shared-ts';
import { STORAGE_KEYS } from '../../shared/constants.js';

export function ServerUrlForm() {
  const [url, setUrl] = useState('');
  const [status, setStatus] = useState<'idle' | 'testing' | 'ok' | 'error'>('idle');
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    browser.storage.sync.get(STORAGE_KEYS.SERVER_URL).then((data) => {
      const stored = data[STORAGE_KEYS.SERVER_URL] as string | undefined;
      if (stored) setUrl(stored);
    });
  }, []);

  async function testConnection() {
    setStatus('testing');
    const normalised = url.replace(/\/$/, '');
    const client = new PassbubbleClient(normalised);
    const ok = await client.healthCheck();
    setStatus(ok ? 'ok' : 'error');
  }

  async function save() {
    const normalised = url.replace(/\/$/, '');
    await browser.storage.sync.set({ [STORAGE_KEYS.SERVER_URL]: normalised });
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '32px' }}>
      <h2 style={{ fontSize: '18px', fontWeight: 600 }}>Server Configuration</h2>
      <label style={{ display: 'flex', flexDirection: 'column', gap: '4px', fontSize: '14px' }}>
        Backend URL
        <input
          type="url"
          value={url}
          onChange={(e) => { setUrl(e.target.value); setStatus('idle'); }}
          placeholder="https://passbubble.yourdomain.com"
          style={{ padding: '8px', borderRadius: '4px', border: '1px solid #e2e8f0', fontSize: '14px' }}
        />
      </label>
      {status === 'ok' && <p style={{ color: '#48bb78', fontSize: '13px' }}>✓ Connected successfully</p>}
      {status === 'error' && <p style={{ color: '#e53e3e', fontSize: '13px' }}>✗ Cannot reach server</p>}
      <div style={{ display: 'flex', gap: '8px' }}>
        <button
          onClick={testConnection}
          disabled={!url || status === 'testing'}
          style={{ padding: '8px 16px', borderRadius: '4px', border: '1px solid #e2e8f0', cursor: 'pointer', fontSize: '13px' }}
        >
          {status === 'testing' ? 'Testing…' : 'Test Connection'}
        </button>
        <button
          onClick={save}
          disabled={!url}
          style={{ padding: '8px 16px', borderRadius: '4px', background: '#4299e1', color: '#fff', border: 'none', cursor: 'pointer', fontSize: '13px' }}
        >
          {saved ? 'Saved!' : 'Save'}
        </button>
      </div>
    </section>
  );
}

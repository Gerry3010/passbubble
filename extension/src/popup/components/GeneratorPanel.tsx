import { useState } from 'react';
import browser from 'webextension-polyfill';
import { MessageType } from '../../shared/constants.js';
import type { GenerateResponse } from '@passbubble/shared-ts';

export function GeneratorPanel() {
  const [length, setLength] = useState(20);
  const [symbols, setSymbols] = useState(true);
  const [generated, setGenerated] = useState('');
  const [strength, setStrength] = useState(0);
  const [copied, setCopied] = useState(false);
  const [isLoading, setIsLoading] = useState(false);

  async function generate() {
    setIsLoading(true);
    const resp = await browser.runtime.sendMessage({
      type: MessageType.GENERATE,
      payload: { length, include_symbols: symbols, count: 1 },
    }) as GenerateResponse | { locked: boolean };
    setIsLoading(false);
    if ('locked' in resp) return;
    const first = resp.passwords[0];
    if (first) {
      setGenerated(first.password);
      setStrength(first.strength);
    }
  }

  async function copy() {
    if (!generated) return;
    await navigator.clipboard.writeText(generated);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  const strengthColor = strength >= 80 ? '#48bb78' : strength >= 50 ? '#ed8936' : '#e53e3e';

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      <h3 style={{ fontSize: '14px', fontWeight: 600, margin: 0 }}>Password Generator</h3>
      <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
        <label style={{ fontSize: '12px', color: '#4a5568' }}>Length: {length}</label>
        <input
          type="range"
          min={8}
          max={64}
          value={length}
          onChange={(e) => setLength(Number(e.target.value))}
          style={{ flex: 1 }}
        />
      </div>
      <label style={{ display: 'flex', gap: '6px', alignItems: 'center', fontSize: '12px', color: '#4a5568' }}>
        <input type="checkbox" checked={symbols} onChange={(e) => setSymbols(e.target.checked)} />
        Include symbols
      </label>
      <button
        onClick={generate}
        disabled={isLoading}
        style={{
          padding: '8px',
          background: '#667eea',
          color: '#fff',
          border: 'none',
          borderRadius: '4px',
          cursor: isLoading ? 'not-allowed' : 'pointer',
          fontSize: '13px',
        }}
      >
        {isLoading ? 'Generating…' : 'Generate'}
      </button>
      {generated && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
          <div
            style={{
              fontFamily: 'monospace',
              fontSize: '12px',
              padding: '8px',
              background: '#f7fafc',
              borderRadius: '4px',
              wordBreak: 'break-all',
            }}
          >
            {generated}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <div
              style={{
                flex: 1,
                height: '4px',
                background: '#e2e8f0',
                borderRadius: '2px',
                overflow: 'hidden',
              }}
            >
              <div
                style={{
                  width: `${strength}%`,
                  height: '100%',
                  background: strengthColor,
                  transition: 'width 0.3s',
                }}
              />
            </div>
            <span style={{ fontSize: '11px', color: strengthColor, fontWeight: 500 }}>
              {strength}/100
            </span>
          </div>
          <button
            onClick={copy}
            style={{
              padding: '6px',
              background: copied ? '#48bb78' : '#fff',
              color: copied ? '#fff' : '#4a5568',
              border: '1px solid #e2e8f0',
              borderRadius: '4px',
              cursor: 'pointer',
              fontSize: '12px',
            }}
          >
            {copied ? 'Copied!' : 'Copy to clipboard'}
          </button>
        </div>
      )}
    </div>
  );
}

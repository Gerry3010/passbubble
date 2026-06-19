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
import browser from 'webextension-polyfill';
import { MessageType } from '../../shared/constants.js';
import type { GenerateResponse } from '@passbubble/shared-ts';
import { term, buttonPrimary, buttonGhost, withDisabled } from '../../shared/theme.js';

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

  const strengthColor = strength >= 80 ? term.green : strength >= 50 ? term.amber : term.red;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      <h3 style={{ fontSize: '14px', fontWeight: 700, margin: 0, color: term.green }}># password generator</h3>
      <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
        <label style={{ fontSize: '12px', color: term.muted }}>Length: {length}</label>
        <input
          type="range"
          min={8}
          max={64}
          value={length}
          onChange={(e) => setLength(Number(e.target.value))}
          style={{ flex: 1 }}
        />
      </div>
      <label style={{ display: 'flex', gap: '6px', alignItems: 'center', fontSize: '12px', color: term.muted }}>
        <input type="checkbox" checked={symbols} onChange={(e) => setSymbols(e.target.checked)} />
        Include symbols
      </label>
      <button
        onClick={generate}
        disabled={isLoading}
        style={withDisabled(buttonPrimary, isLoading)}
      >
        {isLoading ? 'Generating…' : 'Generate'}
      </button>
      {generated && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
          <div
            style={{
              fontFamily: term.font,
              fontSize: '12px',
              padding: '8px',
              background: term.bg,
              border: `1px solid ${term.border}`,
              borderRadius: '2px',
              wordBreak: 'break-all',
              color: term.green,
            }}
          >
            {generated}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <div
              style={{
                flex: 1,
                height: '4px',
                background: term.border,
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
            <span style={{ fontSize: '11px', color: strengthColor, fontWeight: 700 }}>
              {strength}/100
            </span>
          </div>
          <button
            onClick={copy}
            style={copied ? { ...buttonPrimary, padding: '6px', fontSize: '12px' } : { ...buttonGhost, padding: '6px', fontSize: '12px' }}
          >
            {copied ? 'Copied!' : 'Copy to clipboard'}
          </button>
        </div>
      )}
    </div>
  );
}

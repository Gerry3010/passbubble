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

import { useState, useEffect } from 'react';
import type { EntryResponse } from '@passbubble/shared-ts';

const EXTENSION_ORIGIN = (() => {
  // Derive the extension origin for postMessage validation
  try {
    return new URL(document.location.href).origin;
  } catch {
    return document.location.origin;
  }
})();

export function FillSuggestion() {
  const [matches, setMatches] = useState<EntryResponse[]>([]);

  useEffect(() => {
    function handleMessage(event: MessageEvent) {
      // Only accept messages from our own extension origin
      if (event.origin !== EXTENSION_ORIGIN) return;
      const msg = event.data as { type: string; matches?: EntryResponse[] };
      if (msg.type === 'FILL_MATCHES' && Array.isArray(msg.matches)) {
        setMatches(msg.matches);
      }
    }
    window.addEventListener('message', handleMessage);
    return () => window.removeEventListener('message', handleMessage);
  }, []);

  function select(entryId: string) {
    window.parent.postMessage({ type: 'FILL_SELECTED', entryId }, '*');
  }

  function dismiss() {
    window.parent.postMessage({ type: 'FILL_DISMISS' }, '*');
  }

  return (
    <div style={{ padding: '8px', background: '#fff', borderRadius: '8px', border: '1px solid #e2e8f0' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
        <span style={{ fontWeight: 600, color: '#1a202c', fontSize: '12px' }}>
          🔐 Passbubble
        </span>
        <button
          onClick={dismiss}
          style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#718096', fontSize: '16px', lineHeight: 1 }}
        >
          ×
        </button>
      </div>
      {matches.length === 0 ? (
        <p style={{ color: '#718096', fontSize: '12px' }}>No matching entries</p>
      ) : (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0 }}>
          {matches.map((m) => (
            <li key={m.id}>
              <button
                onClick={() => select(m.id)}
                style={{
                  width: '100%',
                  textAlign: 'left',
                  padding: '6px 8px',
                  border: 'none',
                  borderRadius: '4px',
                  background: 'none',
                  cursor: 'pointer',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '2px',
                }}
                onMouseEnter={(e) => ((e.currentTarget as HTMLButtonElement).style.background = '#f7fafc')}
                onMouseLeave={(e) => ((e.currentTarget as HTMLButtonElement).style.background = 'none')}
              >
                <span style={{ fontWeight: 500, color: '#2d3748' }}>{m.name}</span>
                {m.url && (
                  <span style={{ color: '#718096', fontSize: '11px' }}>
                    {new URL(m.url.startsWith('http') ? m.url : `https://${m.url}`).hostname}
                  </span>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

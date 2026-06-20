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

import { useState, useEffect, useLayoutEffect, useRef } from 'react';
import type { EntryResponse } from '@passbubble/shared-ts';
import { term } from '../shared/theme.js';

export function FillSuggestion() {
  const [matches, setMatches] = useState<EntryResponse[]>([]);
  const rootRef = useRef<HTMLDivElement>(null);

  // Tell the embedding content script our real content height so it can size the
  // iframe exactly — otherwise the fixed-height iframe shows an empty area below.
  useLayoutEffect(() => {
    const h = rootRef.current?.getBoundingClientRect().height ?? 0;
    if (h > 0) window.parent.postMessage({ type: 'FILL_RESIZE', height: h }, '*');
  }, [matches]);

  useEffect(() => {
    function handleMessage(event: MessageEvent) {
      // Accept config only from our embedder (the content script). The content
      // script runs in the host page's context, so its postMessage carries the
      // page origin, not the extension origin — validate by source (the parent
      // frame) instead. The match list is non-secret metadata; actual secrets
      // are only ever fetched from the background, which gates on the session.
      if (event.source !== window.parent) return;
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
    <div
      ref={rootRef}
      style={{
        padding: '8px',
        background: term.bg,
        borderRadius: '6px',
        border: `1px solid ${term.green}`,
        boxShadow: `0 0 0 1px ${term.border}`,
        fontFamily: term.font,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
        <span style={{ fontWeight: 700, color: term.green, fontSize: '12px' }}>
          <span style={{ color: term.muted }}>passbubble:~$</span> fill
        </span>
        <button
          onClick={dismiss}
          aria-label="Dismiss"
          style={{ background: 'none', border: 'none', cursor: 'pointer', color: term.muted, fontSize: '16px', lineHeight: 1, fontFamily: term.font }}
        >
          ×
        </button>
      </div>
      {matches.length === 0 ? (
        <p style={{ color: term.muted, fontSize: '12px', margin: 0 }}>No matching entries</p>
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
                  border: `1px solid ${term.border}`,
                  borderRadius: '4px',
                  background: term.surface,
                  color: term.green,
                  cursor: 'pointer',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '2px',
                  fontFamily: term.font,
                }}
                onMouseEnter={(e) => ((e.currentTarget as HTMLButtonElement).style.borderColor = term.green)}
                onMouseLeave={(e) => ((e.currentTarget as HTMLButtonElement).style.borderColor = term.border)}
              >
                <span style={{ fontWeight: 700, color: term.green, overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>
                  {m.name}
                </span>
                {m.url && (
                  <span style={{ color: term.muted, fontSize: '11px', overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>
                    {(() => {
                      try {
                        return new URL(m.url.startsWith('http') ? m.url : `https://${m.url}`).hostname;
                      } catch {
                        return m.url;
                      }
                    })()}
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

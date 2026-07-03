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

// The padlock glyph from the Passbubble brand icon (assets/svg/icon-extension.svg),
// drawn in currentColor so it adopts the button's text colour. viewBox frames just
// the padlock from the 256×256 icon.
function LockGlyph() {
  return (
    <svg width="11" height="15" viewBox="98 86 60 84" fill="none" aria-hidden="true" style={{ flexShrink: 0 }}>
      <path d="M 114,123 L 114,105 A 14,14 0 0 1 142,105 L 142,123" fill="none" stroke="currentColor" strokeWidth={5} strokeLinecap="round" />
      <rect x="102" y="120" width="52" height="46" rx="8" fill="none" stroke="currentColor" strokeWidth={4} />
      <circle cx="128" cy="141" r="6" fill="currentColor" />
      <rect x="125.5" y="141" width="5" height="13" fill="currentColor" />
    </svg>
  );
}

interface TotpInfo {
  code: string;
  remainingSeconds: number;
  entryName?: string;
}

interface TypedInfo {
  entryType: string;
  items: { id: string; name: string; hint: string }[];
}

export function FillSuggestion() {
  const [matches, setMatches] = useState<EntryResponse[]>([]);
  const [generatePassword, setGeneratePassword] = useState<string | undefined>(undefined);
  const [totp, setTotp] = useState<TotpInfo | undefined>(undefined);
  const [typed, setTyped] = useState<TypedInfo | undefined>(undefined);
  const [remaining, setRemaining] = useState(0);
  const [copiedCode, setCopiedCode] = useState<string | null>(null);
  const [locked, setLocked] = useState(false);
  const [loggedIn, setLoggedIn] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  // Tell the embedding content script our real content height so it can size the
  // iframe exactly — otherwise the fixed-height iframe shows an empty area below.
  useLayoutEffect(() => {
    const h = rootRef.current?.getBoundingClientRect().height ?? 0;
    if (h > 0) window.parent.postMessage({ type: 'FILL_RESIZE', height: h }, '*');
  }, [matches, generatePassword, totp, typed, copiedCode, locked]);

  // TOTP countdown. When it runs out, ask the embedder once for the next code
  // (FILL_TOTP_UPDATE arrives if the vault can still produce one).
  useEffect(() => {
    if (!totp) return;
    const iv = setInterval(() => {
      setRemaining((r) => {
        if (r === 1) window.parent.postMessage({ type: 'FILL_TOTP_REFRESH' }, '*');
        return Math.max(0, r - 1);
      });
    }, 1000);
    return () => clearInterval(iv);
  }, [totp]);

  useEffect(() => {
    function handleMessage(event: MessageEvent) {
      // Accept config only from our embedder (the content script). The content
      // script runs in the host page's context, so its postMessage carries the
      // page origin, not the extension origin — validate by source (the parent
      // frame) instead. The match list is non-secret metadata; actual secrets
      // are only ever fetched from the background, which gates on the session.
      if (event.source !== window.parent) return;
      const msg = event.data as {
        type: string;
        matches?: EntryResponse[];
        generatePassword?: string;
        totp?: TotpInfo;
        typed?: TypedInfo;
        code?: string;
        locked?: boolean;
        loggedIn?: boolean;
      };
      if (msg.type === 'FILL_INIT') {
        setMatches(Array.isArray(msg.matches) ? msg.matches : []);
        setGeneratePassword(msg.generatePassword);
        setTotp(msg.totp);
        setTyped(msg.typed);
        setRemaining(msg.totp?.remainingSeconds ?? 0);
        setLocked(!!msg.locked);
        setLoggedIn(!!msg.loggedIn);
      } else if (msg.type === 'FILL_TOTP_UPDATE' && msg.totp?.code) {
        setTotp(msg.totp);
        setRemaining(msg.totp.remainingSeconds ?? 0);
      } else if (msg.type === 'FILL_TOTP_COPY' && typeof msg.code === 'string') {
        // A login fill resolved an entry that also has a 2FA secret: copy the
        // current code (extension-origin document + fresh user gesture) and show
        // a short confirmation before dismissing ourselves.
        navigator.clipboard?.writeText(msg.code).catch(() => {});
        setCopiedCode(msg.code);
        setTimeout(() => window.parent.postMessage({ type: 'FILL_DISMISS' }, '*'), 1800);
      }
    }
    window.addEventListener('message', handleMessage);
    // Tell the embedder we're mounted and listening, so it (re-)sends the config.
    // Posting only on iframe 'load' races React mount — on a cached re-injection
    // 'load' fires before this listener exists and the config is lost.
    window.parent.postMessage({ type: 'FILL_READY' }, '*');
    return () => window.removeEventListener('message', handleMessage);
  }, []);

  function select(entryId: string) {
    window.parent.postMessage({ type: 'FILL_SELECTED', entryId }, '*');
  }

  function useGenerated() {
    if (generatePassword) window.parent.postMessage({ type: 'FILL_USE_GENERATED', password: generatePassword }, '*');
  }

  function unlock() {
    window.parent.postMessage({ type: 'FILL_UNLOCK' }, '*');
  }

  function dismiss() {
    window.parent.postMessage({ type: 'FILL_DISMISS' }, '*');
  }

  function useTotp() {
    if (!totp) return;
    navigator.clipboard?.writeText(totp.code).catch(() => {});
    window.parent.postMessage({ type: 'FILL_TOTP_SELECTED', code: totp.code }, '*');
  }

  function selectTyped(entryId: string) {
    window.parent.postMessage({ type: 'FILL_TYPED_SELECTED', entryId }, '*');
  }

  const isGenerate = typeof generatePassword === 'string';
  const title = copiedCode
    ? '2fa'
    : locked
      ? 'locked'
      : isGenerate
        ? 'generate'
        : totp
          ? '2fa'
          : typed
            ? typed.entryType === 'credit-card'
              ? 'card'
              : 'identity'
            : 'fill';

  return (
    <div
      ref={rootRef}
      onMouseDown={(e) => {
        // A press on the card's dead space (not a button/list item) dismisses.
        // Clicks inside the iframe never reach the host page, so without this a
        // press that was meant for a covered page element would be swallowed.
        if (e.target === e.currentTarget) dismiss();
      }}
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
          <span style={{ color: term.muted }}>passbubble:~$</span> {title}
        </span>
        <button
          onClick={dismiss}
          aria-label="Dismiss"
          style={{ background: 'none', border: 'none', cursor: 'pointer', color: term.muted, fontSize: '16px', lineHeight: 1, fontFamily: term.font }}
        >
          ×
        </button>
      </div>
      {copiedCode ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
          <span style={{ color: term.green, fontSize: '13px', fontWeight: 700 }}>✓ 2FA code copied</span>
          <span style={{ color: term.muted, fontSize: '12px' }}>
            {copiedCode.replace(/^(\d{3})(\d+)$/, '$1 $2')} — paste it into the verification field.
          </span>
        </div>
      ) : locked ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          <span style={{ color: term.muted, fontSize: '12px' }}>
            {loggedIn ? 'Vault locked' : 'Not signed in'} — unlock to fill your logins.
          </span>
          <button
            onClick={unlock}
            style={{
              background: term.green,
              color: term.bg,
              border: `1px solid ${term.green}`,
              borderRadius: '4px',
              padding: '6px 12px',
              fontSize: '12px',
              fontWeight: 700,
              cursor: 'pointer',
              fontFamily: term.font,
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: '6px',
            }}
          >
            {loggedIn ? (
              <>
                <LockGlyph /> Unlock Passbubble
              </>
            ) : (
              'Sign in to Passbubble'
            )}
          </button>
        </div>
      ) : isGenerate ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          <div
            style={{
              border: `1px solid ${term.border}`,
              background: term.surface,
              borderRadius: '4px',
              padding: '6px 8px',
              color: term.green,
              fontSize: '13px',
              wordBreak: 'break-all',
            }}
          >
            {generatePassword}
          </div>
          <button
            onClick={useGenerated}
            style={{
              background: term.green,
              color: term.bg,
              border: `1px solid ${term.green}`,
              borderRadius: '4px',
              padding: '6px 12px',
              fontSize: '12px',
              fontWeight: 700,
              cursor: 'pointer',
              fontFamily: term.font,
            }}
          >
            Use &amp; save
          </button>
          <span style={{ color: term.muted, fontSize: '11px' }}>Fills the password and saves a new entry.</span>
        </div>
      ) : totp ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          {totp.entryName && (
            <span style={{ color: term.muted, fontSize: '11px', overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>
              {totp.entryName}
            </span>
          )}
          <button
            onClick={useTotp}
            title="Fill this code"
            style={{
              width: '100%',
              textAlign: 'left',
              padding: '6px 8px',
              border: `1px solid ${term.border}`,
              borderRadius: '4px',
              background: term.surface,
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: '8px',
              fontFamily: term.font,
            }}
            onMouseEnter={(e) => ((e.currentTarget as HTMLButtonElement).style.borderColor = term.green)}
            onMouseLeave={(e) => ((e.currentTarget as HTMLButtonElement).style.borderColor = term.border)}
          >
            <span style={{ color: term.green, fontSize: '18px', fontWeight: 700, letterSpacing: '2px' }}>
              {totp.code.replace(/^(\d{3})(\d+)$/, '$1 $2')}
            </span>
            <span
              style={{
                color: remaining <= 5 ? term.amber : term.muted,
                fontSize: '11px',
                whiteSpace: 'nowrap',
              }}
            >
              {remaining}s
            </span>
          </button>
          <span style={{ color: term.muted, fontSize: '11px' }}>Click to fill &amp; copy the 2FA code.</span>
        </div>
      ) : typed ? (
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: '4px' }}>
          {typed.items.map((it) => (
            <li key={it.id}>
              <button
                onClick={() => selectTyped(it.id)}
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
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: '8px',
                  fontFamily: term.font,
                }}
                onMouseEnter={(e) => ((e.currentTarget as HTMLButtonElement).style.borderColor = term.green)}
                onMouseLeave={(e) => ((e.currentTarget as HTMLButtonElement).style.borderColor = term.border)}
              >
                <span style={{ fontWeight: 700, overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>
                  {typed.entryType === 'credit-card' ? '💳 ' : '👤 '}
                  {it.name}
                </span>
                {it.hint && (
                  <span style={{ color: term.muted, fontSize: '11px', whiteSpace: 'nowrap' }}>{it.hint}</span>
                )}
              </button>
            </li>
          ))}
        </ul>
      ) : matches.length === 0 ? (
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

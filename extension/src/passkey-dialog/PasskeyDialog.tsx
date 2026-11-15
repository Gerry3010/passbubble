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

// The passkey dialog: an extension page the bridge shows on the requesting
// page. It gets a one-time token from its parent and proves it by asking the
// background for the request's state (which binds the token to this frame).
// Decisions go straight to the background; the parent only ever hears
// "ready", "resize" and "close" from here.
//
// Clickjacking: the buttons that act stay disabled until the frame has been
// visible and unchanged for ARM_DELAY_MS (restarted by a resize or a
// visibility change; in Chrome also by IntersectionObserver v2), and Enter
// or Space do nothing in that time. Cancel always works.

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import browser from 'webextension-polyfill';
import { MessageType, STORAGE_KEYS } from '../shared/constants.js';
import { term, link, withDisabled } from '../shared/theme.js';
import type { DialogState, PasskeyChoice } from '../background/passkey-service.js';

export const ARM_DELAY_MS = 500;
/** IntersectionObserver v2 that has not reported at all by then is treated as
 * missing (seen in Chromium when the frame's rendering stalls), so the dialog
 * does not stay disabled for good. Once it reports, its answer counts. */
export const IO_SILENCE_MS = 2000;
export const IO_REMEASURE_MS = 1000;
const TOKEN_WAIT_MS = 5000;
const TOKEN_PATTERN = /^[A-Za-z0-9_-]{16,64}$/;

const BIDI_CONTROLS = /[\u061C\u200E\u200F\u202A-\u202E\u2066-\u2069]/g;

/** Text from the site, safe to show: no bidi overrides, bounded length. */
export function displayText(value: string | undefined, max = 64): string {
  const clean = (value ?? '').replace(BIDI_CONTROLS, '').replace(/[\u0000-\u001f\u007f]/g, ' ').trim();
  return clean.length > max ? `${clean.slice(0, max - 1)}…` : clean;
}

function postToParent(action: 'ready' | 'resize' | 'close', extra: Record<string, unknown> = {}): void {
  window.parent.postMessage({ type: 'passkey-dialog', action, ...extra }, '*');
}

/** True once the frame was visible and unchanged for `delay` ms. */
export function useArmed(delay = ARM_DELAY_MS): boolean {
  const [armed, setArmed] = useState(false);
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    let visible = document.visibilityState !== 'hidden';
    let observerSaysVisible = true;
    const restart = () => {
      setArmed(false);
      clearTimeout(timer);
      if (visible && observerSaysVisible) timer = setTimeout(() => setArmed(true), delay);
    };
    const onVisibility = () => {
      visible = document.visibilityState !== 'hidden';
      restart();
    };
    let observer: IntersectionObserver | undefined;
    let silence: ReturnType<typeof setTimeout> | undefined;
    let remeasure: ReturnType<typeof setInterval> | undefined;
    let lastReportAt = Date.now();
    if (typeof IntersectionObserver !== 'undefined') {
      try {
        const v2 = new IntersectionObserver(
          (entries) => {
            clearTimeout(silence);
            lastReportAt = Date.now();
            for (const entry of entries) {
              const isVisible = (entry as IntersectionObserverEntry & { isVisible?: boolean }).isVisible;
              if (typeof isVisible === 'boolean' && isVisible !== observerSaysVisible) {
                observerSaysVisible = isVisible;
                restart();
              }
            }
          },
          { threshold: [1], trackVisibility: true, delay: 100 } as IntersectionObserverInit,
        );
        // Only Chrome implements v2; elsewhere the options are ignored.
        if ((v2 as IntersectionObserver & { trackVisibility?: boolean }).trackVisibility) {
          observerSaysVisible = false;
          v2.observe(document.documentElement);
          observer = v2;
          silence = setTimeout(() => {
            observerSaysVisible = true;
            restart();
          }, IO_SILENCE_MS);
          // Chrome sometimes reports "not visible" once and then stops
          // answering at all, although nothing covers the frame. Observing
          // again asks for a fresh reading every second. A covered frame keeps
          // being reported as covered and stays blocked; a "not visible" that
          // Chrome no longer confirms counts as silence, like no report at all.
          remeasure = setInterval(() => {
            v2.unobserve(document.documentElement);
            v2.observe(document.documentElement);
            if (!observerSaysVisible && Date.now() - lastReportAt >= IO_SILENCE_MS) {
              observerSaysVisible = true;
              restart();
            }
          }, IO_REMEASURE_MS);
        } else {
          v2.disconnect();
        }
      } catch {
        // no IntersectionObserver v2
      }
    }
    restart();
    window.addEventListener('resize', restart);
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      clearTimeout(timer);
      clearTimeout(silence);
      clearInterval(remeasure);
      observer?.disconnect();
      window.removeEventListener('resize', restart);
      document.removeEventListener('visibilitychange', onVisibility);
    };
  }, [delay]);

  // Enter / Space must not trigger anything before the frame is armed.
  useEffect(() => {
    if (armed) return;
    const swallow = (e: KeyboardEvent) => {
      if (e.key === 'Enter' || e.key === ' ' || e.key === 'Spacebar') {
        e.preventDefault();
        e.stopPropagation();
      }
    };
    document.addEventListener('keydown', swallow, true);
    return () => document.removeEventListener('keydown', swallow, true);
  }, [armed]);
  return armed;
}

const primary: CSSProperties = {
  background: term.green,
  color: term.bg,
  border: `1px solid ${term.green}`,
  borderRadius: '4px',
  padding: '6px 12px',
  fontSize: '12px',
  fontWeight: 700,
  cursor: 'pointer',
  fontFamily: term.font,
};

const ghost: CSSProperties = {
  background: 'transparent',
  color: term.green,
  border: `1px solid ${term.border}`,
  borderRadius: '4px',
  padding: '6px 12px',
  fontSize: '12px',
  cursor: 'pointer',
  fontFamily: term.font,
};

const rowButton: CSSProperties = {
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
};

const mutedText: CSSProperties = { color: term.muted, fontSize: '11px', margin: 0 };
const ellipsis: CSSProperties = { overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' };

type View =
  | { kind: 'loading' }
  | { kind: 'state'; state: DialogState }
  /** `fallback`: the request is still open, so another device can take it. */
  | { kind: 'error'; message: string; fallback?: boolean }
  | { kind: 'done'; message: string };

export function PasskeyDialog() {
  const [view, setView] = useState<View>({ kind: 'loading' });
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [target, setTarget] = useState<string>('new');
  const tokenRef = useRef<string | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const armed = useArmed();

  const loadState = useCallback(async (token: string): Promise<boolean> => {
    let resp: { state?: DialogState; error?: string; tokenValid?: boolean } | null;
    try {
      resp = (await browser.runtime.sendMessage({ type: MessageType.PASSKEY_UI_STATE, payload: { token } })) as typeof resp;
    } catch {
      resp = null;
    }
    if (!resp?.state) {
      if (resp?.tokenValid) tokenRef.current = token;
      if (tokenRef.current === token) {
        setView({ kind: 'error', message: resp?.error ?? 'This passkey request is no longer open.', fallback: !!resp?.tokenValid });
      }
      return false;
    }
    tokenRef.current = token;
    const state = resp.state;
    if (state.mode === 'create') {
      setTarget((prev) => {
        const usable = state.targets.filter((t) => !state.readOnlyTargets.includes(t.entryId));
        if (prev !== 'new' && usable.some((t) => t.entryId === prev)) return prev;
        return usable[0]?.entryId ?? 'new';
      });
    }
    setView({ kind: 'state', state });
    return true;
  }, []);

  // The token comes from the parent (the bridge). Anything else on the page
  // can post here too, so a token counts only once the background accepts it.
  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      if (event.source !== window.parent || tokenRef.current) return;
      const token = event.data;
      if (typeof token !== 'string' || !TOKEN_PATTERN.test(token)) return;
      void loadState(token);
    };
    window.addEventListener('message', onMessage);
    postToParent('ready');
    const timer = setTimeout(() => {
      if (!tokenRef.current) setView({ kind: 'error', message: 'This passkey request is no longer open.' });
    }, TOKEN_WAIT_MS);
    return () => {
      window.removeEventListener('message', onMessage);
      clearTimeout(timer);
    };
  }, [loadState]);

  // Unlocking (or locking) in the toolbar popup changes what to show.
  useEffect(() => {
    const onChanged = (changes: Record<string, unknown>, area: string) => {
      if (area !== 'session' || !(STORAGE_KEYS.UNLOCKED_SESSION in changes)) return;
      if (tokenRef.current) void loadState(tokenRef.current);
    };
    browser.storage.onChanged.addListener(onChanged);
    return () => browser.storage.onChanged.removeListener(onChanged);
  }, [loadState]);

  // Tell the host our height.
  const reportHeight = () => {
    const h = rootRef.current?.getBoundingClientRect().height ?? 0;
    if (h > 0) postToParent('resize', { height: h });
  };
  useLayoutEffect(reportHeight, [view, notice, busy]);
  useEffect(() => {
    const el = rootRef.current;
    if (!el || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(reportHeight);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  async function decide(choice: PasskeyChoice): Promise<void> {
    const token = tokenRef.current;
    if (!token || busy) return;
    setBusy(true);
    setNotice(null);
    let resp: { ok?: boolean; error?: string; readOnly?: boolean; locked?: boolean } | null;
    try {
      resp = (await browser.runtime.sendMessage({ type: MessageType.PASSKEY_COMPLETE, payload: { token, choice } })) as typeof resp;
    } catch (err) {
      resp = { ok: false, error: err instanceof Error ? err.message : String(err) };
    }
    setBusy(false);
    if (resp?.ok) {
      setView({
        kind: 'done',
        message: choice.action === 'create' ? 'Passkey saved.' : choice.action === 'select' ? 'Signing in…' : 'Done.',
      });
      return;
    }
    if (resp?.locked) {
      await loadState(token);
      return;
    }
    if (resp?.readOnly) {
      setNotice(resp.error ?? 'You only have read access to this entry.');
      await loadState(token);
      setTarget('new');
      return;
    }
    setView({ kind: 'error', message: resp?.error ?? 'Something went wrong.' });
  }

  async function unlock(): Promise<void> {
    // Straight from the click: openPopup needs the user's gesture.
    try {
      const action = browser.action as { openPopup?: () => Promise<void> };
      if (!action.openPopup) throw new Error('no openPopup');
      await action.openPopup();
    } catch {
      try {
        await browser.windows.create({ url: browser.runtime.getURL('popup/index.html'), type: 'popup', width: 380, height: 600 });
      } catch {
        setNotice('Click the Passbubble icon in the toolbar to unlock.');
      }
    }
  }

  const state = view.kind === 'state' ? view.state : null;
  const host = state ? displayText(state.host, 80) : '';
  const title =
    view.kind === 'error'
      ? 'passkey'
      : state?.mode === 'locked'
        ? 'locked'
        : state?.type === 'create'
          ? 'save passkey'
          : 'sign in';

  const cancelButton = (
    <button type="button" style={ghost} disabled={busy} onClick={() => void decide({ action: 'cancel' })}>
      Cancel
    </button>
  );
  const otherDevice = (
    <button type="button" style={{ ...link, fontSize: '11px', color: term.muted }} disabled={busy} onClick={() => void decide({ action: 'native' })}>
      Use another device
    </button>
  );

  let body: ReactNode;
  if (view.kind === 'loading') {
    body = <p style={mutedText}>Loading…</p>;
  } else if (view.kind === 'error') {
    body = (
      <>
        <p role="alert" style={{ color: term.red, fontSize: '12px', margin: 0 }}>
          {view.message}
        </p>
        <div style={{ display: 'flex', justifyContent: view.fallback ? 'space-between' : 'flex-end', alignItems: 'center' }}>
          {view.fallback && otherDevice}
          <button type="button" style={ghost} onClick={() => postToParent('close')}>
            Close
          </button>
        </div>
      </>
    );
  } else if (view.kind === 'done') {
    body = <p style={{ color: term.green, fontSize: '12px', margin: 0 }}>✓ {view.message}</p>;
  } else if (state) {
    const rpNote =
      state.rpId !== state.host ? (
        <p style={mutedText}>
          Passkey for <b style={{ color: term.text }}>{displayText(state.rpId, 80)}</b>
        </p>
      ) : null;
    if (state.mode === 'locked') {
      body = (
        <>
          <p style={{ color: term.green, fontSize: '12px', margin: 0 }}>
            {state.loggedIn ? 'Passbubble is locked.' : 'You are not signed in to Passbubble.'}
          </p>
          <p style={mutedText}>
            {state.type === 'create' ? 'Unlock to save a passkey for this site.' : 'Unlock to sign in with a passkey.'} This box
            continues once the vault is unlocked.
          </p>
          <div style={{ display: 'flex', gap: '6px', justifyContent: 'flex-end' }}>
            {cancelButton}
            <button type="button" style={withDisabled(primary, !armed)} disabled={!armed} onClick={() => void unlock()}>
              {state.loggedIn ? 'Unlock' : 'Sign in'}
            </button>
          </div>
          {otherDevice}
        </>
      );
    } else if (state.mode === 'get') {
      body = (
        <>
          {state.accounts.length === 0 ? (
            <p style={mutedText}>No passkey for this site in Passbubble.</p>
          ) : (
            <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: '4px' }}>
              {state.accounts.map((a) => (
                <li key={a.credentialId}>
                  <button
                    type="button"
                    style={withDisabled(rowButton, !armed || busy)}
                    disabled={!armed || busy}
                    onClick={() => void decide({ action: 'select', credentialId: a.credentialId })}
                  >
                    <span style={{ fontWeight: 700, ...ellipsis }}>{displayText(a.userName) || '(no user name)'}</span>
                    <span style={{ ...mutedText, ...ellipsis }}>{displayText(a.entryName)}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            {otherDevice}
            {cancelButton}
          </div>
        </>
      );
    } else if (state.mode === 'exists') {
      body = (
        <>
          <p style={{ color: term.green, fontSize: '12px', margin: 0 }}>This passkey is already saved in Passbubble.</p>
          <div style={{ display: 'flex', gap: '6px', justifyContent: 'flex-end' }}>
            {cancelButton}
            <button
              type="button"
              style={withDisabled(primary, !armed || busy)}
              disabled={!armed || busy}
              onClick={() => void decide({ action: 'exists-ok' })}
            >
              OK
            </button>
          </div>
          {otherDevice}
        </>
      );
    } else {
      const usable = state.targets.filter((t) => !state.readOnlyTargets.includes(t.entryId));
      body = (
        <>
          <p style={{ color: term.green, fontSize: '12px', margin: 0, ...ellipsis }}>
            Save a passkey for <b>{displayText(state.userName) || '(no user name)'}</b>
          </p>
          <label style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
            <span style={mutedText}>Save in</span>
            <select
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              disabled={busy}
              style={{
                padding: '6px',
                background: term.bg,
                color: term.green,
                border: `1px solid ${term.border}`,
                borderRadius: '4px',
                fontFamily: term.font,
                fontSize: '12px',
              }}
            >
              {usable.map((t) => (
                <option key={t.entryId} value={t.entryId}>
                  {displayText(t.entryName)} ({displayText(t.userName, 40)})
                </option>
              ))}
              <option value="new">New entry</option>
            </select>
          </label>
          <div style={{ display: 'flex', gap: '6px', justifyContent: 'flex-end' }}>
            {cancelButton}
            <button
              type="button"
              style={withDisabled(primary, !armed || busy)}
              disabled={!armed || busy}
              onClick={() => void decide({ action: 'create', target })}
            >
              {busy ? 'Saving…' : 'Save'}
            </button>
          </div>
          {otherDevice}
        </>
      );
    }
    body = (
      <>
        {rpNote}
        {body}
      </>
    );
  }

  return (
    <div
      ref={rootRef}
      style={{
        padding: '10px',
        background: term.bg,
        borderRadius: '6px',
        border: `1px solid ${term.green}`,
        boxShadow: `0 0 0 1px ${term.border}`,
        fontFamily: term.font,
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '8px' }}>
        <span style={{ fontWeight: 700, color: term.green, fontSize: '12px', ...ellipsis }}>
          <span style={{ color: term.muted }}>passbubble:~$</span> {title}
        </span>
      </div>
      {host && (
        <p style={{ color: term.text, fontSize: '13px', fontWeight: 700, margin: 0, ...ellipsis }} title={host}>
          {host}
        </p>
      )}
      {notice && (
        <p role="status" style={{ color: term.amber, fontSize: '11px', margin: 0 }}>
          {notice}
        </p>
      )}
      {body}
    </div>
  );
}

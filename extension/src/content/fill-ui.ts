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

// Injects the fill-suggestion iframe next to a detected password field.
// Uses an <iframe> (not shadow DOM) for a hard CSP boundary — the host page cannot
// read the iframe's DOM or intercept credentials.

import browser from 'webextension-polyfill';
import type { EntryResponse } from '@passbubble/shared-ts';
import { MessageType } from '../shared/constants.js';

let activeIframe: HTMLIFrameElement | null = null;
let activeAnchor: HTMLInputElement | null = null;
let activeCleanup: (() => void) | null = null;

// True while a fill suggestion is currently shown for `anchor` (or for any
// field, if no anchor is given). Used to avoid re-injecting on every DOM
// mutation — injecting the iframe is itself a mutation, which would otherwise
// loop and make the box flash.
export function isFillIframeShown(anchor?: HTMLInputElement): boolean {
  if (!activeIframe || !document.body.contains(activeIframe)) return false;
  return anchor ? activeAnchor === anchor : true;
}

export interface TotpSuggestion {
  code: string;
  remainingSeconds: number;
  entryName?: string;
}

export interface FillPayload {
  // Match mode: existing entries to offer for a login form.
  matches?: EntryResponse[];
  // Generate mode: a fresh password to offer on a signup/register form.
  generatePassword?: string;
  // TOTP mode: the current one-time code to offer on a 2FA field.
  totp?: TotpSuggestion;
  // Locked mode: the vault is locked / logged out, so offer an "unlock" button
  // (which opens the toolbar popup) instead of suggestions.
  locked?: boolean;
  // In locked mode, whether the user is logged in (→ "unlock") or not (→ "sign in").
  loggedIn?: boolean;
}

export interface FillHandlers {
  // A matching entry was chosen (resolved to credentials via the background).
  onFillMatch: (username: string, password: string) => void;
  // The generated password was accepted on a signup form.
  onUseGenerated: (password: string) => void;
  // The offered TOTP code was accepted on a 2FA field.
  onFillTotp?: (code: string) => void;
  onDismiss: () => void;
}

export function injectFillIframe(
  anchorField: HTMLInputElement,
  payload: FillPayload,
  handlers: FillHandlers,
): void {
  removeFillIframe();

  const iframe = document.createElement('iframe');
  iframe.src = browser.runtime.getURL('fill-iframe/index.html');
  iframe.style.cssText = [
    'position: fixed',
    'z-index: 2147483647',
    'border: none',
    'width: 320px',
    'height: 56px', // initial; the iframe reports its real content height (FILL_RESIZE)
    'background: transparent',
  ].join(';');
  positionIframe(iframe, anchorField);
  document.body.appendChild(iframe);
  activeIframe = iframe;
  activeAnchor = anchorField;

  // Keep the fixed-position iframe glued to its anchor while the page scrolls
  // or resizes (capture-phase scroll also catches scrolling sub-containers).
  const reposition = () => positionIframe(iframe, anchorField);
  window.addEventListener('scroll', reposition, { capture: true, passive: true });
  window.addEventListener('resize', reposition, { passive: true });
  activeCleanup = () => {
    window.removeEventListener('scroll', reposition, { capture: true });
    window.removeEventListener('resize', reposition);
  };

  const postInit = () =>
    iframe.contentWindow?.postMessage(
      {
        type: 'FILL_INIT',
        matches: payload.matches ?? [],
        generatePassword: payload.generatePassword,
        totp: payload.totp,
        locked: payload.locked ?? false,
        loggedIn: payload.loggedIn ?? false,
      },
      browser.runtime.getURL(''),
    );
  // Post on load AND on the iframe's FILL_READY handshake (below) to avoid a
  // race where the React app mounts after 'load' fires.
  iframe.addEventListener('load', postInit);

  window.addEventListener('message', function handler(event) {
    // Only accept messages from our extension
    if (event.origin !== new URL(browser.runtime.getURL('')).origin) return;
    const msg = event.data as { type: string; entryId?: string; height?: number; password?: string; code?: string };
    if (msg.type === 'FILL_READY') {
      postInit();
      return;
    }
    if (msg.type === 'FILL_RESIZE' && typeof msg.height === 'number') {
      // Fit the iframe to its content so there's no empty (white) area below it,
      // then re-check the position — only now is the real height known, so the
      // collision check (covering a login button → flip above) is accurate.
      iframe.style.height = `${Math.max(1, Math.ceil(msg.height))}px`;
      positionIframe(iframe, anchorField);
      return;
    }
    if (msg.type === 'FILL_TOTP_REFRESH') {
      // The countdown in the iframe ran out — fetch the next code and re-init.
      browser.runtime
        .sendMessage({ type: MessageType.GET_TOTP_FOR_URL, payload: { url: location.href } })
        .then((r: unknown) => {
          const resp = r as { code?: string; remainingSeconds?: number; entryName?: string } | null;
          if (!resp?.code) return;
          iframe.contentWindow?.postMessage(
            { type: 'FILL_TOTP_UPDATE', totp: resp },
            browser.runtime.getURL(''),
          );
        })
        .catch(() => {});
      return;
    }
    if (msg.type === 'FILL_SELECTED' && msg.entryId) {
      browser.runtime
        .sendMessage({ type: 'FILL_ENTRY', payload: { entryId: msg.entryId } })
        .then((r: unknown) => {
          const resp = (r ?? {}) as { username?: string; password?: string; locked?: boolean; totpCode?: string };
          if (resp.locked) return;
          handlers.onFillMatch(resp.username ?? '', resp.password ?? '');
          if (resp.totpCode) {
            // The entry has a 2FA secret: let the iframe copy the current code
            // to the clipboard (extension-origin document, real user gesture)
            // and show a short "code copied" flash; it dismisses itself after.
            iframe.contentWindow?.postMessage(
              { type: 'FILL_TOTP_COPY', code: resp.totpCode },
              browser.runtime.getURL(''),
            );
            return; // keep the handler alive for the iframe's FILL_DISMISS
          }
          removeFillIframe();
          window.removeEventListener('message', handler);
        });
    } else if (msg.type === 'FILL_TOTP_SELECTED' && typeof msg.code === 'string') {
      handlers.onFillTotp?.(msg.code);
      removeFillIframe();
      window.removeEventListener('message', handler);
    } else if (msg.type === 'FILL_USE_GENERATED' && typeof msg.password === 'string') {
      handlers.onUseGenerated(msg.password);
      removeFillIframe();
      window.removeEventListener('message', handler);
    } else if (msg.type === 'FILL_UNLOCK') {
      // Open the toolbar popup so the user can unlock / sign in. Best-effort —
      // the background swallows browsers that disallow programmatic open.
      browser.runtime.sendMessage({ type: MessageType.OPEN_POPUP, payload: {} }).catch(() => {});
      removeFillIframe();
      window.removeEventListener('message', handler);
    } else if (msg.type === 'FILL_DISMISS') {
      handlers.onDismiss();
      removeFillIframe();
      window.removeEventListener('message', handler);
    }
  });
}

export function removeFillIframe(): void {
  activeCleanup?.();
  activeCleanup = null;
  if (activeIframe) {
    activeIframe.remove();
    activeIframe = null;
  }
  activeAnchor = null;
}

// Place the iframe below its anchor field — unless it would cover an
// interactive element there (typically the login button right under the
// password field, which made the user's first click land on the overlay
// instead of the button); then flip it above the anchor.
function positionIframe(iframe: HTMLIFrameElement, anchor: HTMLInputElement): void {
  const rect = anchor.getBoundingClientRect();
  const own = iframe.getBoundingClientRect();
  const width = own.width || parseFloat(iframe.style.width) || 320;
  const height = own.height || parseFloat(iframe.style.height) || 56;
  const left = Math.max(4, rect.left);
  let top = rect.bottom + 4;
  if (coversInteractiveElement(left, top, width, height, iframe, anchor)) {
    const above = rect.top - height - 4;
    if (above >= 4) top = above;
  }
  iframe.style.top = `${top}px`;
  iframe.style.left = `${left}px`;
}

const INTERACTIVE_SELECTOR = 'button, a[href], input, select, textarea, [role="button"], [type="submit"]';

// Probe a few points of the intended iframe rect for interactive page elements
// underneath. elementsFromPoint returns the whole hit-test stack, so our own
// iframe (topmost once mounted) and wrapper divs don't hide a button below.
function coversInteractiveElement(
  left: number,
  top: number,
  width: number,
  height: number,
  self: HTMLIFrameElement,
  anchor: HTMLInputElement,
): boolean {
  if (typeof document.elementsFromPoint !== 'function') return false;
  const points: [number, number][] = [
    [left + width / 2, top + height / 2],
    [left + 12, top + height - 8],
    [left + width - 12, top + height - 8],
    [left + width / 2, top + 6],
  ];
  for (const [x, y] of points) {
    if (x < 0 || y < 0 || x >= window.innerWidth || y >= window.innerHeight) continue;
    for (const el of document.elementsFromPoint(x, y)) {
      if (el === self || el === anchor) continue;
      if (el.matches(INTERACTIVE_SELECTOR)) return true;
    }
  }
  return false;
}

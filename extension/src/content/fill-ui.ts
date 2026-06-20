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

let activeIframe: HTMLIFrameElement | null = null;
let activeAnchor: HTMLInputElement | null = null;

// True while a fill suggestion is currently shown for `anchor` (or for any
// field, if no anchor is given). Used to avoid re-injecting on every DOM
// mutation — injecting the iframe is itself a mutation, which would otherwise
// loop and make the box flash.
export function isFillIframeShown(anchor?: HTMLInputElement): boolean {
  if (!activeIframe || !document.body.contains(activeIframe)) return false;
  return anchor ? activeAnchor === anchor : true;
}

export interface FillPayload {
  // Match mode: existing entries to offer for a login form.
  matches?: EntryResponse[];
  // Generate mode: a fresh password to offer on a signup/register form.
  generatePassword?: string;
}

export interface FillHandlers {
  // A matching entry was chosen (resolved to credentials via the background).
  onFillMatch: (username: string, password: string) => void;
  // The generated password was accepted on a signup form.
  onUseGenerated: (password: string) => void;
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

  const postInit = () =>
    iframe.contentWindow?.postMessage(
      { type: 'FILL_INIT', matches: payload.matches ?? [], generatePassword: payload.generatePassword },
      browser.runtime.getURL(''),
    );
  // Post on load AND on the iframe's FILL_READY handshake (below) to avoid a
  // race where the React app mounts after 'load' fires.
  iframe.addEventListener('load', postInit);

  window.addEventListener('message', function handler(event) {
    // Only accept messages from our extension
    if (event.origin !== new URL(browser.runtime.getURL('')).origin) return;
    const msg = event.data as { type: string; entryId?: string; height?: number; password?: string };
    if (msg.type === 'FILL_READY') {
      postInit();
      return;
    }
    if (msg.type === 'FILL_RESIZE' && typeof msg.height === 'number') {
      // Fit the iframe to its content so there's no empty (white) area below it.
      iframe.style.height = `${Math.max(1, Math.ceil(msg.height))}px`;
      return;
    }
    if (msg.type === 'FILL_SELECTED' && msg.entryId) {
      browser.runtime
        .sendMessage({ type: 'FILL_ENTRY', payload: { entryId: msg.entryId } })
        .then((resp: { username?: string; password?: string; locked?: boolean }) => {
          if (resp.locked) return;
          handlers.onFillMatch(resp.username ?? '', resp.password ?? '');
          removeFillIframe();
        });
      window.removeEventListener('message', handler);
    } else if (msg.type === 'FILL_USE_GENERATED' && typeof msg.password === 'string') {
      handlers.onUseGenerated(msg.password);
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
  if (activeIframe) {
    activeIframe.remove();
    activeIframe = null;
  }
  activeAnchor = null;
}

function positionIframe(iframe: HTMLIFrameElement, anchor: HTMLInputElement): void {
  const rect = anchor.getBoundingClientRect();
  iframe.style.top = `${rect.bottom + 4}px`;
  iframe.style.left = `${Math.max(4, rect.left)}px`;
}

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

// Hosts the passkey dialog (an extension page) on the page, top right, in the
// top layer: a closed shadow root created from the isolated world holds the
// iframe, so the page cannot reach into it. The one-time token goes to the
// dialog with postMessage aimed at the extension origin, never in the URL.
// If the page removes or restyles the host, the request is cancelled: a
// dialog that is not shown as we placed it must not collect a decision.

import browser from 'webextension-polyfill';

export interface PasskeyDialogHandlers {
  /** The page removed or changed the host: cancel the request. */
  onGone: () => void;
  /** The dialog asked to be closed (after showing an error). */
  onClose: () => void;
}

export interface PasskeyDialogHandle {
  remove: () => void;
}

const WIDTH = 360;
const INITIAL_HEIGHT = 180;
const MAX_HEIGHT = 560;

/** Where the host goes: inside the fullscreen element, else inside an open
 * modal <dialog> (everything outside it is inert), else the document. */
function hostParent(doc: Document): Element {
  if (doc.fullscreenElement) return doc.fullscreenElement;
  for (const dialog of Array.from(doc.querySelectorAll('dialog[open]'))) {
    try {
      if (dialog.matches(':modal')) return dialog;
    } catch {
      // no :modal support: an open dialog is the best guess
      return dialog;
    }
  }
  return doc.body ?? doc.documentElement;
}

export function showPasskeyDialog(token: string, handlers: PasskeyDialogHandlers, doc: Document = document): PasskeyDialogHandle {
  const extensionUrl = browser.runtime.getURL('');
  const extensionOrigin = new URL(extensionUrl).origin;

  const host = doc.createElement('passbubble-passkey');
  host.setAttribute('popover', 'manual');
  const hostStyle = [
    'all: initial',
    'position: fixed',
    'top: 12px',
    'right: 12px',
    'left: auto',
    'bottom: auto',
    'inset: 12px 12px auto auto',
    'margin: 0',
    'padding: 0',
    'border: 0',
    'background: transparent',
    'overflow: visible',
    'display: block',
    'z-index: 2147483647',
    `width: ${WIDTH}px`,
    'max-width: calc(100vw - 24px)',
  ]
    .map((rule) => `${rule} !important`)
    .join(';');
  host.setAttribute('style', hostStyle);
  const shadow = host.attachShadow({ mode: 'closed' });
  const iframe = doc.createElement('iframe');
  iframe.src = browser.runtime.getURL('passkey-dialog/index.html');
  iframe.setAttribute('title', 'Passbubble passkey');
  iframe.style.cssText = [
    'display: block',
    'border: none',
    `width: ${WIDTH}px`,
    'max-width: 100%',
    `height: ${INITIAL_HEIGHT}px`,
    'background: transparent',
    'color-scheme: normal',
  ].join(';');
  shadow.appendChild(iframe);

  const parent = hostParent(doc);
  parent.appendChild(host);
  try {
    (host as HTMLElement & { showPopover?: () => void }).showPopover?.();
  } catch {
    // already open, or the page interfered; the observer below decides
  }

  let removed = false;
  const sendToken = () => iframe.contentWindow?.postMessage(token, extensionUrl);
  iframe.addEventListener('load', sendToken);

  function onMessage(event: MessageEvent) {
    if (!iframe.contentWindow || event.source !== iframe.contentWindow || event.origin !== extensionOrigin) return;
    const msg = event.data as { type?: unknown; action?: unknown; height?: unknown } | null;
    if (!msg || msg.type !== 'passkey-dialog') return;
    if (msg.action === 'ready') sendToken();
    else if (msg.action === 'resize' && typeof msg.height === 'number' && Number.isFinite(msg.height)) {
      iframe.style.height = `${Math.min(MAX_HEIGHT, Math.max(60, Math.ceil(msg.height)))}px`;
    } else if (msg.action === 'close') handlers.onClose();
  }
  window.addEventListener('message', onMessage);

  // Removed, moved or restyled by the page → gone.
  const observer = new MutationObserver(() => {
    if (removed) return;
    if (!host.isConnected || host.parentNode !== parent || host.getAttribute('style') !== hostStyle || host.getAttribute('popover') !== 'manual') {
      remove();
      handlers.onGone();
    }
  });
  observer.observe(host, { attributes: true });
  observer.observe(doc.documentElement, { childList: true, subtree: true });

  function remove() {
    if (removed) return;
    removed = true;
    observer.disconnect();
    window.removeEventListener('message', onMessage);
    host.remove();
  }

  return { remove };
}

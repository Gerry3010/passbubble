// Injects the fill-suggestion iframe next to a detected password field.
// Uses an <iframe> (not shadow DOM) for a hard CSP boundary — the host page cannot
// read the iframe's DOM or intercept credentials.

import browser from 'webextension-polyfill';
import type { EntryResponse } from '@passbubble/shared-ts';

let activeIframe: HTMLIFrameElement | null = null;

export function injectFillIframe(
  anchorField: HTMLInputElement,
  matches: EntryResponse[],
  onFill: (username: string, password: string) => void,
  onDismiss: () => void,
): void {
  removeFillIframe();

  const iframe = document.createElement('iframe');
  iframe.src = browser.runtime.getURL('fill-iframe/index.html');
  iframe.style.cssText = [
    'position: fixed',
    'z-index: 2147483647',
    'border: none',
    'border-radius: 8px',
    'box-shadow: 0 4px 24px rgba(0,0,0,0.18)',
    'width: 320px',
    'height: 200px',
    'background: transparent',
  ].join(';');
  positionIframe(iframe, anchorField);
  document.body.appendChild(iframe);
  activeIframe = iframe;

  iframe.addEventListener('load', () => {
    iframe.contentWindow?.postMessage(
      { type: 'FILL_MATCHES', matches },
      browser.runtime.getURL(''),
    );
  });

  window.addEventListener('message', function handler(event) {
    // Only accept messages from our extension
    if (event.origin !== new URL(browser.runtime.getURL('')).origin) return;
    const msg = event.data as { type: string; entryId?: string };
    if (msg.type === 'FILL_SELECTED' && msg.entryId) {
      browser.runtime
        .sendMessage({ type: 'FILL_ENTRY', payload: { entryId: msg.entryId } })
        .then((resp: { username?: string; password?: string; locked?: boolean }) => {
          if (resp.locked) return;
          onFill(resp.username ?? '', resp.password ?? '');
          removeFillIframe();
        });
      window.removeEventListener('message', handler);
    } else if (msg.type === 'FILL_DISMISS') {
      onDismiss();
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
}

function positionIframe(iframe: HTMLIFrameElement, anchor: HTMLInputElement): void {
  const rect = anchor.getBoundingClientRect();
  iframe.style.top = `${rect.bottom + 4}px`;
  iframe.style.left = `${Math.max(4, rect.left)}px`;
}

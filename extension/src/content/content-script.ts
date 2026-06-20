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

// Content script: runs in every page at document_idle.
// Handles form detection, fill UI injection, and save detection.

import browser from 'webextension-polyfill';
import { MessageType } from '../shared/constants.js';
import { detectLoginForms } from './form-detector.js';
import { injectFillIframe, removeFillIframe } from './fill-ui.js';
import { initSaveDetector } from './save-detector.js';

// Fill a form field in a way that works with React / Vue SPAs.
function fillField(field: HTMLInputElement, value: string): void {
  const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
    window.HTMLInputElement.prototype,
    'value',
  )?.set;
  if (nativeInputValueSetter) {
    nativeInputValueSetter.call(field, value);
  } else {
    field.value = value;
  }
  field.dispatchEvent(new Event('input', { bubbles: true }));
  field.dispatchEvent(new Event('change', { bubbles: true }));
}

// True while our extension context is still valid. After the extension is
// reloaded/updated the old content script keeps running but `browser.runtime`
// is torn down; any sendMessage then throws "Extension context invalidated".
function extensionAlive(): boolean {
  try {
    return !!browser.runtime?.id;
  } catch {
    return false;
  }
}

// Send a message, swallowing the "context invalidated" teardown error. Returns
// null when the context is gone (and tears down our observers so we stop firing
// into a dead runtime — this is the source of the admin-panel console errors).
async function safeSend<T>(message: { type: string; payload: Record<string, unknown> }): Promise<T | null> {
  if (!extensionAlive()) {
    teardown();
    return null;
  }
  try {
    return (await browser.runtime.sendMessage(message)) as T;
  } catch (err) {
    if (String(err).includes('context invalidated') || !extensionAlive()) {
      teardown();
      return null;
    }
    throw err;
  }
}

async function tryInjectFillUI(): Promise<void> {
  const sessionResp = await safeSend<{ isUnlocked?: boolean }>({
    type: MessageType.GET_SESSION,
    payload: {},
  });
  if (!sessionResp?.isUnlocked) return;

  const forms = detectLoginForms().filter((f) => !f.isSignup);
  if (forms.length === 0) return;

  const matchResp = await safeSend<unknown>({
    type: MessageType.GET_MATCHES_FOR_URL,
    payload: { url: location.href },
  });
  if (!Array.isArray(matchResp) || matchResp.length === 0) return;

  const { usernameField, passwordField } = forms[0];
  const anchor = usernameField ?? passwordField;

  injectFillIframe(
    anchor,
    matchResp,
    (username, password) => {
      if (usernameField) fillField(usernameField, username);
      fillField(passwordField, password);
    },
    () => {},
  );
}

// Watch for dynamically added password fields (SPAs). Debounced because pages
// like the admin panel mutate the DOM in bursts — without this we'd fire a
// storm of messages at the service worker on every keystroke/render.
let injectTimer: ReturnType<typeof setTimeout> | null = null;
const observer = new MutationObserver(() => {
  if (!extensionAlive()) {
    teardown();
    return;
  }
  if (!document.querySelector('input[type="password"]')) return;
  if (injectTimer) clearTimeout(injectTimer);
  injectTimer = setTimeout(() => void tryInjectFillUI(), 300);
});

// Dismiss fill iframe when clicking outside it
function onDocumentClick(e: MouseEvent): void {
  const target = e.target as HTMLElement;
  if (target.tagName !== 'IFRAME') removeFillIframe();
}

// Detach everything once our extension context is gone, so a reloaded/updated
// extension's stale content script stops throwing into a dead runtime.
function teardown(): void {
  observer.disconnect();
  if (injectTimer) clearTimeout(injectTimer);
  document.removeEventListener('click', onDocumentClick);
  removeFillIframe();
}

observer.observe(document.body, { childList: true, subtree: true });
document.addEventListener('click', onDocumentClick);

// Initial detection
void tryInjectFillUI();

// Save detection
initSaveDetector();

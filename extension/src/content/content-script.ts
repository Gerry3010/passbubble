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

async function tryInjectFillUI(): Promise<void> {
  const sessionResp = await browser.runtime.sendMessage({
    type: MessageType.GET_SESSION,
    payload: {},
  }) as { isUnlocked?: boolean };
  if (!sessionResp?.isUnlocked) return;

  const forms = detectLoginForms().filter((f) => !f.isSignup);
  if (forms.length === 0) return;

  const matchResp = await browser.runtime.sendMessage({
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

// Watch for dynamically added password fields (SPAs)
const observer = new MutationObserver(() => {
  const hasPwField = !!document.querySelector('input[type="password"]');
  if (hasPwField) {
    void tryInjectFillUI();
  }
});
observer.observe(document.body, { childList: true, subtree: true });

// Initial detection
void tryInjectFillUI();

// Dismiss fill iframe when clicking outside it
document.addEventListener('click', (e) => {
  const target = e.target as HTMLElement;
  if (target.tagName !== 'IFRAME') removeFillIframe();
});

// Save detection
initSaveDetector();

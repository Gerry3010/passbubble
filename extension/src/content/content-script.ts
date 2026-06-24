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
import { detectLoginForms, type DetectedForm } from './form-detector.js';
import { injectFillIframe, removeFillIframe, isFillIframeShown } from './fill-ui.js';
import { initSaveDetector, recoverPendingSave } from './save-detector.js';

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

// Returns the detected login/signup form whose username/password field is `el`,
// or null. Cheap-guards on the element type before scanning the DOM.
function loginFormFor(el: EventTarget | null): DetectedForm | null {
  if (!(el instanceof HTMLInputElement)) return null;
  const t = el.type;
  if (t && t !== 'text' && t !== 'email' && t !== 'password') return null;
  const forms = detectLoginForms();
  return forms.find((f) => f.usernameField === el || f.passwordField === el) ?? null;
}

// Report (once per frame) the host of the frame that has a login/signup form, so
// the popup can pre-fill search + "+ Site" with the form's host (e.g. an SSO iframe).
let reportedLoginFrame = false;
function maybeReportLoginFrame(): void {
  if (reportedLoginFrame || !extensionAlive()) return;
  if (detectLoginForms().length > 0) {
    reportedLoginFrame = true;
    void safeSend({
      type: MessageType.REPORT_LOGIN_FRAME,
      payload: { host: location.hostname.replace(/^www\./, '') },
    });
    observer.disconnect();
  }
}

// Fill the password field(s) with `password`. When the fields live in a real
// <form>, fill every password input in it (covers "confirm password"); otherwise
// only the detected field, to avoid touching unrelated inputs elsewhere on the page.
function fillPasswords(form: DetectedForm, password: string): void {
  if (form.form) {
    form.form.querySelectorAll<HTMLInputElement>('input[type="password"]').forEach((f) => fillField(f, password));
  } else if (form.passwordField) {
    fillField(form.passwordField, password);
  }
}

// Many signup forms reveal a "confirm password" field only AFTER the first
// password is entered. Briefly watch the form and fill any newly-appearing empty
// password field with the same generated password, so the two always match.
function autofillConfirmField(form: DetectedForm, password: string): void {
  if (!form.form) return;
  const root = form.form;
  const obs = new MutationObserver(() => {
    if (!extensionAlive()) {
      obs.disconnect();
      return;
    }
    root.querySelectorAll<HTMLInputElement>('input[type="password"]').forEach((f) => {
      if (!f.value) fillField(f, password);
    });
  });
  obs.observe(root, { childList: true, subtree: true });
  setTimeout(() => obs.disconnect(), 5000);
}

// A generated password is cached per origin so re-showing the box on the same
// site (e.g. when the confirm field is focused) reuses it instead of generating
// a fresh — otherwise the confirm field would never match.
let cachedGenerated: { origin: string; password: string } | null = null;

// Show the fill suggestion for a focused login/signup field. Login forms get the
// match list; signup forms get a generated password that is auto-saved on use.
async function showFillFor(form: DetectedForm): Promise<void> {
  const { usernameField, passwordField } = form;
  const anchor = usernameField ?? passwordField;
  if (!anchor) return;
  // Already shown for this field (e.g. duplicate focus event)? Do nothing.
  if (isFillIframeShown(anchor)) return;

  const stillFocused = () =>
    document.activeElement === usernameField || document.activeElement === passwordField;

  const sessionResp = await safeSend<{ isUnlocked?: boolean; isLoggedIn?: boolean }>({
    type: MessageType.GET_SESSION,
    payload: {},
  });
  if (!sessionResp?.isUnlocked) {
    // Locked or logged out: on a real login form (not a signup), offer an unlock
    // prompt whose button opens the toolbar popup. Nothing to suggest on signup.
    if (form.isSignup || !stillFocused()) return;
    injectFillIframe(
      anchor,
      { locked: true, loggedIn: !!sessionResp?.isLoggedIn },
      { onFillMatch: () => {}, onUseGenerated: () => {}, onDismiss: () => {} },
    );
    return;
  }

  if (form.isSignup) {
    // Register form → offer a generated password (cached per origin so the
    // confirm field reuses the same one) that we auto-save once.
    const origin = location.origin;
    let password = cachedGenerated?.origin === origin ? cachedGenerated.password : undefined;
    if (!password) {
      const gen = await safeSend<{ passwords?: { password: string }[] }>({
        type: MessageType.GENERATE,
        payload: { length: 20, include_symbols: true, count: 1 },
      });
      password = gen?.passwords?.[0]?.password;
      if (!password) return;
      cachedGenerated = { origin, password };
    }
    if (!stillFocused()) return;

    const pw = password;
    injectFillIframe(
      anchor,
      { generatePassword: pw },
      {
        onFillMatch: () => {},
        onUseGenerated: () => {
          // Only fill — never create an entry here. The username/email field is
          // often still empty at generation time, so saving is deferred to the
          // submit-time save-detector, which offers an in-page "save?" bar (and
          // can offer to update an existing entry on a known site).
          fillPasswords(form, pw);
          autofillConfirmField(form, pw); // fill the confirm field when it appears
        },
        onDismiss: () => {},
      },
    );
    return;
  }

  const matchResp = await safeSend<unknown>({
    type: MessageType.GET_MATCHES_FOR_URL,
    payload: { url: location.href },
  });
  if (!Array.isArray(matchResp) || matchResp.length === 0) return;
  if (!stillFocused()) return;

  injectFillIframe(
    anchor,
    { matches: matchResp },
    {
      onFillMatch: (username, password) => {
        if (usernameField) fillField(usernameField, username);
        if (passwordField) fillField(passwordField, password);
      },
      onUseGenerated: () => {},
      onDismiss: () => {},
    },
  );
}

// Show on focus of a login field (like a real password manager) rather than on
// mere presence — otherwise the box re-appears after every fill/dismiss.
function onFocusIn(e: FocusEvent): void {
  if (!extensionAlive()) {
    teardown();
    return;
  }
  const form = loginFormFor(e.target);
  if (form) void showFillFor(form);
}

// Dismiss when clicking outside the suggestion AND outside a login field (so
// clicking the field to focus it doesn't immediately close the box).
function onDocumentClick(e: MouseEvent): void {
  const target = e.target as HTMLElement;
  if (target.tagName === 'IFRAME' || loginFormFor(target)) return;
  removeFillIframe();
}

function onKeyDown(e: KeyboardEvent): void {
  if (e.key === 'Escape') removeFillIframe();
}

// When the tab regains focus (e.g. after unlocking in the popup), show the
// suggestion if a login field is currently focused.
function onWindowFocus(): void {
  if (!extensionAlive()) {
    teardown();
    return;
  }
  const form = loginFormFor(document.activeElement);
  if (form) void showFillFor(form);
}

// A login form may load after document_idle; watch only to report its host once.
const observer = new MutationObserver(() => {
  if (!extensionAlive()) {
    teardown();
    return;
  }
  maybeReportLoginFrame();
});

// Detach everything once our extension context is gone, so a reloaded/updated
// extension's stale content script stops throwing into a dead runtime.
function teardown(): void {
  observer.disconnect();
  document.removeEventListener('focusin', onFocusIn);
  document.removeEventListener('click', onDocumentClick);
  document.removeEventListener('keydown', onKeyDown);
  window.removeEventListener('focus', onWindowFocus);
  removeFillIframe();
}

document.addEventListener('focusin', onFocusIn);
document.addEventListener('click', onDocumentClick);
document.addEventListener('keydown', onKeyDown);
window.addEventListener('focus', onWindowFocus);
observer.observe(document.documentElement, { childList: true, subtree: true });

// Initial: report the host, and show now if a login field is already focused
// (many login pages autofocus their first field).
maybeReportLoginFrame();
{
  const focused = loginFormFor(document.activeElement);
  if (focused) void showFillFor(focused);
}

// Save detection
initSaveDetector();
// A "save this login?" offer can survive a post-login navigation in
// storage.session; the DOM-only bar does not, so re-show it on load.
void recoverPendingSave();

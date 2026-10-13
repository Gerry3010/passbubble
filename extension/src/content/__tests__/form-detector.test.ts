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

import { describe, it, expect, beforeEach } from 'vitest';
import { detectLoginForms } from '../form-detector.js';

// Helpers

function addInput(type: string, attrs: Record<string, string> = {}): HTMLInputElement {
  const el = document.createElement('input');
  el.type = type;
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  document.body.appendChild(el);
  return el;
}

describe('detectLoginForms', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('returns empty array when there is no password field', () => {
    addInput('text');
    addInput('email');
    expect(detectLoginForms()).toHaveLength(0);
  });

  it('detects a single password field as one form', () => {
    addInput('password');
    expect(detectLoginForms()).toHaveLength(1);
  });

  it('detects multiple password fields as multiple forms', () => {
    addInput('password');
    addInput('password');
    expect(detectLoginForms()).toHaveLength(2);
  });

  it('finds a preceding email field as the username field', () => {
    addInput('email');
    addInput('password');
    const [form] = detectLoginForms();
    expect(form.usernameField).not.toBeNull();
    expect(form.usernameField!.type).toBe('email');
  });

  it('finds a preceding text field as the username field', () => {
    addInput('text');
    addInput('password');
    const [form] = detectLoginForms();
    expect(form.usernameField).not.toBeNull();
  });

  it('prefers autocomplete="username" field over generic text', () => {
    addInput('text', { id: 'generic' });
    addInput('text', { autocomplete: 'username', id: 'user' });
    addInput('password');
    const [form] = detectLoginForms();
    expect(form.usernameField!.getAttribute('autocomplete')).toBe('username');
  });

  it('skips hidden password fields', () => {
    // Hidden via style (offsetParent null is already mocked; test visibility: hidden)
    const pw = addInput('password');
    pw.style.visibility = 'hidden';
    expect(detectLoginForms()).toHaveLength(0);
  });

  it('detects autocomplete="new-password" as a signup form', () => {
    addInput('text');
    addInput('password', { autocomplete: 'new-password' });
    const [form] = detectLoginForms();
    expect(form.isSignup).toBe(true);
  });

  it('detects a plain password field as a login form (not signup)', () => {
    addInput('text');
    addInput('password', { autocomplete: 'current-password' });
    const [form] = detectLoginForms();
    expect(form.isSignup).toBe(false);
  });

  it('detects "confirm" in name as signup', () => {
    addInput('password', { name: 'confirm_password' });
    const [form] = detectLoginForms();
    expect(form.isSignup).toBe(true);
  });

  it('sets passwordField to the input element', () => {
    const pw = addInput('password');
    const [form] = detectLoginForms();
    expect(form.passwordField).toBe(pw);
  });

  it('sets form to the containing <form> element when present', () => {
    const formEl = document.createElement('form');
    const email = document.createElement('input');
    email.type = 'email';
    const pw = document.createElement('input');
    pw.type = 'password';
    formEl.appendChild(email);
    formEl.appendChild(pw);
    document.body.appendChild(formEl);

    const [detected] = detectLoginForms();
    expect(detected.form).toBe(formEl);
  });

  it('sets form to null when no surrounding <form> element', () => {
    addInput('password');
    const [form] = detectLoginForms();
    expect(form.form).toBeNull();
  });

  it('still detects a password field added dynamically before calling detectLoginForms', () => {
    // MutationObserver wiring is in content-script.ts; here we verify that the
    // scan function picks up dynamically added fields correctly
    const pw = document.createElement('input');
    pw.type = 'password';
    document.body.appendChild(pw);
    expect(detectLoginForms()).toHaveLength(1);
  });

  it('detects a username-first login step (visible email + hidden password in a form)', () => {
    // Mirrors IONOS: page 1 has the email field + a hidden password placeholder;
    // the real password field only appears on step 2.
    const formEl = document.createElement('form');
    const email = document.createElement('input');
    email.type = 'email';
    email.setAttribute('autocomplete', 'username');
    const pw = document.createElement('input');
    pw.type = 'password';
    pw.style.display = 'none';
    formEl.append(email, pw);
    document.body.appendChild(formEl);

    const forms = detectLoginForms();
    expect(forms).toHaveLength(1);
    expect(forms[0].usernameField).toBe(email);
    expect(forms[0].passwordField).toBeNull();
    expect(forms[0].form).toBe(formEl);
  });

  it('does NOT treat a password-less form (newsletter/contact) as a login', () => {
    const formEl = document.createElement('form');
    const email = document.createElement('input');
    email.type = 'email';
    formEl.append(email);
    document.body.appendChild(formEl);
    expect(detectLoginForms()).toHaveLength(0);
  });

  it('does not double-count a form whose password field is visible', () => {
    const formEl = document.createElement('form');
    const email = document.createElement('input');
    email.type = 'email';
    const pw = document.createElement('input');
    pw.type = 'password';
    formEl.append(email, pw);
    document.body.appendChild(formEl);
    // Visible password → one normal login form, not an extra username-first one.
    expect(detectLoginForms()).toHaveLength(1);
  });
});

// Flutter web backs only the focused text field with a real <input> (see
// flutter.ts); these mirror the DOM shapes its engine produces.
describe('detectLoginForms on Flutter web', () => {
  function flutterHost(): HTMLElement {
    const view = document.createElement('flutter-view');
    const host = document.createElement('flt-text-editing-host');
    view.appendChild(host);
    document.body.appendChild(view);
    return host;
  }

  function flutterInput(type: string, attrs: Record<string, string> = {}): HTMLInputElement {
    const el = document.createElement('input');
    el.type = type;
    for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
    return el;
  }

  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('pairs the focused username with its AutofillGroup password sibling', () => {
    const form = document.createElement('form');
    const user = flutterInput('text', { autocomplete: 'username' });
    const pw = flutterInput('password', { autocomplete: 'current-password' });
    form.append(user, pw);
    flutterHost().appendChild(form);
    user.focus();

    const [detected] = detectLoginForms();
    expect(detected.usernameField).toBe(user);
    expect(detected.passwordField).toBe(pw);
  });

  it('ignores a dormant password input left behind in another form', () => {
    const host = flutterHost();
    const dormantForm = document.createElement('form');
    dormantForm.appendChild(flutterInput('password', { autocomplete: 'current-password' }));
    const userForm = document.createElement('form');
    const user = flutterInput('text', { autocomplete: 'username' });
    userForm.appendChild(user);
    host.append(dormantForm, userForm);
    user.focus();

    const forms = detectLoginForms();
    expect(forms).toHaveLength(1);
    expect(forms[0].usernameField).toBe(user);
    expect(forms[0].passwordField).toBeNull();
  });

  it('treats a focused Flutter field with an e-mail keyboard as a username step', () => {
    const user = flutterInput('text', { autocomplete: 'on', inputmode: 'email' });
    flutterHost().appendChild(user);
    user.focus();

    const [detected] = detectLoginForms();
    expect(detected.usernameField).toBe(user);
    expect(detected.passwordField).toBeNull();
  });

  it('does not offer logins on a focused Flutter field without any login hint', () => {
    const chat = flutterInput('text', { autocomplete: 'on' });
    flutterHost().appendChild(chat);
    chat.focus();
    expect(detectLoginForms()).toHaveLength(0);
  });

  it('finds fields inside a shadow root when given that root', () => {
    const pane = document.createElement('flt-glass-pane');
    document.body.appendChild(pane);
    const shadow = pane.attachShadow({ mode: 'open' });
    const user = flutterInput('text', { autocomplete: 'username' });
    const pw = flutterInput('password');
    const form = document.createElement('form');
    form.append(user, pw);
    shadow.appendChild(form);
    pw.focus();

    expect(detectLoginForms()).toHaveLength(0); // document scan can't see into it
    const [detected] = detectLoginForms(shadow);
    expect(detected.passwordField).toBe(pw);
    expect(detected.usernameField).toBe(user);
  });
});

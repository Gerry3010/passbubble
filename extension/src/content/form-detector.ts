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

import { deepActiveElement, isFlutterField, isLiveFlutterField } from './flutter.js';

export interface DetectedForm {
  form: HTMLFormElement | null;
  usernameField: HTMLInputElement | null;
  // null for a multi-step "username first" login step, where the password field
  // is not on this page yet (only a hidden placeholder, if any).
  passwordField: HTMLInputElement | null;
  isSignup: boolean;
}

function isVisible(el: HTMLElement): boolean {
  if (el.offsetParent === null) return false;
  const style = window.getComputedStyle(el);
  return style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0';
}

// Whether a fill into `el` can take effect. Flutter's inputs are transparent and
// often 0×0 by design, so visibility says nothing there — only whether Flutter
// still listens to the input (see flutter.ts).
function isFillable(el: HTMLInputElement): boolean {
  return isFlutterField(el) ? isLiveFlutterField(el) : isVisible(el);
}

function findUsernameField(passwordField: HTMLInputElement, root: Document | ShadowRoot): HTMLInputElement | null {
  // Walk backwards in the DOM to find the nearest text/email field before the password field
  const inputs = Array.from(root.querySelectorAll<HTMLInputElement>(
    'input[type="text"], input[type="email"], input[autocomplete="username"], input:not([type])',
  )).filter(isFillable);

  // Prefer autocomplete="username" or type="email"
  const preferred = inputs.filter(
    (i) =>
      i.getAttribute('autocomplete') === 'username' ||
      i.type === 'email' ||
      /user|email|login|name|account/i.test(i.name + i.id + i.placeholder),
  );
  if (preferred.length > 0) return preferred[preferred.length - 1];

  // Fall back to any visible text input that appears before the password field in DOM order
  const all = Array.from(root.querySelectorAll<HTMLInputElement>('input')).filter(isFillable);
  const pwdIdx = all.indexOf(passwordField);
  for (let i = pwdIdx - 1; i >= 0; i--) {
    const el = all[i];
    if (el.type === 'text' || el.type === 'email' || !el.type) return el;
  }
  return null;
}

// Flutter username-first step: the focused Flutter input when it says it holds a
// login id (autocomplete/autofill hint or the e-mail keyboard). Without an
// AutofillGroup Flutter has no password input in the DOM at that point, so the
// generic detection never sees a login form there.
function flutterUsernameStep(root: Document | ShadowRoot): HTMLInputElement | null {
  const active = deepActiveElement();
  if (!(active instanceof HTMLInputElement) || active.getRootNode() !== root || !isFlutterField(active)) return null;
  if (active.type !== 'text' && active.type !== 'email') return null;
  const ac = (active.getAttribute('autocomplete') ?? '').toLowerCase();
  const inputmode = (active.getAttribute('inputmode') ?? '').toLowerCase();
  return ac === 'username' || ac === 'email' || inputmode === 'email' || active.type === 'email' ? active : null;
}

// `root` is the document, or the shadow root a focused field lives in (older
// Flutter web versions render their inputs inside one).
export function detectLoginForms(root: Document | ShadowRoot = document): DetectedForm[] {
  const passwordFields = Array.from(
    root.querySelectorAll<HTMLInputElement>('input[type="password"]'),
  ).filter(isFillable);

  const forms: DetectedForm[] = passwordFields.map((pw) => {
    const autocomplete = pw.getAttribute('autocomplete') ?? '';
    const isSignup =
      autocomplete.includes('new-password') ||
      /confirm|repeat|retype/i.test(pw.name + pw.id + pw.placeholder);
    return {
      form: pw.closest('form'),
      usernameField: findUsernameField(pw, root),
      passwordField: pw,
      isSignup,
    };
  });

  // Multi-step "username first" logins (IONOS, Google, Microsoft, …): the first
  // page shows only the username/email field; the password field comes on the
  // next step. We detect these by a <form> that DOES contain a password field
  // but none visible (a hidden placeholder is the tell that it's a login form,
  // not a newsletter/contact form) plus a visible username/email field — so the
  // fill / unlock box appears on the username field too.
  const seenUser = new Set(forms.map((f) => f.usernameField).filter(Boolean));
  const userSelector =
    'input[autocomplete="username"], input[type="email"], input[type="text"], input:not([type])';
  for (const formEl of Array.from(root.querySelectorAll<HTMLFormElement>('form'))) {
    const pwInForm = Array.from(formEl.querySelectorAll<HTMLInputElement>('input[type="password"]'));
    if (pwInForm.length === 0 || pwInForm.some(isFillable)) continue; // no pw, or handled above
    const candidates = Array.from(formEl.querySelectorAll<HTMLInputElement>(userSelector)).filter(isFillable);
    const user =
      candidates.find((u) => u.getAttribute('autocomplete') === 'username' || u.type === 'email') ??
      candidates[0];
    if (user && !seenUser.has(user)) {
      seenUser.add(user);
      forms.push({ form: formEl, usernameField: user, passwordField: null, isSignup: false });
    }
  }

  const flutterUser = flutterUsernameStep(root);
  if (flutterUser && !seenUser.has(flutterUser)) {
    forms.push({ form: flutterUser.form, usernameField: flutterUser, passwordField: null, isSignup: false });
  }
  return forms;
}

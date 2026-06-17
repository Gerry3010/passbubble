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

export interface DetectedForm {
  form: HTMLFormElement | null;
  usernameField: HTMLInputElement | null;
  passwordField: HTMLInputElement;
  isSignup: boolean;
}

function isVisible(el: HTMLElement): boolean {
  if (el.offsetParent === null) return false;
  const style = window.getComputedStyle(el);
  return style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0';
}

function findUsernameField(passwordField: HTMLInputElement): HTMLInputElement | null {
  // Walk backwards in the DOM to find the nearest text/email field before the password field
  const inputs = Array.from(document.querySelectorAll<HTMLInputElement>(
    'input[type="text"], input[type="email"], input[autocomplete="username"], input:not([type])',
  )).filter(isVisible);

  // Prefer autocomplete="username" or type="email"
  const preferred = inputs.filter(
    (i) =>
      i.getAttribute('autocomplete') === 'username' ||
      i.type === 'email' ||
      /user|email|login|name|account/i.test(i.name + i.id + i.placeholder),
  );
  if (preferred.length > 0) return preferred[preferred.length - 1];

  // Fall back to any visible text input that appears before the password field in DOM order
  const all = Array.from(document.querySelectorAll<HTMLInputElement>('input')).filter(isVisible);
  const pwdIdx = all.indexOf(passwordField);
  for (let i = pwdIdx - 1; i >= 0; i--) {
    const el = all[i];
    if (el.type === 'text' || el.type === 'email' || !el.type) return el;
  }
  return null;
}

export function detectLoginForms(): DetectedForm[] {
  const passwordFields = Array.from(
    document.querySelectorAll<HTMLInputElement>('input[type="password"]'),
  ).filter(isVisible);

  return passwordFields.map((pw) => {
    const autocomplete = pw.getAttribute('autocomplete') ?? '';
    const isSignup =
      autocomplete.includes('new-password') ||
      /confirm|repeat|retype/i.test(pw.name + pw.id + pw.placeholder);
    return {
      form: pw.closest('form'),
      usernameField: findUsernameField(pw),
      passwordField: pw,
      isSignup,
    };
  });
}

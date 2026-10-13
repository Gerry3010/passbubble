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

// Flutter web text fields. Flutter paints its UI onto a canvas and backs only
// the FOCUSED text field with a real (transparent) <input>, created on focus.
// Fields of an AutofillGroup share a <form>: while one of them is focused the
// others sit in it as 0×0 inputs that Flutter also listens to. When the focus
// leaves, Flutter detaches its listeners and leaves the inputs behind "dormant"
// (moved to -9999px) or removes them. Consequences for autofill:
//   - a fill only reaches Flutter while its input is focused (or is a sibling
//     in the focused input's form) — clicking into our suggestion iframe moves
//     the focus away, so the iframe must not take it;
//   - dormant inputs are dead ends: writing to them changes nothing, and Flutter
//     overwrites them with its own state on the next focus;
//   - without an AutofillGroup the other login field does not exist until the
//     user focuses it, so its value has to wait for that focus.

// Elements Flutter web puts its text-editing inputs under, across engine
// versions (newer: <flutter-view> in the light DOM; older: inside the shadow
// root of <flt-glass-pane>; semantics mode: <flt-semantics-host>).
const FLUTTER_HOSTS = 'flutter-view, flt-glass-pane, flt-text-editing-host, flt-semantics-host';

// True when `el` is one of Flutter web's text-editing inputs.
export function isFlutterField(el: Element): boolean {
  let node: Element | null = el;
  while (node) {
    if (node.closest(FLUTTER_HOSTS)) return true;
    const root = node.getRootNode();
    node = root instanceof ShadowRoot ? root.host : null;
  }
  return false;
}

// The focused element, resolved through open shadow roots (document.activeElement
// stops at the shadow host).
export function deepActiveElement(): Element | null {
  let active = document.activeElement;
  while (active?.shadowRoot?.activeElement) active = active.shadowRoot.activeElement;
  return active;
}

// True while Flutter is listening to this input: it is focused, or it belongs to
// the focused input's form (an AutofillGroup). Everything else is dormant.
export function isLiveFlutterField(el: HTMLInputElement): boolean {
  const active = deepActiveElement();
  if (el === active) return true;
  return !!el.form && active instanceof HTMLInputElement && active.form === el.form;
}

// ---- Deferred fill -----------------------------------------------------------
// Values of a chosen login entry that had no live field to go into (no
// AutofillGroup: the other field is not in the DOM yet). They are applied when
// the user focuses that field next — same URL, short window, one shot each.
// A route change (the login went through) or submitting with Enter drops them,
// so a leftover username never lands in some text field of the next screen.

const PENDING_TTL_MS = 60_000;

let pending: { username?: string; password?: string; expires: number; href: string } | null = null;

export function rememberPendingFill(values: { username?: string; password?: string }): void {
  if (!values.username && !values.password) {
    pending = null;
    return;
  }
  pending = { ...values, expires: Date.now() + PENDING_TTL_MS, href: location.href };
}

export function clearPendingFill(): void {
  pending = null;
}

export function hasPendingFill(): boolean {
  if (pending && (Date.now() > pending.expires || location.href !== pending.href)) pending = null;
  return !!pending;
}

// True when a Flutter text field on the page (live or dormant) already holds
// text — then the user has typed their username themselves.
export function flutterTextEntered(): boolean {
  return Array.from(document.querySelectorAll<HTMLInputElement>('input[type="text"], input[type="email"]')).some(
    (i) => i.value !== '' && isFlutterField(i),
  );
}

// inputmode values Flutter sets for keyboard types that are clearly not a login
// id (url, numbers, phone) — never put a username there.
const NON_USERNAME_INPUTMODES = new Set(['url', 'numeric', 'decimal', 'tel', 'none']);

// Take the pending value that belongs into `field`, if any: a password for a
// password field, the username for an empty text field that is not obviously
// meant for something else (URL, number, one-time code, …). Consumes the value.
export function takePendingFill(field: HTMLInputElement): string | null {
  if (!hasPendingFill() || !pending) return null;
  if (field.value) return null;
  let value: string | undefined;
  if (field.type === 'password') {
    value = pending.password;
    pending.password = undefined;
  } else {
    if (field.type !== 'text' && field.type !== 'email') return null;
    const ac = (field.getAttribute('autocomplete') ?? '').toLowerCase();
    if (ac && !['on', 'off', 'username', 'email'].includes(ac)) return null;
    if (NON_USERNAME_INPUTMODES.has((field.getAttribute('inputmode') ?? '').toLowerCase())) return null;
    value = pending.username;
    pending.username = undefined;
  }
  if (!pending.username && !pending.password) pending = null;
  return value ?? null;
}

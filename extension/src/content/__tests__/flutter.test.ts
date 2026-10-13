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

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  clearPendingFill,
  deepActiveElement,
  flutterTextEntered,
  hasPendingFill,
  isFlutterField,
  isLiveFlutterField,
  rememberPendingFill,
  takePendingFill,
} from '../flutter.js';

// Builds Flutter web's text-editing DOM (engine ≥ 3.22): <flutter-view> with a
// <flt-text-editing-host> holding one <form> per autofill group.
function flutterHost(): HTMLElement {
  const view = document.createElement('flutter-view');
  const host = document.createElement('flt-text-editing-host');
  view.appendChild(host);
  document.body.appendChild(view);
  return host;
}

function input(type: string, attrs: Record<string, string> = {}): HTMLInputElement {
  const el = document.createElement('input');
  el.type = type;
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  return el;
}

describe('isFlutterField', () => {
  it('recognises inputs under <flutter-view>', () => {
    const el = input('text');
    flutterHost().appendChild(el);
    expect(isFlutterField(el)).toBe(true);
  });

  it('recognises inputs inside the shadow root of an older <flt-glass-pane>', () => {
    const pane = document.createElement('flt-glass-pane');
    document.body.appendChild(pane);
    const el = input('password');
    pane.attachShadow({ mode: 'open' }).appendChild(el);
    expect(isFlutterField(el)).toBe(true);
  });

  it('is false for ordinary page inputs', () => {
    const el = input('text');
    document.body.appendChild(el);
    expect(isFlutterField(el)).toBe(false);
  });
});

describe('deepActiveElement', () => {
  it('resolves the focused element inside an open shadow root', () => {
    const pane = document.createElement('flt-glass-pane');
    document.body.appendChild(pane);
    const el = input('text');
    pane.attachShadow({ mode: 'open' }).appendChild(el);
    el.focus();
    expect(deepActiveElement()).toBe(el);
  });
});

describe('isLiveFlutterField', () => {
  it('treats the focused input and its AutofillGroup siblings as live, other forms as dormant', () => {
    const host = flutterHost();
    const dormantForm = document.createElement('form');
    const dormant = input('password');
    dormantForm.appendChild(dormant);
    const groupForm = document.createElement('form');
    const user = input('text', { autocomplete: 'username' });
    const pw = input('password', { autocomplete: 'current-password' });
    groupForm.append(user, pw);
    host.append(dormantForm, groupForm);

    user.focus();

    expect(isLiveFlutterField(user)).toBe(true);
    expect(isLiveFlutterField(pw)).toBe(true);
    expect(isLiveFlutterField(dormant)).toBe(false);
  });

  it('treats a formless input as live only while focused', () => {
    const host = flutterHost();
    const a = input('text');
    const b = input('text');
    host.append(a, b);
    a.focus();
    expect(isLiveFlutterField(a)).toBe(true);
    expect(isLiveFlutterField(b)).toBe(false);
  });
});

describe('pending fill', () => {
  beforeEach(() => clearPendingFill());
  afterEach(() => {
    vi.useRealTimers();
    history.replaceState(null, '', '/');
  });

  it('hands the password to a password field and the username to a text field, once each', () => {
    rememberPendingFill({ username: 'me@example.com', password: 'secret' });
    const pw = input('password');
    const user = input('text', { autocomplete: 'on' });

    expect(takePendingFill(pw)).toBe('secret');
    expect(takePendingFill(pw)).toBeNull();
    expect(takePendingFill(user)).toBe('me@example.com');
    expect(hasPendingFill()).toBe(false);
  });

  it('never puts the username into a field meant for something else', () => {
    rememberPendingFill({ username: 'me@example.com' });
    expect(takePendingFill(input('text', { inputmode: 'url' }))).toBeNull();
    expect(takePendingFill(input('text', { autocomplete: 'one-time-code' }))).toBeNull();
    expect(takePendingFill(input('text', { inputmode: 'numeric' }))).toBeNull();
    expect(takePendingFill(input('email'))).toBe('me@example.com');
  });

  it('leaves fields alone that already hold text', () => {
    rememberPendingFill({ password: 'secret' });
    const pw = input('password');
    pw.value = 'typed';
    expect(takePendingFill(pw)).toBeNull();
    expect(hasPendingFill()).toBe(true);
  });

  it('expires after a minute', () => {
    vi.useFakeTimers();
    rememberPendingFill({ password: 'secret' });
    vi.advanceTimersByTime(61_000);
    expect(hasPendingFill()).toBe(false);
    expect(takePendingFill(input('password'))).toBeNull();
  });

  it('is dropped once the URL changes (the login went through)', () => {
    rememberPendingFill({ username: 'me@example.com' });
    history.pushState(null, '', '/#/entries');
    expect(hasPendingFill()).toBe(false);
    expect(takePendingFill(input('text'))).toBeNull();
  });

  it('remembers nothing when every value found its field', () => {
    rememberPendingFill({ username: undefined, password: undefined });
    expect(hasPendingFill()).toBe(false);
  });
});

describe('flutterTextEntered', () => {
  it('notices a username the user typed into a (dormant) Flutter field', () => {
    const user = input('text');
    flutterHost().appendChild(user);
    expect(flutterTextEntered()).toBe(false);
    user.value = 'typed@example.com';
    expect(flutterTextEntered()).toBe(true);
  });

  it('ignores ordinary page inputs', () => {
    const search = input('text');
    search.value = 'query';
    document.body.appendChild(search);
    expect(flutterTextEntered()).toBe(false);
  });
});

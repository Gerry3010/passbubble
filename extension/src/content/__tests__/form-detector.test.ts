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
});

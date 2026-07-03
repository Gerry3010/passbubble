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

import { describe, it, expect } from 'vitest';
import { classifyField, classifyOtpField } from '../field-classifier.js';

function makeInput(attrs: Record<string, string>): HTMLInputElement {
  const el = document.createElement('input');
  for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
  return el;
}

describe('classifyOtpField', () => {
  it('accepts autocomplete="one-time-code" regardless of other hints', () => {
    expect(classifyOtpField(makeInput({ autocomplete: 'one-time-code' }))).toBe(true);
    expect(classifyOtpField(makeInput({ type: 'text', autocomplete: 'one-time-code' }))).toBe(true);
  });

  it('accepts a short numeric field with an OTP-ish name', () => {
    expect(classifyOtpField(makeInput({ inputmode: 'numeric', name: 'otp' }))).toBe(true);
    expect(classifyOtpField(makeInput({ inputmode: 'numeric', id: 'totp-token' }))).toBe(true);
    expect(classifyOtpField(makeInput({ maxlength: '6', name: 'twofa_code' }))).toBe(true);
    expect(classifyOtpField(makeInput({ maxlength: '6', placeholder: 'Verification code' }))).toBe(true);
    expect(classifyOtpField(makeInput({ type: 'tel', name: 'security-code' }))).toBe(true);
    expect(classifyOtpField(makeInput({ inputmode: 'numeric', placeholder: 'Einmalcode' }))).toBe(true);
  });

  it('rejects password/email fields', () => {
    expect(classifyOtpField(makeInput({ type: 'password', name: 'otp' }))).toBe(false);
    expect(classifyOtpField(makeInput({ type: 'email', name: 'code' }))).toBe(false);
  });

  it('rejects numeric fields without any OTP hint', () => {
    expect(classifyOtpField(makeInput({ inputmode: 'numeric', name: 'amount' }))).toBe(false);
    expect(classifyOtpField(makeInput({ maxlength: '6', name: 'plz' }))).toBe(false);
  });

  it('rejects long free-text fields even when named "code"', () => {
    // e.g. a discount-code textarea-like input: not numeric, no maxlength
    expect(classifyOtpField(makeInput({ type: 'text', name: 'promo_code' }))).toBe(false);
  });

  it('rejects a postcode field with a code-like German name', () => {
    expect(classifyOtpField(makeInput({ inputmode: 'numeric', name: 'postleitzahl' }))).toBe(false);
  });
});

describe('classifyField', () => {
  it('classifies by autocomplete tokens (Stripe-style checkout)', () => {
    expect(classifyField(makeInput({ autocomplete: 'cc-number' }))).toBe('cc-number');
    expect(classifyField(makeInput({ autocomplete: 'cc-name' }))).toBe('cc-name');
    expect(classifyField(makeInput({ autocomplete: 'cc-exp' }))).toBe('cc-exp');
    expect(classifyField(makeInput({ autocomplete: 'cc-csc' }))).toBe('cc-csc');
    expect(classifyField(makeInput({ autocomplete: 'billing cc-exp-month' }))).toBe('cc-exp-month');
    expect(classifyField(makeInput({ autocomplete: 'shipping street-address' }))).toBe('street-address');
    expect(classifyField(makeInput({ autocomplete: 'address-level2' }))).toBe('city');
    expect(classifyField(makeInput({ autocomplete: 'country-name' }))).toBe('country');
  });

  it('classifies by name/placeholder heuristics (Amazon-style markup)', () => {
    expect(classifyField(makeInput({ name: 'cardNumber', type: 'tel' }))).toBe('cc-number');
    expect(classifyField(makeInput({ id: 'expirationDate', placeholder: 'MM/YY' }))).toBe('cc-exp');
    expect(classifyField(makeInput({ name: 'securityCode' }))).toBe('cc-csc');
    expect(classifyField(makeInput({ name: 'nameOnCard' }))).toBe('cc-name');
  });

  it('classifies German form fields', () => {
    expect(classifyField(makeInput({ name: 'kartennummer' }))).toBe('cc-number');
    expect(classifyField(makeInput({ placeholder: 'Vorname' }))).toBe('given-name');
    expect(classifyField(makeInput({ placeholder: 'Nachname' }))).toBe('family-name');
    expect(classifyField(makeInput({ name: 'strasse' }))).toBe('street-address');
    expect(classifyField(makeInput({ name: 'plz' }))).toBe('postal-code');
    expect(classifyField(makeInput({ name: 'wohnort' }))).toBe('city');
    expect(classifyField(makeInput({ name: 'firma' }))).toBe('organization');
  });

  it('classifies type=email and type=tel inputs', () => {
    expect(classifyField(makeInput({ type: 'email' }))).toBe('email');
    expect(classifyField(makeInput({ type: 'tel', name: 'phone' }))).toBe('tel');
  });

  it('classifies select elements (country, expiry month)', () => {
    const country = document.createElement('select');
    country.name = 'country';
    expect(classifyField(country)).toBe('country');
    const month = document.createElement('select');
    month.setAttribute('autocomplete', 'cc-exp-month');
    expect(classifyField(month)).toBe('cc-exp-month');
  });

  it('never classifies login/OTP/password fields', () => {
    expect(classifyField(makeInput({ type: 'password', name: 'password' }))).toBeNull();
    expect(classifyField(makeInput({ autocomplete: 'username' }))).toBeNull();
    expect(classifyField(makeInput({ autocomplete: 'current-password' }))).toBeNull();
    expect(classifyField(makeInput({ autocomplete: 'one-time-code' }))).toBeNull();
    expect(classifyField(makeInput({ inputmode: 'numeric', name: 'otp' }))).toBeNull();
  });

  it('returns null for fields it does not understand', () => {
    expect(classifyField(makeInput({ name: 'search' }))).toBeNull();
    expect(classifyField(makeInput({ type: 'checkbox', name: 'country' }))).toBeNull();
    expect(classifyField(document.createElement('div'))).toBeNull();
  });
});

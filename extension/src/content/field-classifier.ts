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

// Heuristic input-field classification for autofill. Primary signal is the
// standard `autocomplete` token; fallbacks look at name/id/placeholder/aria-label
// (English + German).

type ClassifiableElement = HTMLInputElement | HTMLSelectElement;

function haystack(el: ClassifiableElement): string {
  const placeholder = el instanceof HTMLInputElement ? el.placeholder : '';
  return `${el.name} ${el.id} ${placeholder} ${el.getAttribute('aria-label') ?? ''}`;
}

/**
 * True when `el` looks like a one-time-code (TOTP/SMS) field: the standard
 * autocomplete="one-time-code" token, or a short numeric-ish input whose
 * name/id/placeholder/label mentions an OTP concept.
 */
export function classifyOtpField(el: HTMLInputElement): boolean {
  const type = el.type;
  if (type === 'password' || type === 'email' || type === 'checkbox' || type === 'radio') return false;
  const ac = (el.getAttribute('autocomplete') ?? '').toLowerCase();
  if (ac.includes('one-time-code')) return true;

  const numeric = (el.getAttribute('inputmode') ?? '').toLowerCase() === 'numeric' || type === 'number' || type === 'tel';
  const shortField = el.maxLength >= 4 && el.maxLength <= 8;
  if (!numeric && !shortField) return false;
  return /otp|totp|2fa|two.?fa|mfa|one.?time|einmal|verif|authenticator|(^|[^a-z])code([^a-z]|$)/i.test(
    haystack(el),
  );
}

// Field kinds the typed autofill (credit cards / identities) understands.
// The cc-* names deliberately mirror the standard autocomplete tokens.
export type FieldKind =
  | 'cc-number'
  | 'cc-name'
  | 'cc-exp'
  | 'cc-exp-month'
  | 'cc-exp-year'
  | 'cc-csc'
  | 'name'
  | 'given-name'
  | 'family-name'
  | 'organization'
  | 'email'
  | 'tel'
  | 'street-address'
  | 'postal-code'
  | 'city'
  | 'state'
  | 'country';

export const CC_KINDS: ReadonlySet<FieldKind> = new Set([
  'cc-number',
  'cc-name',
  'cc-exp',
  'cc-exp-month',
  'cc-exp-year',
  'cc-csc',
]);

// autocomplete token → kind (only tokens we can fill).
const AUTOCOMPLETE_KINDS: Record<string, FieldKind> = {
  'cc-number': 'cc-number',
  'cc-name': 'cc-name',
  'cc-exp': 'cc-exp',
  'cc-exp-month': 'cc-exp-month',
  'cc-exp-year': 'cc-exp-year',
  'cc-csc': 'cc-csc',
  name: 'name',
  'given-name': 'given-name',
  'family-name': 'family-name',
  organization: 'organization',
  email: 'email',
  tel: 'tel',
  'tel-national': 'tel',
  'street-address': 'street-address',
  'address-line1': 'street-address',
  'postal-code': 'postal-code',
  'address-level2': 'city',
  'address-level1': 'state',
  country: 'country',
  'country-name': 'country',
};

// Fallback name/id/placeholder/label patterns, checked in order — the most
// specific first so e.g. "cardholder name" hits cc-name, not name.
const KIND_PATTERNS: Array<[FieldKind, RegExp]> = [
  ['cc-number', /card.?(number|no)|kartennummer|cc.?num|creditcard|kreditkarte/i],
  ['cc-csc', /cvv|cvc|csc|card.?verif|security.?code|prüfnummer|kartenprüf/i],
  ['cc-exp-month', /(exp|expiry|ablauf|gültig)\w*[-_ ]?(month|monat)|cc.?month/i],
  ['cc-exp-year', /(exp|expiry|ablauf|gültig)\w*[-_ ]?(year|jahr)|cc.?year/i],
  ['cc-exp', /exp(iry|iration)?.?date|mm.?[/-].?(yy|jj)|ablaufdatum|gültig.?bis/i],
  ['cc-name', /card.?holder|holder.?name|name.?on.?card|karteninhaber/i],
  ['email', /e-?mail/i],
  ['tel', /phone|telefon|(^|[^a-z])tel([^a-z]|$)|handy|mobil/i],
  ['given-name', /first.?name|vorname|given.?name/i],
  ['family-name', /last.?name|nachname|surname|family.?name|zuname/i],
  ['organization', /company|organi[sz]ation|firma/i],
  ['street-address', /street|address.?line.?1|(^|[^a-z])addres+e?([^a-z]|$)|straße|strasse|anschrift/i],
  ['postal-code', /zip|postal|postcode|(^|[^a-z])plz([^a-z]|$)|postleitzahl/i],
  ['city', /city|town|(^|[^a-z])ort([^a-z]|$)|stadt|wohnort/i],
  ['state', /state|province|region|bundesland/i],
  ['country', /country|(^|[^a-z])land([^a-z]|$)/i],
];

/**
 * Classifies a form field for typed autofill (credit card / identity data).
 * Returns null for fields we don't understand — including anything that looks
 * like a login or OTP field, which the login autofill handles instead.
 */
export function classifyField(el: Element): FieldKind | null {
  const isInput = el instanceof HTMLInputElement;
  if (!isInput && !(el instanceof HTMLSelectElement)) return null;
  if (isInput) {
    const t = el.type;
    if (t === 'password' || t === 'hidden' || t === 'checkbox' || t === 'radio' || t === 'submit' || t === 'button') {
      return null;
    }
    if (classifyOtpField(el)) return null;
  }

  const ac = (el.getAttribute('autocomplete') ?? '').toLowerCase();
  if (ac) {
    // The relevant token is the last one (earlier ones are section/shipping hints).
    for (const token of ac.split(/\s+/).reverse()) {
      const kind = AUTOCOMPLETE_KINDS[token];
      if (kind) return kind;
      if (token === 'off' || token === 'on') continue;
    }
    // username/current-password/one-time-code etc.: not ours.
    if (/user|password|one-time/.test(ac)) return null;
  }

  if (isInput && el.type === 'email') return 'email';
  if (isInput && el.type === 'tel') {
    // A bare type=tel is ambiguous (often used for card/OTP inputs) — only
    // classify as phone when the name agrees or nothing else matches better.
    for (const [kind, re] of KIND_PATTERNS) if (re.test(haystack(el))) return kind;
    return 'tel';
  }

  const hay = haystack(el);
  for (const [kind, re] of KIND_PATTERNS) if (re.test(hay)) return kind;
  return null;
}

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
// standard `autocomplete` token; fallbacks look at name/id/placeholder/aria-label.

function haystack(el: HTMLInputElement): string {
  return `${el.name} ${el.id} ${el.placeholder} ${el.getAttribute('aria-label') ?? ''}`;
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

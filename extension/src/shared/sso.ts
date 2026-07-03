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

// "Sign in with …" (SSO) detection shared by the content script (button-text
// heuristics) and the background (OAuth navigation matching).

export type SsoProvider = 'google' | 'apple' | 'microsoft' | 'github' | 'facebook';

export const SSO_PROVIDER_LABELS: Record<SsoProvider, string> = {
  google: 'Google',
  apple: 'Apple',
  microsoft: 'Microsoft',
  github: 'GitHub',
  facebook: 'Facebook',
};

/** Type guard for provider values read from entry data (untyped JSON). */
export function isSsoProvider(value: unknown): value is SsoProvider {
  return typeof value === 'string' && value in SSO_PROVIDER_LABELS;
}

const PROVIDER_NAME_RES: Array<[SsoProvider, RegExp]> = [
  ['google', /google/i],
  ['apple', /\bapple\b/i],
  ['microsoft', /microsoft|azure ad/i],
  ['github', /github/i],
  ['facebook', /facebook/i],
];

// "sign in / continue"-flavoured words, EN + DE. Both orders are covered by
// requiring the verb and the provider name anywhere in the same button text
// ("Sign in with Google", "Mit Google anmelden", "Continue with Apple", …).
const SSO_VERB_RE = /sign.?in|sign.?up|log.?in|anmeld|einlogg|registrier|weiter|continue|fortfahren/i;

/**
 * The SSO provider a clickable's text refers to, when the text looks like a
 * "sign in with X" affordance — null otherwise.
 */
export function providerFromText(text: string): SsoProvider | null {
  const t = text.slice(0, 300);
  if (!SSO_VERB_RE.test(t)) return null;
  for (const [provider, re] of PROVIDER_NAME_RES) {
    if (re.test(t)) return provider;
  }
  return null;
}

// OAuth/OpenID authorization endpoints per provider. Deliberately narrow —
// matching all of google.com would record a plain visit as an SSO login.
const PROVIDER_URL_RES: Array<[SsoProvider, RegExp]> = [
  ['google', /^https:\/\/accounts\.google\.com\/(o\/oauth2|signin|v3\/signin|gsi)/i],
  ['apple', /^https:\/\/appleid\.apple\.com\/auth\//i],
  ['microsoft', /^https:\/\/login\.(microsoftonline|live)\.com\//i],
  ['github', /^https:\/\/github\.com\/login\/oauth\//i],
  ['facebook', /^https:\/\/(www|m)\.facebook\.com\/(v[\d.]+\/)?(dialog\/oauth|login)/i],
];

/** The SSO provider whose authorization endpoint `url` is, or null. */
export function providerForUrl(url: string): SsoProvider | null {
  for (const [provider, re] of PROVIDER_URL_RES) {
    if (re.test(url)) return provider;
  }
  return null;
}

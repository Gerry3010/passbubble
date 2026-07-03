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
import { providerFromText, providerForUrl } from '../sso.js';

describe('providerFromText', () => {
  it('recognises English "sign in with" buttons', () => {
    expect(providerFromText('Sign in with Google')).toBe('google');
    expect(providerFromText('Continue with Apple')).toBe('apple');
    expect(providerFromText('Log in with GitHub')).toBe('github');
    expect(providerFromText('Sign up with Facebook')).toBe('facebook');
    expect(providerFromText('Sign in with Microsoft')).toBe('microsoft');
  });

  it('recognises German button wording in both orders', () => {
    expect(providerFromText('Mit Google anmelden')).toBe('google');
    expect(providerFromText('Weiter mit Apple')).toBe('apple');
    expect(providerFromText('Mit GitHub einloggen')).toBe('github');
    expect(providerFromText('Anmelden mit Microsoft')).toBe('microsoft');
  });

  it('recognises aria-label style text', () => {
    expect(providerFromText('Sign in with Google Button')).toBe('google');
  });

  it('rejects buttons that merely mention a provider', () => {
    expect(providerFromText('Download Google Chrome')).toBeNull();
    expect(providerFromText('Visit our GitHub')).toBeNull();
    expect(providerFromText('Google')).toBeNull();
  });

  it('rejects plain login buttons', () => {
    expect(providerFromText('Sign in')).toBeNull();
    expect(providerFromText('Anmelden')).toBeNull();
  });
});

describe('providerForUrl', () => {
  it('matches provider OAuth authorization endpoints', () => {
    expect(providerForUrl('https://accounts.google.com/o/oauth2/v2/auth?client_id=x')).toBe('google');
    expect(providerForUrl('https://accounts.google.com/signin/oauth')).toBe('google');
    expect(providerForUrl('https://appleid.apple.com/auth/authorize?client_id=x')).toBe('apple');
    expect(providerForUrl('https://login.microsoftonline.com/common/oauth2/v2.0/authorize')).toBe('microsoft');
    expect(providerForUrl('https://login.live.com/oauth20_authorize.srf')).toBe('microsoft');
    expect(providerForUrl('https://github.com/login/oauth/authorize?client_id=x')).toBe('github');
    expect(providerForUrl('https://www.facebook.com/v18.0/dialog/oauth?client_id=x')).toBe('facebook');
  });

  it('does not match ordinary provider pages', () => {
    expect(providerForUrl('https://www.google.com/search?q=x')).toBeNull();
    expect(providerForUrl('https://github.com/Gerry3010/passbubble')).toBeNull();
    expect(providerForUrl('https://mail.google.com/')).toBeNull();
    expect(providerForUrl('https://example.com/login/oauth/authorize')).toBeNull();
  });
});

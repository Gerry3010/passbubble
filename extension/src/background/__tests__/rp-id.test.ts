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
import { checkRequestOrigin, isIpHost, normalizeRpId } from '../rp-id.js';

const ok = (url: string, rpId?: unknown) => checkRequestOrigin(url, rpId);

describe('normalizeRpId', () => {
  it('lowercases and drops a trailing dot and a port', () => {
    expect(normalizeRpId('Example.COM')).toBe('example.com');
    expect(normalizeRpId('example.com.')).toBe('example.com');
    expect(normalizeRpId('example.com:8443')).toBe('example.com');
  });

  it('converts unicode names to punycode', () => {
    expect(normalizeRpId('bücher.example')).toBe('xn--bcher-kva.example');
  });

  it('rejects what cannot be a domain', () => {
    for (const bad of ['', 'a b.com', 'user@example.com', 'example.com/path', 'exa%6dple.com', '.example.com', 'a..b', 42, null]) {
      expect(normalizeRpId(bad)).toBeNull();
    }
  });
});

describe('checkRequestOrigin', () => {
  it('accepts the page host as rp id, and the host as default', () => {
    expect(ok('https://example.com/login', 'example.com')).toEqual({
      ok: true,
      origin: 'https://example.com',
      host: 'example.com',
      rpId: 'example.com',
    });
    expect(ok('https://login.example.com/', undefined)).toMatchObject({ ok: true, rpId: 'login.example.com' });
  });

  it('accepts a parent domain on a label boundary only', () => {
    expect(ok('https://login.example.com/', 'example.com')).toMatchObject({ ok: true, rpId: 'example.com' });
    expect(ok('https://a.b.example.com/', 'b.example.com')).toMatchObject({ ok: true });
    expect(ok('https://notexample.com/', 'example.com').ok).toBe(false);
    expect(ok('https://example.com/', 'login.example.com').ok).toBe(false);
    expect(ok('https://example.com/', 'other.com').ok).toBe(false);
  });

  it('treats www and the bare domain as different rp ids', () => {
    expect(ok('https://www.example.com/', 'example.com')).toMatchObject({ ok: true, rpId: 'example.com' });
    expect(ok('https://example.com/', 'www.example.com').ok).toBe(false);
    expect(ok('https://www.example.com/', undefined)).toMatchObject({ ok: true, rpId: 'www.example.com' });
  });

  it('refuses public suffixes, including private ones like github.io', () => {
    expect(ok('https://alice.github.io/', 'github.io')).toEqual({ ok: false, reason: 'public suffix' });
    expect(ok('https://alice.github.io/', 'alice.github.io')).toMatchObject({ ok: true });
    expect(ok('https://shop.example.co.uk/', 'co.uk')).toEqual({ ok: false, reason: 'public suffix' });
    expect(ok('https://shop.example.co.uk/', 'uk')).toEqual({ ok: false, reason: 'public suffix' });
    expect(ok('https://shop.example.co.uk/', 'example.co.uk')).toMatchObject({ ok: true, rpId: 'example.co.uk' });
    expect(ok('https://example.com/', 'com').ok).toBe(false);
  });

  it('needs https, or http on localhost', () => {
    expect(ok('http://example.com/', 'example.com')).toEqual({ ok: false, reason: 'insecure origin' });
    expect(ok('http://localhost:3000/', undefined)).toEqual({
      ok: true,
      origin: 'http://localhost:3000',
      host: 'localhost',
      rpId: 'localhost',
    });
    expect(ok('https://app.localhost/', 'localhost')).toMatchObject({ ok: true, rpId: 'localhost' });
    expect(ok('http://app.localhost/', 'localhost').ok).toBe(false);
    for (const url of ['file:///tmp/x.html', 'chrome-extension://abc/popup.html', 'data:text/html,hi', 'about:blank', 'not a url']) {
      expect(ok(url, undefined).ok).toBe(false);
    }
  });

  it('leaves IP addresses to the browser', () => {
    expect(ok('https://192.168.1.10/', undefined)).toEqual({ ok: false, reason: 'ip address' });
    expect(ok('https://[::1]/', undefined)).toEqual({ ok: false, reason: 'ip address' });
    expect(ok('http://127.0.0.1:8080/', undefined).ok).toBe(false);
    expect(ok('https://example.com/', '10.0.0.1').ok).toBe(false);
    expect(isIpHost('10.0.0.1')).toBe(true);
    expect(isIpHost('[::1]')).toBe(true);
    expect(isIpHost('example.com')).toBe(false);
  });

  it('normalizes the requested rp id before comparing', () => {
    expect(ok('https://login.example.com/', 'EXAMPLE.com.')).toMatchObject({ ok: true, rpId: 'example.com' });
    expect(ok('https://login.example.com/', 'example.com:443')).toMatchObject({ ok: true, rpId: 'example.com' });
    expect(ok('https://login.example.com/', 'exam ple.com').ok).toBe(false);
  });

  it('takes the origin with its port from the page URL', () => {
    expect(ok('https://example.com:8443/x', undefined)).toMatchObject({ ok: true, origin: 'https://example.com:8443' });
  });
});

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
import { matchEntriesForUrl } from '../autofill-service.js';
import type { EntryResponse } from '@passbubble/shared-ts';

function makeEntry(url: string | undefined, matchPatterns?: string[]): EntryResponse {
  return {
    id: '1',
    owner_id: 'u1',
    type: 'password',
    name: 'Test',
    url,
    match_patterns: matchPatterns,
    encrypted_data: '',
    data_nonce: '',
    created_at: '',
    updated_at: '',
  };
}

describe('matchEntriesForUrl', () => {
  it('matches exact host', () => {
    const entries = [makeEntry('https://github.com')];
    expect(matchEntriesForUrl('https://github.com/login', entries)).toHaveLength(1);
  });

  it('strips www from page URL', () => {
    const entries = [makeEntry('https://github.com')];
    expect(matchEntriesForUrl('https://www.github.com/login', entries)).toHaveLength(1);
  });

  it('strips www from entry URL', () => {
    const entries = [makeEntry('https://www.github.com')];
    expect(matchEntriesForUrl('https://github.com/login', entries)).toHaveLength(1);
  });

  it('matches subdomain of entry host', () => {
    const entries = [makeEntry('https://example.com')];
    expect(matchEntriesForUrl('https://login.example.com', entries)).toHaveLength(1);
  });

  it('does not match a different domain', () => {
    const entries = [makeEntry('https://evil.com')];
    expect(matchEntriesForUrl('https://example.com', entries)).toHaveLength(0);
  });

  it('does not match evil.com against example.com via suffix trick', () => {
    // "notexample.com" must not match entry "example.com"
    const entries = [makeEntry('https://example.com')];
    expect(matchEntriesForUrl('https://notexample.com', entries)).toHaveLength(0);
  });

  it('excludes entries without URL', () => {
    const entries = [makeEntry(undefined)];
    expect(matchEntriesForUrl('https://example.com', entries)).toHaveLength(0);
  });

  it('returns empty array for an unparsable page URL', () => {
    const entries = [makeEntry('https://example.com')];
    expect(matchEntriesForUrl('not-a-url', entries)).toHaveLength(0);
  });

  it('matches multiple entries for the same host', () => {
    const entries = [makeEntry('https://example.com'), makeEntry('https://example.com/other')];
    expect(matchEntriesForUrl('https://example.com', entries)).toHaveLength(2);
  });

  describe('match_patterns (take precedence over url)', () => {
    it('matches a bare-host pattern', () => {
      const entries = [makeEntry('https://unrelated.com', ['github.com'])];
      expect(matchEntriesForUrl('https://github.com/login', entries)).toHaveLength(1);
    });

    it('matches a wildcard subdomain pattern', () => {
      const entries = [makeEntry(undefined, ['*.github.com'])];
      expect(matchEntriesForUrl('https://gist.github.com', entries)).toHaveLength(1);
      expect(matchEntriesForUrl('https://a.b.github.com', entries)).toHaveLength(1);
    });

    it('wildcard subdomain does not match the bare apex', () => {
      const entries = [makeEntry(undefined, ['*.github.com'])];
      expect(matchEntriesForUrl('https://github.com', entries)).toHaveLength(0);
    });

    it('ignores the port in a host:port pattern', () => {
      const entries = [makeEntry(undefined, ['127.0.0.1:32831'])];
      expect(matchEntriesForUrl('http://127.0.0.1:32831/typo3/login', entries)).toHaveLength(1);
    });

    it('normalises www. and full-URL patterns', () => {
      const entries = [makeEntry(undefined, ['http://www.a-trust.at'])];
      expect(matchEntriesForUrl('https://a-trust.at/', entries)).toHaveLength(1);
    });

    it('matches any of several patterns', () => {
      const entries = [makeEntry(undefined, ['foo.com', 'bar.com'])];
      expect(matchEntriesForUrl('https://bar.com', entries)).toHaveLength(1);
    });

    it('does not fall back to url when match_patterns is set and misses', () => {
      const entries = [makeEntry('https://github.com', ['gitlab.com'])];
      expect(matchEntriesForUrl('https://github.com', entries)).toHaveLength(0);
    });
  });
});

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

import { describe, expect, it } from 'vitest';
import {
  MAX_TAG_LENGTH,
  cleanTag,
  collectTags,
  containsTag,
  hasAllTags,
  normalizeTags,
  parseTagInput,
  parseTagQuery,
  putTags,
  sameTag,
  suggestTags,
  tagInputError,
  tagsOf,
} from '../tags.js';
import type { EntryData } from '../../types/vault.js';

describe('cleanTag', () => {
  it('trims, strips leading # and collapses internal whitespace', () => {
    expect(cleanTag('  #work  ')).toBe('work');
    expect(cleanTag('##client')).toBe('client');
    expect(cleanTag('# my \t  client ')).toBe('my client');
    expect(cleanTag('c#')).toBe('c#');
  });

  it('follows the cross-client order: trim, strip #, trim, collapse', () => {
    expect(cleanTag('# foo')).toBe('foo');
    expect(cleanTag('##a  b ')).toBe('a b');
    // Only the leading run of # goes; a # after a space stays.
    expect(cleanTag('# #work')).toBe('#work');
  });

  it('returns an empty string when nothing is left', () => {
    expect(cleanTag('')).toBe('');
    expect(cleanTag('   ')).toBe('');
    expect(cleanTag('###')).toBe('');
  });
});

describe('tagInputError', () => {
  it('accepts tags up to the limit and refuses longer ones', () => {
    expect(tagInputError('a'.repeat(MAX_TAG_LENGTH))).toBeNull();
    expect(tagInputError('a'.repeat(MAX_TAG_LENGTH + 1))).toMatch(/at most 40 characters/);
  });

  it('measures the cleaned tag, not the raw input', () => {
    expect(tagInputError(`  #${'a'.repeat(MAX_TAG_LENGTH)}  `)).toBeNull();
  });

  it('counts code points, so emoji count as one character', () => {
    expect(tagInputError('🔑'.repeat(MAX_TAG_LENGTH))).toBeNull();
    expect(tagInputError('🔑'.repeat(MAX_TAG_LENGTH + 1))).not.toBeNull();
  });

  it('ignores blank input', () => {
    expect(tagInputError('  ')).toBeNull();
  });
});

describe('normalizeTags', () => {
  it('cleans every tag and drops empty results', () => {
    expect(normalizeTags([' work ', '#', '', '#client'])).toEqual(['work', 'client']);
  });

  it('drops tags longer than the limit', () => {
    expect(normalizeTags(['a'.repeat(MAX_TAG_LENGTH + 1), 'ok'])).toEqual(['ok']);
  });

  it('de-duplicates case-insensitively, keeping the first spelling and the order', () => {
    expect(normalizeTags(['Work', 'client', 'WORK', '#work', 'Client'])).toEqual(['Work', 'client']);
  });

  it('ignores non-string items and non-array values', () => {
    expect(normalizeTags(['work', 42, null, { tag: 'x' }, ['y'], 'home'])).toEqual(['work', 'home']);
    expect(normalizeTags('work')).toEqual([]);
    expect(normalizeTags(undefined)).toEqual([]);
    expect(normalizeTags({ 0: 'work' })).toEqual([]);
  });
});

describe('tagsOf / putTags', () => {
  it('reads the normalised tags of decrypted entry data', () => {
    expect(tagsOf({ tags: ['#Work', 'work', 7] })).toEqual(['Work']);
    expect(tagsOf({})).toEqual([]);
    expect(tagsOf(null)).toEqual([]);
  });

  it('writes normalised tags and keeps every other key', () => {
    const data = { username: 'alice', extra_key: 1 } as EntryData & { extra_key: number };
    putTags(data, [' #work ', 'WORK', 'client']);
    expect(data).toEqual({ username: 'alice', extra_key: 1, tags: ['work', 'client'] });
  });

  it('removes the key when no tag is left', () => {
    const data: EntryData = { username: 'alice', tags: ['old'] };
    putTags(data, ['  ', '#']);
    expect(data).toEqual({ username: 'alice' });
    expect('tags' in data).toBe(false);
  });
});

describe('matching', () => {
  it('compares tags case-insensitively', () => {
    expect(sameTag('Work', 'wORK')).toBe(true);
    expect(sameTag('work', 'works')).toBe(false);
    expect(containsTag(['Work', 'client'], 'CLIENT')).toBe(true);
    expect(containsTag([], 'x')).toBe(false);
  });

  it('normalises the stored tags and the filter before comparing', () => {
    expect(sameTag('#Work ', 'work')).toBe(true);
    expect(containsTag(['  #Client'], 'client')).toBe(true);
    expect(hasAllTags(['#Work ', 'my   team'], ['work', '#My Team'])).toBe(true);
  });

  it('ignores blank filter tags', () => {
    expect(hasAllTags(['work'], ['', '#', 'work'])).toBe(true);
  });

  it('requires every filter tag (AND)', () => {
    expect(hasAllTags(['Work', 'Client'], ['work', 'client'])).toBe(true);
    expect(hasAllTags(['Work'], ['work', 'client'])).toBe(false);
    expect(hasAllTags([], ['work'])).toBe(false);
  });

  it('matches everything for an empty filter', () => {
    expect(hasAllTags([], [])).toBe(true);
    expect(hasAllTags(['work'], [])).toBe(true);
  });
});

describe('collectTags', () => {
  it('unions all tags with the first-seen spelling, sorted case-insensitively', () => {
    expect(collectTags([['work', 'Zeta'], ['WORK', 'alpha'], [], ['Beta', 'zeta']])).toEqual([
      'alpha',
      'Beta',
      'work',
      'Zeta',
    ]);
  });

  it('normalises the collected spelling and skips blank tags', () => {
    expect(collectTags([['#Work ', ' '], ['work']])).toEqual(['Work']);
  });

  it('sorts by plain comparison of the lowercased tag, not by locale', () => {
    // localeCompare would put "é" next to "e"; plain comparison puts it after "z".
    expect(collectTags([['zeta', 'éclair', 'Echo', '_x', '10']])).toEqual(['10', '_x', 'Echo', 'zeta', 'éclair']);
  });

  it('returns an empty list for a vault without tags', () => {
    expect(collectTags([])).toEqual([]);
    expect(collectTags([[], []])).toEqual([]);
  });
});

describe('suggestTags', () => {
  const vault = ['client', 'home', 'Office', 'work', 'workshop'];

  it('puts prefix matches before inner matches', () => {
    expect(suggestTags(vault, 'o')).toEqual(['Office', 'home', 'work', 'workshop']);
    expect(suggestTags(vault, 'h')).toEqual(['home', 'workshop']);
    expect(suggestTags(vault, 'WOR')).toEqual(['work', 'workshop']);
  });

  it('skips excluded tags (case-insensitive) and respects the limit', () => {
    expect(suggestTags(vault, 'work', { exclude: ['WORK'] })).toEqual(['workshop']);
    expect(suggestTags(vault, 'o', { limit: 1 })).toEqual(['Office']);
  });

  it('suggests nothing for blank input', () => {
    expect(suggestTags(vault, '  ')).toEqual([]);
    expect(suggestTags(vault, '#')).toEqual([]);
  });

  it('cleans the input first', () => {
    expect(suggestTags(vault, '#cli')).toEqual(['client']);
  });
});

describe('parseTagQuery', () => {
  it('splits # tokens from the text search', () => {
    expect(parseTagQuery('#work #client foo')).toEqual({ tags: ['work', 'client'], text: 'foo' });
    expect(parseTagQuery('foo #work bar')).toEqual({ tags: ['work'], text: 'foo bar' });
  });

  it('handles any whitespace and joins the text with single spaces', () => {
    expect(parseTagQuery('  github\t #Work\n  login  ')).toEqual({ tags: ['Work'], text: 'github login' });
  });

  it('ignores a bare # and de-duplicates the tags', () => {
    expect(parseTagQuery('# foo')).toEqual({ tags: [], text: 'foo' });
    expect(parseTagQuery('#work ##WORK')).toEqual({ tags: ['work'], text: '' });
  });

  it('keeps a # inside a word as text', () => {
    expect(parseTagQuery('c# notes')).toEqual({ tags: [], text: 'c# notes' });
  });

  it('returns nothing for an empty query', () => {
    expect(parseTagQuery('')).toEqual({ tags: [], text: '' });
    expect(parseTagQuery('   ')).toEqual({ tags: [], text: '' });
  });
});

describe('parseTagInput', () => {
  it('splits on commas and normalises', () => {
    expect(parseTagInput(' #work, client ,, Work,  my   team ')).toEqual({
      tags: ['work', 'client', 'my team'],
      error: null,
    });
  });

  it('returns no tags for blank input', () => {
    expect(parseTagInput('')).toEqual({ tags: [], error: null });
    expect(parseTagInput(' , ')).toEqual({ tags: [], error: null });
  });

  it('refuses a tag that is too long instead of dropping it', () => {
    const result = parseTagInput(`work, ${'x'.repeat(MAX_TAG_LENGTH + 1)}`);
    expect(result.tags).toEqual([]);
    expect(result.error).toMatch(/at most 40 characters/);
  });
});

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

import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/util/entry_tags.dart';

void main() {
  group('cleanTag', () {
    test('trims, strips leading # and collapses whitespace', () {
      expect(cleanTag('  work  '), 'work');
      expect(cleanTag('#work'), 'work');
      expect(cleanTag('###work'), 'work');
      expect(cleanTag('# work'), 'work');
      expect(cleanTag('# foo'), 'foo');
      expect(cleanTag('##a  b '), 'a b');
      expect(cleanTag('client \t  a\nb'), 'client a b');
      expect(cleanTag('c#'), 'c#', reason: 'only leading # are stripped');
      expect(cleanTag('#'), '');
      expect(cleanTag('   '), '');
    });
  });

  group('tagInputError', () {
    test('refuses tags longer than 40 characters', () {
      expect(tagInputError('a' * 40), isNull);
      expect(tagInputError('a' * 41), isNotNull);
      // Measured after cleaning, so the # and padding do not count.
      expect(tagInputError('  #${'a' * 40}  '), isNull);
      // Counted in code points, not UTF-16 units.
      expect(tagInputError('😀' * 40), isNull);
      expect(tagInputError(''), isNull);
    });
  });

  group('joinTags / splitTags', () {
    test('join with a comma and a space, normalised', () {
      expect(joinTags(['Work', ' #client  a', 'WORK']), 'Work, client a');
      expect(joinTags(const []), '');
    });

    test('split on commas like the tag input, normalised', () {
      expect(splitTags('Work, client   a,#home,,work'), ['Work', 'client a', 'home']);
      expect(splitTags(''), isEmpty);
      expect(splitTags(' , '), isEmpty);
      expect(splitTags('a;b'), ['a;b'], reason: 'only the comma separates');
    });

    test('a joined list splits back into itself', () {
      const tags = ['Work', 'client a', 'c#'];
      expect(splitTags(joinTags(tags)), tags);
    });
  });

  group('normalizeTags', () {
    test('cleans, drops empty and over-long, de-dupes keeping the first spelling', () {
      expect(
        normalizeTags([' Work', '#client', 'work', '', '#', 'a' * 41, 'CLIENT', 'home']),
        ['Work', 'client', 'home'],
      );
    });

    test('keeps the order and ignores non-string items', () {
      expect(normalizeTags(['b', 1, null, 'a', true, {'x': 1}, 'c']), ['b', 'a', 'c']);
    });
  });

  group('tagsOf / putTags', () {
    test('reads normalised tags, empty for a missing or malformed value', () {
      expect(tagsOf({'tags': ['#a', 'A', 'b', 3]}), ['a', 'b']);
      expect(tagsOf({}), isEmpty);
      expect(tagsOf({'tags': 'work'}), isEmpty);
      expect(tagsOf({'tags': null}), isEmpty);
    });

    test('writes normalised tags and omits the key when none are left', () {
      final data = <String, dynamic>{'tags': ['old'], 'notes': 'n'};
      putTags(data, ['#x', 'X', ' y ']);
      expect(data['tags'], ['x', 'y']);
      putTags(data, ['', '#']);
      expect(data.containsKey('tags'), isFalse);
      expect(data['notes'], 'n');
    });
  });

  group('matching', () {
    test('hasAllTags is a case-insensitive AND', () {
      const tags = ['Work', 'Client'];
      expect(hasAllTags(tags, []), isTrue);
      expect(hasAllTags(tags, ['work']), isTrue);
      expect(hasAllTags(tags, ['WORK', 'client']), isTrue);
      expect(hasAllTags(tags, ['work', 'home']), isFalse);
      expect(hasAllTags(const [], ['work']), isFalse);
    });

    test('stored tags are normalised before matching', () {
      expect(hasAllTags(['#Work ', ' client   a'], ['work', 'CLIENT A']), isTrue);
      expect(sameTag('#Work ', 'work'), isTrue);
      expect(containsTag(['##home'], 'Home'), isTrue);
    });

    test('containsTag and sameTag ignore case', () {
      expect(sameTag('Work', 'wORK'), isTrue);
      expect(containsTag(['a', 'Work'], 'work'), isTrue);
      expect(containsTag(['a'], 'work'), isFalse);
    });
  });

  group('collectTags', () {
    test('unions with the first-seen spelling, sorted case-insensitively', () {
      expect(
        collectTags([
          ['work', 'Zeta'],
          ['Work', 'alpha'],
          [],
          ['ALPHA', 'beta'],
        ]),
        ['alpha', 'beta', 'work', 'Zeta'],
      );
    });

    test('normalises its input and sorts by code point, not UTF-16 unit', () {
      expect(collectTags([['#Work ', 'work']]), ['Work']);
      // U+FF01 (one UTF-16 unit, 0xFF01) sorts before U+1F600 (surrogates
      // 0xD83D…) by code point; a UTF-16 compare would flip them.
      expect(collectTags([['😀', '！', 'b', 'B', 'a']]), ['a', 'b', '！', '😀']);
    });
  });

  group('suggestTags', () {
    const vault = ['alpha', 'bank', 'Work', 'network', 'workshop'];

    test('prefix matches first, then inner matches, case-insensitive', () {
      expect(suggestTags(vault, 'WOR'), ['Work', 'workshop', 'network']);
      expect(suggestTags(vault, '#wor'), ['Work', 'workshop', 'network']);
    });

    test('skips tags already on the entry and nothing for blank input', () {
      expect(suggestTags(vault, 'wor', exclude: ['work']), ['workshop', 'network']);
      expect(suggestTags(vault, '  '), isEmpty);
      expect(suggestTags(vault, 'a', limit: 2), ['alpha', 'bank']);
    });
  });

  group('parseTagQuery', () {
    test('splits #tag tokens from the text', () {
      final q = parseTagQuery('#work #client foo');
      expect(q.tags, ['work', 'client']);
      expect(q.text, 'foo');
    });

    test('tokens may appear anywhere; text words are re-joined by one space', () {
      final q = parseTagQuery('  foo   #Work bar #work ');
      expect(q.tags, ['Work']);
      expect(q.text, 'foo bar');
    });

    test('a bare # and an empty query yield nothing', () {
      expect(parseTagQuery('#').isEmpty, isTrue);
      expect(parseTagQuery('   ').isEmpty, isTrue);
      expect(parseTagQuery('').isEmpty, isTrue);
      expect(parseTagQuery('##x').tags, ['x']);
    });

    test('plain text has no tags', () {
      final q = parseTagQuery('mail.example.com');
      expect(q.tags, isEmpty);
      expect(q.text, 'mail.example.com');
      expect(q.isEmpty, isFalse);
    });
  });
}

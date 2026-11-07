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

// Entry tags: normalisation, matching and `#tag` search parsing. Tags live in
// the E2E-encrypted entry JSON under [tagsKey] as a list of strings, so the
// server never sees them. Every client (app, extension, CLI) applies the same
// rules; keep this file in step with them.

/// Key of the tag list inside the decrypted entry JSON. Omitted when an entry
/// has no tags.
const tagsKey = 'tags';

/// Longest tag accepted, in Unicode code points. Longer tags are refused by
/// the input and dropped on save.
const maxTagLength = 40;

final _leadingHashes = RegExp(r'^#+');
final _whitespaceRun = RegExp(r'\s+');

/// One raw tag in canonical form, in this order: trim, strip leading `#`
/// characters, trim again, collapse internal whitespace runs to one space
/// (`"# foo"` → `"foo"`, `"##a  b "` → `"a b"`). Empty when nothing is left.
/// Does not check the length (see [tagInputError]).
String cleanTag(String raw) => raw
    .trim()
    .replaceFirst(_leadingHashes, '')
    .trim()
    .replaceAll(_whitespaceRun, ' ');

/// What tags are compared by: the cleaned tag, lowercased. Stored tags are
/// cleaned here too, so `"#Work "` matches `"work"`.
String _matchKey(String tag) => cleanTag(tag).toLowerCase();

/// Orders strings by Unicode code point (not UTF-16 unit, which differs for
/// characters outside the BMP), like the other clients.
int _compareCodePoints(String a, String b) {
  final ia = a.runes.iterator, ib = b.runes.iterator;
  while (true) {
    final hasA = ia.moveNext(), hasB = ib.moveNext();
    if (!hasA || !hasB) return hasA ? 1 : (hasB ? -1 : 0);
    if (ia.current != ib.current) return ia.current - ib.current;
  }
}

/// Why [raw] cannot be added as a tag, or null when it can (or is blank, which
/// the input simply ignores).
String? tagInputError(String raw) {
  if (cleanTag(raw).runes.length > maxTagLength) {
    return 'Tags can be at most $maxTagLength characters';
  }
  return null;
}

/// The tag list as it is saved: every item cleaned, empty and over-long ones
/// dropped, duplicates (case-insensitive) removed keeping the first spelling,
/// order kept. Non-string items are ignored, so this also reads a stored
/// `tags` value of unknown shape.
List<String> normalizeTags(Iterable<Object?> raw) {
  final seen = <String>{};
  final out = <String>[];
  for (final item in raw) {
    if (item is! String) continue;
    final tag = cleanTag(item);
    if (tag.isEmpty || tag.runes.length > maxTagLength) continue;
    if (seen.add(_matchKey(tag))) out.add(tag);
  }
  return out;
}

/// The tags of a decrypted entry, normalised; empty when there are none or the
/// stored value is not a list.
List<String> tagsOf(Map<String, dynamic> data) {
  final raw = data[tagsKey];
  return raw is List ? normalizeTags(raw) : const [];
}

/// Writes [tags] into [data] in normalised form, or removes the key when no
/// tag is left.
void putTags(Map<String, dynamic> data, Iterable<String> tags) {
  final normalized = normalizeTags(tags);
  if (normalized.isEmpty) {
    data.remove(tagsKey);
  } else {
    data[tagsKey] = normalized;
  }
}

/// The tags in text form, for formats that keep them in one cell or field.
String joinTags(Iterable<String> tags) => normalizeTags(tags).join(', ');

/// The tags in a text cell or field, split on commas like the app's tag input.
List<String> splitTags(String text) => normalizeTags(text.split(','));

/// Whether two tags are the same tag (cleaned, case-insensitive).
bool sameTag(String a, String b) => _matchKey(a) == _matchKey(b);

/// Whether [tags] contains [tag] (case-insensitive).
bool containsTag(Iterable<String> tags, String tag) =>
    tags.any((t) => sameTag(t, tag));

/// Whether an entry carrying [entryTags] has every tag in [required] (AND,
/// cleaned, case-insensitive). True when [required] is empty.
bool hasAllTags(Iterable<String> entryTags, Iterable<String> required) {
  final have = {for (final t in entryTags) _matchKey(t)};
  return required.every((t) => have.contains(_matchKey(t)));
}

/// Union of the tags of all entries: first-seen spelling, sorted by the
/// lowercased tag, code point by code point (no locale collation).
List<String> collectTags(Iterable<Iterable<String>> perEntry) {
  final byKey = <String, String>{};
  for (final tags in perEntry) {
    for (final t in normalizeTags(tags)) {
      byKey.putIfAbsent(_matchKey(t), () => t);
    }
  }
  return byKey.values.toList()
    ..sort((a, b) => _compareCodePoints(a.toLowerCase(), b.toLowerCase()));
}

/// Vault tags matching what the user is typing ([input], case-insensitive),
/// prefix matches first, without those already in [exclude].
List<String> suggestTags(
  Iterable<String> vaultTags,
  String input, {
  Iterable<String> exclude = const [],
  int limit = 8,
}) {
  final q = _matchKey(input);
  if (q.isEmpty) return const [];
  final prefix = <String>[];
  final inner = <String>[];
  for (final t in vaultTags) {
    if (containsTag(exclude, t)) continue;
    final lower = t.toLowerCase();
    if (lower.startsWith(q)) {
      prefix.add(t);
    } else if (lower.contains(q)) {
      inner.add(t);
    }
  }
  return [...prefix, ...inner].take(limit).toList();
}

/// A search box query split into its parts.
class TagQuery {
  /// Tags from `#tag` tokens, normalised (an entry must carry all of them).
  final List<String> tags;

  /// The remaining words for the normal text search, joined by single spaces.
  final String text;

  const TagQuery({this.tags = const [], this.text = ''});

  bool get isEmpty => tags.isEmpty && text.isEmpty;
}

/// Splits a search query on whitespace: tokens starting with `#` are tag
/// filters, everything else is text. `#work #client foo` = tags work AND
/// client plus the text "foo". A bare `#` is ignored.
TagQuery parseTagQuery(String query) {
  final tags = <String>[];
  final words = <String>[];
  for (final token in query.split(_whitespaceRun)) {
    if (token.isEmpty) continue;
    if (token.startsWith('#')) {
      tags.add(token);
    } else {
      words.add(token);
    }
  }
  return TagQuery(tags: normalizeTags(tags), text: words.join(' '));
}

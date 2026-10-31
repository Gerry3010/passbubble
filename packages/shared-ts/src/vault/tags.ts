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
// the E2E-encrypted entry JSON under TAGS_KEY as an array of strings, so the
// server never sees them. Every client (app, extension, CLI) applies the same
// rules; keep this file in step with flutter_app/lib/core/util/entry_tags.dart.

import type { EntryData } from '../types/vault.js';

/** Key of the tag list inside the decrypted entry JSON. Omitted when an entry
 * has no tags. */
export const TAGS_KEY = 'tags';

/** Longest tag accepted, in Unicode code points. Longer tags are refused by
 * the input and dropped on save. */
export const MAX_TAG_LENGTH = 40;

const LEADING_HASHES = /^#+/;
const WHITESPACE_RUN = /\s+/g;

// Tag length in Unicode code points (not UTF-16 units), like the other clients.
function codePoints(s: string): number {
  return [...s].length;
}

/** One raw tag in canonical form: trimmed, leading `#` characters stripped,
 * trimmed again, internal whitespace runs collapsed to one space (this order,
 * in every client). Empty when nothing is left. Does not check the length
 * (see tagInputError). */
export function cleanTag(raw: string): string {
  return raw.trim().replace(LEADING_HASHES, '').trim().replace(WHITESPACE_RUN, ' ');
}

// Comparison key: matching normalises both sides, so a stored "#Work " is the
// same tag as a filter "work".
function tagKey(tag: string): string {
  return cleanTag(tag).toLowerCase();
}

/** Why `raw` cannot be added as a tag, or null when it can (or is blank, which
 * the input simply ignores). */
export function tagInputError(raw: string): string | null {
  if (codePoints(cleanTag(raw)) > MAX_TAG_LENGTH) {
    return `Tags can be at most ${MAX_TAG_LENGTH} characters`;
  }
  return null;
}

/** The tag list as it is saved: every item cleaned, empty and over-long ones
 * dropped, duplicates (case-insensitive) removed keeping the first spelling,
 * order kept. Non-string items are ignored and a non-array yields [], so this
 * also reads a stored `tags` value of unknown shape. */
export function normalizeTags(raw: unknown): string[] {
  if (!Array.isArray(raw)) return [];
  const seen = new Set<string>();
  const out: string[] = [];
  for (const item of raw) {
    if (typeof item !== 'string') continue;
    const tag = cleanTag(item);
    if (!tag || codePoints(tag) > MAX_TAG_LENGTH) continue;
    const key = tagKey(tag);
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(tag);
  }
  return out;
}

/** The tags of a decrypted entry, normalised; empty when there are none or the
 * stored value is not an array. */
export function tagsOf(data: { tags?: unknown } | null | undefined): string[] {
  return normalizeTags(data?.tags);
}

/** Writes `tags` into `data` in normalised form, or removes the key when no
 * tag is left. Every other key of `data` is left untouched. */
export function putTags(data: EntryData, tags: unknown): void {
  const normalized = normalizeTags(tags);
  if (normalized.length === 0) delete data.tags;
  else data.tags = normalized;
}

/** Whether two tags are the same tag (normalised, case-insensitive). */
export function sameTag(a: string, b: string): boolean {
  return tagKey(a) === tagKey(b);
}

/** Whether `tags` contains `tag` (normalised, case-insensitive). */
export function containsTag(tags: Iterable<string>, tag: string): boolean {
  for (const t of tags) if (sameTag(t, tag)) return true;
  return false;
}

/** Whether an entry carrying `entryTags` has every tag in `required` (AND,
 * normalised, case-insensitive). True when `required` is empty; blank filter
 * tags are ignored. */
export function hasAllTags(entryTags: Iterable<string>, required: Iterable<string>): boolean {
  const have = new Set<string>();
  for (const t of entryTags) have.add(tagKey(t));
  for (const t of required) {
    const key = tagKey(t);
    if (key && !have.has(key)) return false;
  }
  return true;
}

/** Union of the tags of all entries: first-seen (normalised) spelling, sorted
 * by the lowercased tag with a plain `<` comparison (no locale collation). */
export function collectTags(perEntry: Iterable<Iterable<string>>): string[] {
  const byKey = new Map<string, string>();
  for (const tags of perEntry) {
    for (const raw of tags) {
      const tag = cleanTag(raw);
      const key = tag.toLowerCase();
      if (key && !byKey.has(key)) byKey.set(key, tag);
    }
  }
  return [...byKey.entries()].sort(([ka], [kb]) => (ka < kb ? -1 : ka > kb ? 1 : 0)).map(([, tag]) => tag);
}

/** Vault tags matching what the user is typing (`input`, case-insensitive),
 * prefix matches first, without those already in `exclude`. */
export function suggestTags(
  vaultTags: Iterable<string>,
  input: string,
  { exclude = [], limit = 8 }: { exclude?: Iterable<string>; limit?: number } = {},
): string[] {
  const q = tagKey(input);
  if (!q) return [];
  const excluded = [...exclude];
  const prefix: string[] = [];
  const inner: string[] = [];
  for (const t of vaultTags) {
    if (containsTag(excluded, t)) continue;
    const key = tagKey(t);
    if (key.startsWith(q)) prefix.push(t);
    else if (key.includes(q)) inner.push(t);
  }
  return [...prefix, ...inner].slice(0, limit);
}

/** A search box query split into its parts. */
export interface TagQuery {
  /** Tags from `#tag` tokens, normalised (an entry must carry all of them). */
  tags: string[];
  /** The remaining words for the normal text search, joined by single spaces. */
  text: string;
}

/** Splits a search query on whitespace: tokens starting with `#` are tag
 * filters, everything else is text. `#work #client foo` = tags work AND
 * client plus the text "foo". A bare `#` is ignored. */
export function parseTagQuery(query: string): TagQuery {
  const tags: string[] = [];
  const words: string[] = [];
  for (const token of query.split(WHITESPACE_RUN)) {
    if (!token) continue;
    if (token.startsWith('#')) tags.push(token);
    else words.push(token);
  }
  return { tags: normalizeTags(tags), text: words.join(' ') };
}

/** Reads a comma-separated tag input. `error` is set when a tag is too long —
 * the input refuses it instead of silently dropping it; `tags` is the
 * normalised list to save otherwise. */
export function parseTagInput(input: string): { tags: string[]; error: string | null } {
  const parts = input.split(',');
  for (const part of parts) {
    const error = tagInputError(part);
    if (error) return { tags: [], error };
  }
  return { tags: normalizeTags(parts), error: null };
}

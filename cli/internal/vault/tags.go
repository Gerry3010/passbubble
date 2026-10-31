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

package vault

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Entry tags live inside the encrypted entry JSON (EntryData.Tags), so the
// server never sees them. The app, the extension and the CLI apply the same
// rules: normalise on save, match case-insensitively, AND for several tags.

// MaxTagLength is the longest tag, in characters (code points), a client saves.
const MaxTagLength = 40

// NormalizeTag cleans a single tag: surrounding whitespace and leading '#'
// characters are removed and inner whitespace runs collapse to one space.
// The result is "" when nothing is left.
func NormalizeTag(s string) string {
	s = strings.TrimLeft(strings.TrimSpace(s), "#")
	return strings.Join(strings.Fields(s), " ")
}

// NormalizeTags prepares a tag list for saving: every tag is normalised,
// empty results are dropped and case-insensitive duplicates removed (the
// first spelling wins). The order stays as given. A tag longer than
// MaxTagLength is an error.
func NormalizeTags(in []string) ([]string, error) {
	var out []string
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		t := NormalizeTag(raw)
		if t == "" {
			continue
		}
		if n := utf8.RuneCountInString(t); n > MaxTagLength {
			return nil, fmt.Errorf("tag %q is too long (%d characters, at most %d)", t, n, MaxTagLength)
		}
		k := strings.ToLower(t)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out, nil
}

// tagKey is the comparison form of a tag. Normalising first keeps matching
// robust against tags another client saved without normalising.
func tagKey(t string) string {
	return strings.ToLower(NormalizeTag(t))
}

// HasTags reports whether tags contains every tag in want, ignoring case.
// An empty want matches everything.
func HasTags(tags, want []string) bool {
	have := make(map[string]bool, len(tags))
	for _, t := range tags {
		have[tagKey(t)] = true
	}
	for _, w := range want {
		if !have[tagKey(w)] {
			return false
		}
	}
	return true
}

// RemoveTags returns tags without those in remove, ignoring case. The input
// slice is not modified.
func RemoveTags(tags, remove []string) []string {
	drop := make(map[string]bool, len(remove))
	for _, r := range remove {
		drop[tagKey(r)] = true
	}
	var out []string
	for _, t := range tags {
		if !drop[tagKey(t)] {
			out = append(out, t)
		}
	}
	return out
}

// EditTags applies a tag edit to current: clearAll drops every tag, then
// remove and add apply in that order. The result is normalised; existing
// spellings win over added duplicates.
func EditTags(current []string, clearAll bool, remove, add []string) ([]string, error) {
	tags := current
	if clearAll {
		tags = nil
	}
	tags = RemoveTags(tags, remove)
	return NormalizeTags(append(tags, add...))
}

// TagCount is one tag of the vault with the number of entries carrying it.
type TagCount struct {
	Name  string
	Count int
}

// CountTags builds the vault's tag list from the tags of each entry: the
// union of all tags in the spelling seen first, sorted case-insensitively,
// each with the number of entries that carry it.
func CountTags(perEntry [][]string) []TagCount {
	index := map[string]int{} // tag key → position in out
	var out []TagCount
	for _, tags := range perEntry {
		counted := map[string]bool{} // a tag listed twice on one entry counts once
		for _, t := range tags {
			k := tagKey(t)
			if k == "" || counted[k] {
				continue
			}
			counted[k] = true
			if i, ok := index[k]; ok {
				out[i].Count++
				continue
			}
			index[k] = len(out)
			out = append(out, TagCount{Name: NormalizeTag(t), Count: 1})
		}
	}
	slices.SortFunc(out, func(a, b TagCount) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out
}

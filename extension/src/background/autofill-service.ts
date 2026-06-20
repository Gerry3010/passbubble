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

import type { EntryResponse } from '@passbubble/shared-ts';
import { normaliseHost, patternMatchLevel } from '../shared/utils.js';

// Returns entries matching the page, best first. Exact-host matches rank above
// subdomain/wildcard matches; entries that only share the registrable domain
// (e.g. another host of the same site) are a fallback, used ONLY when nothing
// stronger matched — so a precise host is never buried under loose matches.
export function matchEntriesForUrl(url: string, entries: EntryResponse[]): EntryResponse[] {
  const pageHost = normaliseHost(url);
  if (!pageHost) return [];

  const scored: { entry: EntryResponse; level: number }[] = [];
  for (const e of entries) {
    // Dedicated match patterns take precedence; otherwise fall back to the
    // entry's display URL so entries created before this feature still match.
    const patterns = e.match_patterns?.length ? e.match_patterns : e.url ? [e.url] : [];
    let level = -1;
    for (const p of patterns) level = Math.max(level, patternMatchLevel(pageHost, p));
    if (level >= 0) scored.push({ entry: e, level });
  }

  // Prefer host/subdomain/wildcard matches (level ≥ 1); only when there are none
  // do we surface same-domain (level 0) entries.
  const strong = scored.filter((s) => s.level >= 1);
  const chosen = strong.length ? strong : scored;
  chosen.sort((a, b) => b.level - a.level);
  return chosen.map((s) => s.entry);
}

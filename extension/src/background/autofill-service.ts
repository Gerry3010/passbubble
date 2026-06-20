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
import { normaliseHost, patternMatchesHost } from '../shared/utils.js';

export function matchEntriesForUrl(url: string, entries: EntryResponse[]): EntryResponse[] {
  const pageHost = normaliseHost(url);
  if (!pageHost) return [];
  return entries.filter((e) => {
    // Dedicated match patterns take precedence; otherwise fall back to the
    // entry's display URL so entries created before this feature still match.
    const patterns = e.match_patterns?.length ? e.match_patterns : e.url ? [e.url] : [];
    return patterns.some((p) => patternMatchesHost(pageHost, p));
  });
}

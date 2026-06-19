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

import 'package:web/web.dart' as web;

/// Strips the query string from the current browser URL via the History API,
/// without triggering a navigation. With hash routing the query lives inside
/// the fragment (`#/register?token=...`); a path-strategy query in
/// `location.search` is dropped by simply not re-appending it.
void stripUrlQuery() {
  final loc = web.window.location;
  var hash = loc.hash;
  final q = hash.indexOf('?');
  if (q >= 0) hash = hash.substring(0, q);
  // Nothing to strip if there's neither a fragment query nor a search query.
  if (q < 0 && loc.search.isEmpty) return;
  web.window.history.replaceState(null, '', '${loc.pathname}$hash');
}

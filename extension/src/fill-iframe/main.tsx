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

import { createRoot } from 'react-dom/client';
import { FillSuggestion } from './FillSuggestion.js';

// Flutter web fields lose a fill once a press here moves the focus into this
// iframe (see content/flutter.ts). When the embedder asks for it, cancel every
// press's default action before React mounts, so there is no window in which a
// click can still take the focus.
if (new URLSearchParams(location.search).has('keepFocus')) {
  document.addEventListener('mousedown', (e) => e.preventDefault(), true);
}

const root = document.getElementById('root')!;
createRoot(root).render(<FillSuggestion />);

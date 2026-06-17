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

import { vi, beforeEach } from 'vitest';

// Make all HTML elements "visible" in jsdom (offsetParent is always null in jsdom)
Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
  get() {
    return document.body;
  },
});

// Mock the Clipboard API (not available in jsdom)
Object.defineProperty(navigator, 'clipboard', {
  value: {
    writeText: vi.fn().mockResolvedValue(undefined),
    readText: vi.fn().mockResolvedValue(''),
  },
  writable: true,
});

beforeEach(() => {
  vi.clearAllMocks();
  // Clean up any DOM changes between tests
  document.body.innerHTML = '';
});

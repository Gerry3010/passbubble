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

import { describe, it, expect } from 'vitest';
import { countsAsActivity } from '../activity.js';
import { MessageType } from '../../shared/constants.js';

const ORIGIN = 'chrome-extension://abc/';
const POPUP = { url: `${ORIGIN}popup/index.html` };
const PAGE = { url: 'https://example.com/login', tab: { id: 1 } };

describe('countsAsActivity', () => {
  it('counts the popup asking for the session', () => {
    expect(countsAsActivity(MessageType.GET_SESSION, POPUP, ORIGIN)).toBe(true);
  });

  it('does not let a page keep the vault unlocked by focusing a login field', () => {
    expect(countsAsActivity(MessageType.GET_SESSION, PAGE, ORIGIN)).toBe(false);
    expect(countsAsActivity(MessageType.GET_SESSION, {}, ORIGIN)).toBe(false);
  });

  it('counts a fill the user picked in the box', () => {
    expect(countsAsActivity(MessageType.FILL_ENTRY, PAGE, ORIGIN)).toBe(true);
  });

  it('ignores passive traffic', () => {
    expect(countsAsActivity(MessageType.OFFER_SAVE, PAGE, ORIGIN)).toBe(false);
  });
});

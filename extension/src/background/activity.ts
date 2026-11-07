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

import { MessageType } from '../shared/constants.js';

// Messages that count as "the user is actively using the vault" — receiving one
// restarts the idle auto-lock countdown. Passive background traffic (autofill
// URL matching, login-frame reports, save offers) is deliberately excluded so
// that merely browsing does not keep the vault unlocked forever.
const ACTIVITY_MESSAGES = new Set<string>([
  MessageType.GET_SESSION,
  MessageType.UNLOCK,
  MessageType.UNLOCK_WITH_PIN,
  MessageType.SEARCH_ENTRIES,
  MessageType.LIST_FOLDERS,
  MessageType.GET_ENTRY,
  MessageType.CREATE_ENTRY,
  MessageType.UPDATE_ENTRY,
  MessageType.EDIT_ENTRY,
  MessageType.DELETE_ENTRY,
  MessageType.GENERATE,
  MessageType.FILL_ENTRY,
  MessageType.GET_FILL_HOST,
]);

/** Whether a message restarts the idle countdown. The content script asks for
 * the session whenever a login field gets the focus, which a page can trigger
 * by itself, so only the extension's own pages count there. */
export function countsAsActivity(type: string, sender: { url?: string }, extensionOrigin: string): boolean {
  if (!ACTIVITY_MESSAGES.has(type)) return false;
  return type !== MessageType.GET_SESSION || !!sender.url?.startsWith(extensionOrigin);
}

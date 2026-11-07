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

// What the popup's edit form asks the background to change (EDIT_ENTRY). The
// form sends only what the user changed; the background applies it to the entry
// as it is on the server at save time. Keys the form does not know, and keys
// another device changed in the meantime, therefore stay as they are.

import { dropStrayTotpSecret, putTags } from '@passbubble/shared-ts';
import type { EntryData } from '@passbubble/shared-ts';

export interface EntryEdit {
  /** Only when changed. */
  name?: string;
  /** Only when changed; '' clears the URL. */
  url?: string;
  /** Changed keys of the entry data. '' or null removes the key. */
  fields?: Record<string, string | number | null>;
  /** The complete new custom field list, only when it changed. */
  customFields?: unknown[];
  /** The complete new tag list, only when it changed. */
  tags?: string[];
}

export function applyEntryEdit(data: EntryData, edit: EntryEdit): void {
  const target = data as Record<string, unknown>;
  // Compared against the stored password, so this comes before any change to it:
  // a stray copy left behind would no longer equal a new password and would then
  // pass for a real secret.
  dropStrayTotpSecret(data);
  for (const [key, value] of Object.entries(edit.fields ?? {})) {
    if (value === '' || value === null) delete target[key];
    else target[key] = value;
  }
  if (Array.isArray(edit.customFields)) {
    if (edit.customFields.length > 0) target.custom_fields = edit.customFields;
    else delete target.custom_fields;
  }
  if (edit.tags) putTags(data, edit.tags);
  // The app also stores name and url inside the data, and its edit form prefers
  // those copies over the entry's metadata. Without this, editing the entry in
  // the app later would undo a rename made here.
  if (edit.name !== undefined && 'name' in target) target.name = edit.name;
  if (edit.url !== undefined && 'url' in target) {
    if (edit.url) target.url = edit.url;
    else delete target.url;
  }
}

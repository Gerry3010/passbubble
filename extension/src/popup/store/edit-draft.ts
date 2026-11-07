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

// Unsaved input of the edit form. When the vault locks (a Save that came back
// locked, the idle timeout, the lock button) the popup replaces the vault panel
// with the unlock screen, which unmounts the form. So does switching tabs. The
// draft brings the input back when the panel returns.
//
// Deliberately only in this module's memory and never in browser storage: it can
// hold plaintext secrets, and a locked vault must not leave those on disk or in
// storage.session. It is gone with the popup window.

import type { EntryResponse } from '@passbubble/shared-ts';
import { useSessionStore } from './session.js';

export interface EditDraft<Model = unknown> {
  entry: EntryResponse;
  model: Model;
}

let draft: { owner: string | undefined; value: EditDraft } | null = null;

/** The current draft, unless it was made under another account. */
export function getEditDraft(): EditDraft | null {
  if (draft && draft.owner !== useSessionStore.getState().userEmail) draft = null;
  return draft?.value ?? null;
}

export function setEditDraft(value: EditDraft): void {
  draft = { owner: useSessionStore.getState().userEmail, value };
}

export function clearEditDraft(): void {
  draft = null;
}

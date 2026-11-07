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

import { useMemo, useState } from 'react';
import { collectTags } from '@passbubble/shared-ts';
import type { EntryResponse } from '@passbubble/shared-ts';
import { EntryList } from './EntryList.js';
import { EntryDetail } from './EntryDetail.js';
import { CreateEntryForm } from './CreateEntryForm.js';
import { EditEntryForm } from './EditEntryForm.js';
import { useEntriesStore } from '../store/entries.js';
import { getEditDraft } from '../store/edit-draft.js';

type View =
  | { kind: 'list' }
  | { kind: 'detail'; entry: EntryResponse }
  | { kind: 'create' }
  | { kind: 'edit'; entry: EntryResponse };

/** The vault tab: switches between the entry list, a detail view, and the
 * create and edit forms. Unsaved edits reopen the edit form when the panel comes
 * back, e.g. after the vault locked in the middle of them. */
export function VaultPanel() {
  const [view, setView] = useState<View>(() => {
    const draft = getEditDraft();
    return draft ? { kind: 'edit', entry: draft.entry } : { kind: 'list' };
  });
  const load = useEntriesStore((s) => s.load);
  const entryTags = useEntriesStore((s) => s.entryTags);
  const vaultTags = useMemo(() => collectTags(Object.values(entryTags)), [entryTags]);

  if (view.kind === 'detail') {
    const { entry } = view;
    return (
      <EntryDetail
        entry={entry}
        onBack={() => setView({ kind: 'list' })}
        onEdit={() => setView({ kind: 'edit', entry })}
      />
    );
  }
  if (view.kind === 'edit') {
    const { entry } = view;
    return (
      <EditEntryForm
        entry={entry}
        vaultTags={vaultTags}
        onCancel={() => setView({ kind: 'detail', entry })}
        onSaved={(meta) => {
          void load();
          setView({ kind: 'detail', entry: { ...entry, ...meta } });
        }}
      />
    );
  }
  if (view.kind === 'create') {
    return (
      <CreateEntryForm
        vaultTags={vaultTags}
        onCancel={() => setView({ kind: 'list' })}
        onCreated={() => {
          void load();
          setView({ kind: 'list' });
        }}
      />
    );
  }
  return (
    <EntryList
      onSelect={(entry) => setView({ kind: 'detail', entry })}
      onCreate={() => setView({ kind: 'create' })}
    />
  );
}

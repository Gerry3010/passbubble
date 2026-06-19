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

import { useState } from 'react';
import type { EntryResponse } from '@passbubble/shared-ts';
import { EntryList } from './EntryList.js';
import { EntryDetail } from './EntryDetail.js';
import { CreateEntryForm } from './CreateEntryForm.js';
import { useEntriesStore } from '../store/entries.js';

type View = { kind: 'list' } | { kind: 'detail'; entry: EntryResponse } | { kind: 'create' };

/** The vault tab: switches between the entry list, a detail view, and the
 * create form. */
export function VaultPanel() {
  const [view, setView] = useState<View>({ kind: 'list' });
  const search = useEntriesStore((s) => s.search);

  if (view.kind === 'detail') {
    return <EntryDetail entry={view.entry} onBack={() => setView({ kind: 'list' })} />;
  }
  if (view.kind === 'create') {
    return (
      <CreateEntryForm
        onCancel={() => setView({ kind: 'list' })}
        onCreated={() => {
          void search('');
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

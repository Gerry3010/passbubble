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

import { useState, useEffect } from 'react';
import { useEntriesStore } from '../store/entries.js';
import type { EntryResponse } from '@passbubble/shared-ts';

interface CopyState {
  entryId: string;
  field: 'username' | 'password';
}

export function EntryList() {
  const { entries, isLoading, search, copyField } = useEntriesStore();
  const [query, setQuery] = useState('');
  const [copied, setCopied] = useState<CopyState | null>(null);

  useEffect(() => {
    void search('');
  }, []);

  useEffect(() => {
    const timer = setTimeout(() => void search(query), 300);
    return () => clearTimeout(timer);
  }, [query]);

  async function handleCopy(entryId: string, field: 'username' | 'password') {
    await copyField(entryId, field);
    setCopied({ entryId, field });
    setTimeout(() => setCopied(null), 1500);
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
      <input
        type="search"
        placeholder="Search entries…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        style={{
          padding: '8px',
          borderRadius: '4px',
          border: '1px solid #e2e8f0',
          fontSize: '13px',
          width: '100%',
        }}
      />
      {isLoading && <p style={{ color: '#718096', fontSize: '12px' }}>Loading…</p>}
      {!isLoading && entries.length === 0 && (
        <p style={{ color: '#718096', fontSize: '12px' }}>No entries found</p>
      )}
      <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: '4px' }}>
        {entries.map((entry: EntryResponse) => (
          <EntryItem
            key={entry.id}
            entry={entry}
            copied={copied}
            onCopy={handleCopy}
          />
        ))}
      </ul>
    </div>
  );
}

function EntryItem({
  entry,
  copied,
  onCopy,
}: {
  entry: EntryResponse;
  copied: CopyState | null;
  onCopy: (id: string, field: 'username' | 'password') => void;
}) {
  const isCopiedUser = copied?.entryId === entry.id && copied.field === 'username';
  const isCopiedPw = copied?.entryId === entry.id && copied.field === 'password';

  return (
    <li
      style={{
        padding: '8px',
        borderRadius: '6px',
        border: '1px solid #e2e8f0',
        display: 'flex',
        flexDirection: 'column',
        gap: '4px',
      }}
    >
      <span style={{ fontWeight: 600, fontSize: '13px', color: '#2d3748' }}>{entry.name}</span>
      {entry.url && (
        <span style={{ color: '#718096', fontSize: '11px' }}>{entry.url}</span>
      )}
      <div style={{ display: 'flex', gap: '4px', marginTop: '2px' }}>
        <CopyButton
          label={isCopiedUser ? 'Copied!' : 'Username'}
          copied={isCopiedUser}
          onClick={() => onCopy(entry.id, 'username')}
        />
        <CopyButton
          label={isCopiedPw ? 'Copied!' : 'Password'}
          copied={isCopiedPw}
          onClick={() => onCopy(entry.id, 'password')}
        />
      </div>
    </li>
  );
}

function CopyButton({
  label,
  copied,
  onClick,
}: {
  label: string;
  copied: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      style={{
        padding: '3px 8px',
        fontSize: '11px',
        borderRadius: '3px',
        border: '1px solid #e2e8f0',
        background: copied ? '#48bb78' : '#fff',
        color: copied ? '#fff' : '#4a5568',
        cursor: 'pointer',
        transition: 'background 0.2s',
      }}
    >
      {label}
    </button>
  );
}

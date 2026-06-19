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

import { useState, useEffect, useMemo } from 'react';
import { useEntriesStore } from '../store/entries.js';
import type { EntryResponse, FolderResponse } from '@passbubble/shared-ts';
import { term, input, buttonPrimary, buttonGhost, link, muted } from '../../shared/theme.js';

interface CopyState {
  entryId: string;
  field: 'username' | 'password';
}

export function EntryList({
  onSelect,
  onCreate,
}: {
  onSelect?: (entry: EntryResponse) => void;
  onCreate?: () => void;
} = {}) {
  const { entries, folders, currentHost, isLoading, load, copyField } = useEntriesStore();
  const [query, setQuery] = useState('');
  const [folderId, setFolderId] = useState<string | null>(null);
  const [seededHost, setSeededHost] = useState(false);
  const [copied, setCopied] = useState<CopyState | null>(null);

  useEffect(() => {
    void load();
  }, []);

  // Once the vault has loaded, pre-fill the search with the active tab's host
  // so the popup opens straight onto the credentials for the current site.
  useEffect(() => {
    if (!seededHost && currentHost) {
      setQuery(currentHost);
      setSeededHost(true);
    }
  }, [currentHost, seededHost]);

  const q = query.trim().toLowerCase();
  const searching = q.length > 0;

  const results = useMemo(() => {
    if (!searching) return [];
    return entries.filter(
      (e) => e.name.toLowerCase().includes(q) || (e.url ?? '').toLowerCase().includes(q),
    );
  }, [entries, q, searching]);

  // Folder-browser view (empty search): folders + entries at the current level.
  const subfolders = useMemo(
    () => folders.filter((f) => (f.parent_id ?? null) === folderId),
    [folders, folderId],
  );
  const folderEntries = useMemo(
    () => entries.filter((e) => (e.folder_id ?? null) === folderId),
    [entries, folderId],
  );
  const folderById = useMemo(() => {
    const m = new Map<string, FolderResponse>();
    for (const f of folders) m.set(f.id, f);
    return m;
  }, [folders]);

  const currentFolder = folderId ? folderById.get(folderId) : undefined;

  async function handleCopy(entryId: string, field: 'username' | 'password') {
    await copyField(entryId, field);
    setCopied({ entryId, field });
    setTimeout(() => setCopied(null), 1500);
  }

  function countInFolder(id: string): number {
    return entries.filter((e) => (e.folder_id ?? null) === id).length;
  }

  const isEmpty = searching
    ? results.length === 0
    : subfolders.length === 0 && folderEntries.length === 0;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
      <div style={{ display: 'flex', gap: '6px' }}>
        <input
          type="search"
          placeholder="grep entries…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          style={{ ...input, flex: 1 }}
        />
        {onCreate && (
          <button
            onClick={onCreate}
            title="New entry"
            style={{ ...buttonPrimary, padding: '0 12px', fontSize: '16px' }}
          >
            +
          </button>
        )}
      </div>

      {/* Breadcrumb when browsing inside a folder */}
      {!searching && currentFolder && (
        <button
          onClick={() => setFolderId(currentFolder.parent_id ?? null)}
          style={{ ...link, alignSelf: 'flex-start' }}
        >
          ‹ {currentFolder.name}
        </button>
      )}

      {isLoading && entries.length === 0 && <p style={muted}>Loading…</p>}
      {!isLoading && isEmpty && <p style={muted}>No entries found</p>}

      <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: '4px' }}>
        {/* Folder rows (only in the browser view) */}
        {!searching &&
          subfolders.map((f) => (
            <FolderRow key={f.id} folder={f} count={countInFolder(f.id)} onOpen={() => setFolderId(f.id)} />
          ))}

        {/* Entry rows: filtered results when searching, else current folder */}
        {(searching ? results : folderEntries).map((entry: EntryResponse) => (
          <EntryItem
            key={entry.id}
            entry={entry}
            copied={copied}
            onCopy={handleCopy}
            onSelect={onSelect}
          />
        ))}
      </ul>
    </div>
  );
}

function FolderRow({
  folder,
  count,
  onOpen,
}: {
  folder: FolderResponse;
  count: number;
  onOpen: () => void;
}) {
  return (
    <li>
      <button
        onClick={onOpen}
        style={{
          ...buttonGhost,
          width: '100%',
          textAlign: 'left',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: '8px',
        }}
      >
        <span style={{ fontWeight: 700 }}>📁 {folder.name}</span>
        <span style={{ color: term.muted, fontSize: '11px' }}>{count}</span>
      </button>
    </li>
  );
}

function EntryItem({
  entry,
  copied,
  onCopy,
  onSelect,
}: {
  entry: EntryResponse;
  copied: CopyState | null;
  onCopy: (id: string, field: 'username' | 'password') => void;
  onSelect?: (entry: EntryResponse) => void;
}) {
  const isCopiedUser = copied?.entryId === entry.id && copied.field === 'username';
  const isCopiedPw = copied?.entryId === entry.id && copied.field === 'password';

  return (
    <li
      style={{
        padding: '8px',
        borderRadius: '4px',
        border: `1px solid ${term.border}`,
        background: term.surface,
        display: 'flex',
        flexDirection: 'column',
        gap: '4px',
        minWidth: 0,
      }}
    >
      <span
        onClick={onSelect ? () => onSelect(entry) : undefined}
        style={{
          fontWeight: 700,
          fontSize: '13px',
          color: term.green,
          cursor: onSelect ? 'pointer' : 'default',
          display: 'block',
          overflow: 'hidden',
          whiteSpace: 'nowrap',
          textOverflow: 'ellipsis',
        }}
      >
        {entry.name}
      </span>
      {entry.url && (
        <span
          title={entry.url}
          style={{
            color: term.muted,
            fontSize: '11px',
            display: 'block',
            overflow: 'hidden',
            whiteSpace: 'nowrap',
            textOverflow: 'ellipsis',
          }}
        >
          {entry.url}
        </span>
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
        {onSelect && (
          <CopyButton label="Details" copied={false} onClick={() => onSelect(entry)} />
        )}
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
      style={copied
        ? { ...buttonPrimary, padding: '3px 8px', fontSize: '11px' }
        : { ...buttonGhost, padding: '3px 8px', fontSize: '11px' }}
    >
      {label}
    </button>
  );
}

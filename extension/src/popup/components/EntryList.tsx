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
import {
  cleanTag,
  collectTags,
  containsTag,
  hasAllTags,
  normalizeTags,
  parseTagQuery,
  sameTag,
} from '@passbubble/shared-ts';
import type { EntryResponse, FolderResponse } from '@passbubble/shared-ts';
import { patternMatchesHost } from '../../shared/utils.js';
import { term, input, buttonPrimary, buttonGhost, link, muted, tagChip, tagChipActive } from '../../shared/theme.js';

const NO_TAGS: string[] = [];
const NO_ENTRY_TAGS: Record<string, string[]> = {};

// Tag chips shown before the "+N more" toggle; active chips always show.
const TAG_CHIP_LIMIT = 8;

/** `query` without its `#tag` tokens for `tag` (case-insensitive). */
function removeTagTokens(query: string, tag: string): string {
  return query
    .split(/\s+/)
    .filter((token) => !(token.startsWith('#') && sameTag(cleanTag(token), tag)))
    .join(' ')
    .trim();
}

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
  const {
    entries,
    folders,
    usernames,
    entryTags = NO_ENTRY_TAGS,
    currentHost,
    isLoading,
    load,
    copyField,
    toggleSite,
    removeMatch,
    toggleFavorite,
  } = useEntriesStore();
  const [query, setQuery] = useState('');
  const [folderId, setFolderId] = useState<string | null>(null);
  const [showAll, setShowAll] = useState(false);
  const [copied, setCopied] = useState<CopyState | null>(null);
  // Tags switched on via the chips; `#tag` tokens in the query filter as well.
  const [selectedTags, setSelectedTags] = useState<string[]>([]);
  const [allTagsShown, setAllTagsShown] = useState(false);

  useEffect(() => {
    void load();
  }, []);

  // `#tag` tokens in the query and the selected chips filter together (an
  // entry needs all of them); the remaining words are the text search.
  const parsed = useMemo(() => parseTagQuery(query), [query]);
  const activeTags = useMemo(() => normalizeTags([...selectedTags, ...parsed.tags]), [selectedTags, parsed.tags]);
  const vaultTags = useMemo(
    () => collectTags(entries.map((e) => entryTags[e.id] ?? NO_TAGS)),
    [entries, entryTags],
  );

  // The active tab's host is only the placeholder, never the field's value: the
  // popup opens on the current site's credentials, and typing (or picking a
  // tag) replaces the host instead of adding to it. "show all" (or a host
  // without matches) falls back to the folder browser.
  const typed = parsed.text.toLowerCase();
  const filtering = typed.length > 0 || activeTags.length > 0;
  const hostQuery =
    !showAll && !query.trim() && selectedTags.length === 0 && currentHost ? currentHost.toLowerCase() : '';
  const q = typed || hostQuery;

  const matches = useMemo(() => {
    if (!q && activeTags.length === 0) return [];
    return entries.filter(
      (e) =>
        hasAllTags(entryTags[e.id] ?? NO_TAGS, activeTags) &&
        (!q ||
          e.name.toLowerCase().includes(q) ||
          (e.url ?? '').toLowerCase().includes(q) ||
          (usernames[e.id] ?? '').toLowerCase().includes(q) ||
          // Typed text also finds tag text, like in the app (not the host prefill).
          (!!typed && (entryTags[e.id] ?? NO_TAGS).some((t) => t.toLowerCase().includes(typed))) ||
          // Match patterns: substring (partial typing) OR wildcard host-match, so a
          // query like "gist.github.com" also finds an entry whose pattern is
          // "*.github.com". (Add/remove stay exact — only search expands wildcards.)
          (e.match_patterns ?? []).some((p) => p.toLowerCase().includes(q) || patternMatchesHost(q, p))),
    );
  }, [entries, usernames, entryTags, q, typed, activeTags]);

  const hostMode = !filtering && hostQuery.length > 0 && matches.length > 0;
  const searching = filtering || hostMode;
  const results = searching ? matches : [];

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

  // A chip switches its tag on or off. A tag typed as `#tag` is switched off
  // by removing the token from the query, so the field never disagrees with
  // the chips.
  function toggleTag(tag: string) {
    if (containsTag(parsed.tags, tag)) {
      setQuery(removeTagTokens(query, tag));
      setSelectedTags((prev) => prev.filter((t) => !sameTag(t, tag)));
      return;
    }
    setSelectedTags((prev) => (containsTag(prev, tag) ? prev.filter((t) => !sameTag(t, tag)) : [...prev, tag]));
  }

  function clearTags() {
    setSelectedTags([]);
    if (parsed.tags.length > 0) setQuery(parsed.text);
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
          aria-label="Search entries"
          placeholder={hostQuery ? currentHost : 'grep entries…'}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          autoFocus
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

      {vaultTags.length > 0 && (
        <TagFilter
          tags={vaultTags}
          active={activeTags}
          expanded={allTagsShown}
          onToggle={toggleTag}
          onToggleExpanded={() => setAllTagsShown((v) => !v)}
          onClear={clearTags}
        />
      )}

      {/* Breadcrumb when browsing inside a folder */}
      {!searching && currentFolder && (
        <button
          onClick={() => setFolderId(currentFolder.parent_id ?? null)}
          style={{ ...link, alignSelf: 'flex-start' }}
        >
          ‹ {currentFolder.name}
        </button>
      )}

      {hostMode && (
        <p style={{ ...muted, margin: 0, fontSize: '11px' }}>
          matches for {currentHost} ·{' '}
          <button onClick={() => setShowAll(true)} style={{ ...link, fontSize: '11px' }}>
            show all
          </button>
        </p>
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
            tags={entryTags[entry.id] ?? NO_TAGS}
            copied={copied}
            currentHost={currentHost}
            onCopy={handleCopy}
            onSelect={onSelect}
            onToggleSite={toggleSite}
            onRemoveMatch={removeMatch}
            onToggleFavorite={toggleFavorite}
          />
        ))}
      </ul>
    </div>
  );
}

/** The vault's tags as toggle chips. Collapsed to TAG_CHIP_LIMIT chips (plus
 * any active ones) until expanded. */
function TagFilter({
  tags,
  active,
  expanded,
  onToggle,
  onToggleExpanded,
  onClear,
}: {
  tags: string[];
  active: string[];
  expanded: boolean;
  onToggle: (tag: string) => void;
  onToggleExpanded: () => void;
  onClear: () => void;
}) {
  const visible = expanded ? tags : tags.filter((t, i) => i < TAG_CHIP_LIMIT || containsTag(active, t));
  const hidden = tags.length - visible.length;
  return (
    <div role="group" aria-label="Filter by tag" style={{ display: 'flex', flexWrap: 'wrap', gap: '4px', alignItems: 'center' }}>
      {visible.map((t) => {
        const on = containsTag(active, t);
        return (
          <button key={t} aria-pressed={on} onClick={() => onToggle(t)} style={on ? tagChipActive : tagChip}>
            #{t}
          </button>
        );
      })}
      {tags.length > TAG_CHIP_LIMIT && (hidden > 0 || expanded) && (
        <button onClick={onToggleExpanded} style={{ ...link, fontSize: '11px' }}>
          {expanded ? 'less' : `+${hidden} more`}
        </button>
      )}
      {active.length > 0 && (
        <button onClick={onClear} style={{ ...link, fontSize: '11px', color: term.muted }}>
          clear
        </button>
      )}
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
  tags,
  copied,
  currentHost,
  onCopy,
  onSelect,
  onToggleSite,
  onRemoveMatch,
  onToggleFavorite,
}: {
  entry: EntryResponse;
  tags: string[];
  copied: CopyState | null;
  currentHost: string;
  onCopy: (id: string, field: 'username' | 'password') => void;
  onSelect?: (entry: EntryResponse) => void;
  onToggleSite: (id: string) => void;
  onRemoveMatch: (id: string, pattern: string) => void;
  onToggleFavorite: (id: string) => void;
}) {
  const isCopiedUser = copied?.entryId === entry.id && copied.field === 'username';
  const isCopiedPw = copied?.entryId === entry.id && copied.field === 'password';

  const patterns = entry.match_patterns ?? [];
  const hostActive = !!currentHost && patterns.includes(currentHost);
  const [showSites, setShowSites] = useState(false);

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
      <span style={{ display: 'flex', alignItems: 'center', gap: '4px', minWidth: 0 }}>
        <button
          onClick={() => onToggleFavorite(entry.id)}
          aria-label={entry.favorite ? 'Remove from favorites' : 'Add to favorites'}
          title={entry.favorite ? 'Remove from favorites' : 'Add to favorites'}
          style={{
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            padding: 0,
            fontSize: '13px',
            lineHeight: 1,
            color: entry.favorite ? term.green : term.muted,
            flexShrink: 0,
          }}
        >
          {entry.favorite ? '★' : '☆'}
        </button>
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
            flex: 1,
            minWidth: 0,
          }}
        >
          {typeIcon(entry.type)}
          {entry.name}
        </span>
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
      {tags.length > 0 && (
        <span
          style={{
            color: term.muted,
            fontSize: '11px',
            display: 'block',
            overflow: 'hidden',
            whiteSpace: 'nowrap',
            textOverflow: 'ellipsis',
          }}
        >
          {tags.map((t) => `#${t}`).join(' ')}
        </span>
      )}
      <div style={{ display: 'flex', gap: '4px', marginTop: '2px', flexWrap: 'wrap' }}>
        {/* Username/password shortcuts only for login-ish types — cards,
            identities and notes have neither. */}
        {['password', 'api-key', 'ssh-key', 'totp'].includes(entry.type) && (
          <>
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
          </>
        )}
        {onSelect && (
          <CopyButton label="Details" copied={false} onClick={() => onSelect(entry)} />
        )}
        {currentHost && (
          <CopyButton
            label={hostActive ? `✓ ${currentHost}` : '+ Site'}
            copied={hostActive}
            onClick={() => onToggleSite(entry.id)}
          />
        )}
      </div>

      {/* Toggleable list of this entry's autofill match patterns. */}
      {patterns.length > 0 && (
        <button
          onClick={() => setShowSites((v) => !v)}
          style={{ ...link, alignSelf: 'flex-start', fontSize: '11px' }}
        >
          {showSites ? '▾' : '▸'} {patterns.length} match {patterns.length === 1 ? 'site' : 'sites'}
        </button>
      )}
      {showSites && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '3px', paddingLeft: '4px' }}>
          {patterns.map((p) => (
            <div
              key={p}
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                gap: '8px',
                fontSize: '11px',
                color: term.muted,
                fontFamily: term.font,
              }}
            >
              <span style={{ overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}>{p}</span>
              <button
                onClick={() => onRemoveMatch(entry.id, p)}
                aria-label={`Remove ${p}`}
                title={`Remove ${p}`}
                style={{ ...link, color: term.red, flexShrink: 0 }}
              >
                ×
              </button>
            </div>
          ))}
        </div>
      )}
    </li>
  );
}

// Small type marker in front of the entry name; logins stay unmarked (they are
// the default and the list would otherwise be a wall of keys).
function typeIcon(type: string): string {
  switch (type) {
    case 'credit-card':
      return '💳 ';
    case 'bank-account':
      return '🏦 ';
    case 'identity':
      return '👤 ';
    case 'note':
      return '📝 ';
    case 'license':
      return '📜 ';
    case 'api-key':
    case 'ssh-key':
      return '⚙ ';
    default:
      return '';
  }
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

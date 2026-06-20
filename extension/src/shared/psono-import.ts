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

// Parses an (unencrypted) Psono JSON export into create-ready records.
// Mirrors backend/pkg/importers/psono.go. Crypto/upload happens in the
// background service worker — this only produces plaintext intermediates.

import type { EntryData } from '@passbubble/shared-ts';

export interface ImportRecord {
  name: string;
  type: string; // "password" | "totp" | "note"
  url?: string;
  matchPatterns?: string[];
  data: EntryData;
}

export interface ImportParseResult {
  records: ImportRecord[];
  skipped: number;
  warnings: string[];
}

interface PsonoItem {
  [key: string]: unknown;
}

interface PsonoFolder {
  name?: string;
  items?: PsonoItem[];
  folders?: PsonoFolder[];
}

interface PsonoExport {
  folders?: PsonoFolder[];
  items?: PsonoItem[];
}

function str(item: PsonoItem, key: string): string {
  const v = item[key];
  return typeof v === 'string' ? v : '';
}

// Split Psono urlfilter values (space/comma/newline/semicolon separated) from the
// first non-empty source into a deduplicated pattern list.
function parseURLFilters(...sources: string[]): string[] | undefined {
  const raw = sources.find((s) => s.trim() !== '');
  if (!raw) return undefined;
  const seen = new Set<string>();
  const out: string[] = [];
  for (const f of raw.split(/[\s,;]+/)) {
    const t = f.trim();
    if (t && !seen.has(t)) {
      seen.add(t);
      out.push(t);
    }
  }
  return out.length ? out : undefined;
}

function convertItem(item: PsonoItem, result: ImportParseResult): void {
  const entryType = str(item, 'type');
  const name = str(item, 'name');
  let rec: ImportRecord | null = null;

  switch (entryType) {
    case 'website_password': {
      const totp = str(item, 'website_password_totp_code');
      rec = {
        name: str(item, 'website_password_title') || name,
        type: totp ? 'totp' : 'password',
        url: str(item, 'website_password_url') || undefined,
        data: {
          username: str(item, 'website_password_username'),
          password: str(item, 'website_password_password'),
          totp_secret: totp || undefined,
          notes: str(item, 'website_password_notes') || undefined,
        },
      };
      break;
    }
    case 'application_password':
      rec = {
        name: str(item, 'application_password_title') || name,
        type: 'password',
        data: {
          username: str(item, 'application_password_username'),
          password: str(item, 'application_password_password'),
        },
      };
      break;
    case 'bookmark':
      rec = {
        name: str(item, 'bookmark_title') || name,
        type: 'password',
        url: str(item, 'bookmark_url') || undefined,
        data: {},
      };
      break;
    case 'totp':
      rec = {
        name: str(item, 'totp_title') || name,
        type: 'totp',
        data: {
          totp_secret: str(item, 'totp_code'),
          notes: str(item, 'totp_notes') || undefined,
        },
      };
      break;
    case 'note':
      rec = {
        name: str(item, 'note_title') || name,
        type: 'note',
        data: { notes: str(item, 'note_notes') },
      };
      break;
    default:
      result.skipped++;
      if (entryType) result.warnings.push(`skipping unknown Psono type "${entryType}" (${name})`);
      return;
  }

  if (!rec.name) {
    result.skipped++;
    result.warnings.push('skipping unnamed Psono entry');
    return;
  }

  rec.matchPatterns = parseURLFilters(
    str(item, 'website_password_url_filter'),
    str(item, 'application_password_url_filter'),
    str(item, 'bookmark_url_filter'),
    str(item, 'urlfilter'),
  );

  result.records.push(rec);
}

function walkFolder(folder: PsonoFolder, result: ImportParseResult): void {
  for (const item of folder.items ?? []) convertItem(item, result);
  for (const sub of folder.folders ?? []) walkFolder(sub, result);
}

/** Parse a Psono JSON export string. Throws on malformed JSON. */
export function parsePsono(json: string): ImportParseResult {
  const result: ImportParseResult = { records: [], skipped: 0, warnings: [] };
  const data = JSON.parse(json) as PsonoExport;
  for (const item of data.items ?? []) convertItem(item, result);
  for (const folder of data.folders ?? []) walkFolder(folder, result);
  return result;
}

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

// Parts shared by the create and the edit form.

import {
  base32Decode,
  normalizeTags,
  parseOtpauthUri,
  suggestTags,
  tagInputError,
} from '@passbubble/shared-ts';
import type { EntryData } from '@passbubble/shared-ts';
import { input, errorText, tagChip } from '../../shared/theme.js';

// Accepts a 2FA setup value as pasted by the user — either an otpauth:// URI
// (the QR-code payload) or a bare base32 secret — and returns the EntryData
// TOTP fields, or null when the value is not usable.
export function parseTotpInput(raw: string): Partial<EntryData> | null {
  const value = raw.trim();
  if (!value) return {};
  const parsed = parseOtpauthUri(value);
  if (parsed) {
    return {
      totp_secret: parsed.secret,
      issuer: parsed.issuer,
      period: parsed.period,
      digits: parsed.digits,
      algorithm: parsed.algorithm,
    };
  }
  const secret = value.replace(/[\s-]/g, '').toUpperCase();
  try {
    base32Decode(secret);
  } catch {
    return null;
  }
  return { totp_secret: secret };
}

/** Comma-separated tag input with completions from the tags already used in the
 * vault. The caller holds the text and normalises it on save (parseTagInput). */
export function TagsInput({
  value,
  onChange,
  vaultTags,
}: {
  value: string;
  onChange: (next: string) => void;
  vaultTags: string[];
}) {
  // The tag being typed is the text after the last comma; vault tags that
  // match it are offered as completions.
  const parts = value.split(',');
  const typingTag = parts[parts.length - 1];
  const enteredTags = normalizeTags(parts.slice(0, -1));
  const error = parts.map(tagInputError).find((e) => e !== null) ?? null;
  const suggestions = suggestTags(vaultTags, typingTag, { exclude: enteredTags, limit: 6 });

  return (
    <>
      <input
        style={input}
        placeholder="Tags (comma-separated, optional)"
        aria-label="Tags"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      {error && <span style={{ ...errorText, fontSize: '11px' }}>{error}</span>}
      {suggestions.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '4px' }}>
          {suggestions.map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => onChange([...enteredTags, t].join(', ') + ', ')}
              style={tagChip}
            >
              #{t}
            </button>
          ))}
        </div>
      )}
    </>
  );
}

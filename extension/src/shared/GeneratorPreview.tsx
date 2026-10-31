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

// The generated password with its strength bar and a Regenerate button.
// When the options cannot produce a password, the reason is shown instead.

import { checkStrength } from '@passbubble/shared-ts';
import { term, buttonGhost, errorText, withDisabled } from './theme.js';

export function GeneratorPreview({
  password,
  error,
  onRegenerate,
}: {
  password: string;
  error: string | null;
  onRegenerate: () => void;
}) {
  const strength = password && !error ? checkStrength(password) : null;
  const score = strength?.score ?? 0;
  const color = score >= 80 ? term.green : score >= 50 ? term.amber : term.red;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
      <output
        aria-label="Generated password"
        style={{
          display: 'block',
          fontFamily: term.font,
          fontSize: '13px',
          padding: '6px 8px',
          minHeight: '18px',
          background: term.surface,
          border: `1px solid ${error ? term.red : term.border}`,
          borderRadius: '4px',
          wordBreak: 'break-all',
          color: term.green,
        }}
      >
        {error ? <span style={errorText}>{error}</span> : password}
      </output>
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <div
          style={{ flex: 1, height: '4px', background: term.border, borderRadius: '2px', overflow: 'hidden' }}
        >
          <div style={{ width: `${Math.max(0, Math.min(100, score))}%`, height: '100%', background: color }} />
        </div>
        {strength && (
          <span style={{ fontSize: '11px', color, fontWeight: 700, whiteSpace: 'nowrap' }}>{strength.level}</span>
        )}
        <button
          type="button"
          onClick={onRegenerate}
          disabled={!!error}
          style={withDisabled({ ...buttonGhost, padding: '2px 8px', fontSize: '11px', whiteSpace: 'nowrap' }, !!error)}
        >
          ↻ Regenerate
        </button>
      </div>
    </div>
  );
}

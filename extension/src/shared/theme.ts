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

// Terminal / "matrix" theme shared by the popup and the options page. The look
// mirrors the brand icon: phosphor green on near-black, monospace everywhere.

import type { CSSProperties } from 'react';

export const term = {
  bg: '#0a0a0a',
  surface: '#0e140f',
  green: '#00ff41',
  greenDim: '#19a23a',
  text: '#00ff41',
  muted: '#5f8c6a',
  border: '#1d3a24',
  borderBright: '#00ff4155',
  red: '#ff5f56',
  amber: '#ffb000',
  font: "'Courier New', 'Lucida Console', Monaco, monospace",
} as const;

export const input: CSSProperties = {
  padding: '8px',
  borderRadius: '2px',
  border: `1px solid ${term.border}`,
  background: term.bg,
  color: term.green,
  fontFamily: term.font,
  fontSize: '13px',
  width: '100%',
  boxSizing: 'border-box',
};

/** Solid green "primary" action — black text on phosphor green. */
export const buttonPrimary: CSSProperties = {
  padding: '8px',
  background: term.green,
  color: term.bg,
  border: `1px solid ${term.green}`,
  borderRadius: '2px',
  cursor: 'pointer',
  fontFamily: term.font,
  fontSize: '13px',
  fontWeight: 700,
};

/** Outlined "ghost" action — green text on transparent. */
export const buttonGhost: CSSProperties = {
  padding: '8px',
  background: 'transparent',
  color: term.green,
  border: `1px solid ${term.border}`,
  borderRadius: '2px',
  cursor: 'pointer',
  fontFamily: term.font,
  fontSize: '13px',
};

/** Bare text link (no border/background). */
export const link: CSSProperties = {
  background: 'none',
  border: 'none',
  color: term.green,
  cursor: 'pointer',
  fontFamily: term.font,
  fontSize: '12px',
  padding: 0,
};

export const card: CSSProperties = {
  border: `1px solid ${term.border}`,
  borderRadius: '4px',
  background: term.surface,
  padding: '8px',
};

export const heading: CSSProperties = {
  margin: 0,
  fontSize: '15px',
  fontWeight: 700,
  color: term.green,
  fontFamily: term.font,
};

export const muted: CSSProperties = {
  color: term.muted,
  fontSize: '12px',
  margin: 0,
};

export const errorText: CSSProperties = {
  color: term.red,
  fontSize: '12px',
  margin: 0,
};

/** Disables a button visually (keeps the layout identical). */
export function withDisabled(style: CSSProperties, disabled: boolean): CSSProperties {
  return disabled ? { ...style, opacity: 0.5, cursor: 'not-allowed' } : style;
}

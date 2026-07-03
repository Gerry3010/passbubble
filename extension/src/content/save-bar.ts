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

// In-page "save this login?" bar, injected by the save detector. Rendered in a
// closed Shadow DOM (style-isolated, unreadable by the page). It shows no
// secrets — only the host + username; the password stays in the background's
// pending-save and is only consumed by CONFIRM_SAVE.

const term = {
  bg: '#0a0a0a',
  surface: '#0e140f',
  green: '#00ff41',
  muted: '#5f8c6a',
  border: '#1d3a24',
  yellow: '#ffcc00',
  font: "'Courier New', 'Lucida Console', Monaco, monospace",
};

let barHost: HTMLElement | null = null;

// True when `node` is (inside) the save bar. The shadow root is closed, so
// events from inside it are retargeted to the host element — checking the host
// and its subtree is sufficient for outside listeners.
export function saveBarContains(node: unknown): boolean {
  return !!barHost && node instanceof Node && (node === barHost || barHost.contains(node));
}

export function removeSaveBar(): void {
  if (barHost) {
    barHost.remove();
    barHost = null;
  }
}

export interface SaveBarInfo {
  host: string;
  username: string;
  // Existing entries that already match this site. When present, the bar offers
  // to UPDATE one of them (picker when more than one) in addition to creating a
  // new entry. Empty/undefined → plain "save this login?" bar.
  candidates?: { id: string; username: string }[];
  // The entry most likely meant by an update (same username, changed password).
  suggestUpdateId?: string;
}

export interface SaveBarHandlers {
  onSaveNew: () => void;
  onUpdate: (entryId: string) => void;
  onDismiss: () => void;
  onNever: () => void;
}

export function showSaveBar(info: SaveBarInfo, handlers: SaveBarHandlers): void {
  removeSaveBar();

  const candidates = info.candidates ?? [];
  const hasCandidates = candidates.length > 0;

  const host = document.createElement('div');
  host.style.cssText =
    'all: initial; position: fixed; top: 12px; left: 50%; transform: translateX(-50%); z-index: 2147483647;';
  const shadow = host.attachShadow({ mode: 'closed' });

  const bar = document.createElement('div');
  bar.style.cssText = [
    `font-family: ${term.font}`,
    `background: ${term.bg}`,
    `color: ${term.green}`,
    `border: 1px solid ${term.green}`,
    'border-radius: 6px',
    'padding: 10px 12px',
    'display: flex',
    'align-items: center',
    'gap: 12px',
    'box-shadow: 0 4px 24px rgba(0,0,0,0.4)',
    'font-size: 13px',
    'max-width: 90vw',
  ].join(';');

  const text = document.createElement('div');
  text.style.cssText = 'display:flex; flex-direction:column; gap:2px; min-width:0;';
  const title = document.createElement('span');
  title.textContent = hasCandidates ? 'Update this login?' : 'Save this login?';
  title.style.cssText = `font-weight:700; color:${term.green};`;
  const sub = document.createElement('span');
  sub.style.cssText = `font-size:11px; overflow:hidden; white-space:nowrap; text-overflow:ellipsis;`;
  if (info.username) {
    sub.textContent = `${info.host} — ${info.username}`;
    sub.style.color = term.muted;
  } else {
    // No username detected — warn the user (yellow ⚠) so they know the entry
    // would be saved without one and can fix the field before saving.
    sub.textContent = `⚠ ${info.host} — no username detected`;
    sub.style.color = term.yellow;
  }
  text.appendChild(title);
  text.appendChild(sub);
  bar.appendChild(text);

  const mkBtn = (label: string, primary: boolean): HTMLButtonElement => {
    const b = document.createElement('button');
    b.textContent = label;
    b.style.cssText = [
      `font-family: ${term.font}`,
      'font-size: 12px',
      'padding: 6px 12px',
      'border-radius: 4px',
      'cursor: pointer',
      primary ? `background: ${term.green}` : `background: ${term.surface}`,
      primary ? 'color: #0a0a0a' : `color: ${term.muted}`,
      primary ? `border: 1px solid ${term.green}` : `border: 1px solid ${term.border}`,
      primary ? 'font-weight: 700' : 'font-weight: 400',
    ].join(';');
    return b;
  };

  if (hasCandidates) {
    // Picker for which existing entry to overwrite (only when more than one).
    let selectedId = info.suggestUpdateId ?? candidates[0].id;
    if (candidates.length > 1) {
      const select = document.createElement('select');
      select.style.cssText = [
        `font-family: ${term.font}`,
        'font-size: 12px',
        'padding: 5px 8px',
        'border-radius: 4px',
        `background: ${term.surface}`,
        `color: ${term.green}`,
        `border: 1px solid ${term.border}`,
        'max-width: 180px',
      ].join(';');
      for (const c of candidates) {
        const opt = document.createElement('option');
        opt.value = c.id;
        opt.textContent = c.username || '(no username)';
        if (c.id === selectedId) opt.selected = true;
        select.appendChild(opt);
      }
      select.addEventListener('change', () => {
        selectedId = select.value;
      });
      bar.appendChild(select);
    }

    const updateBtn = mkBtn('Update', true);
    const newBtn = mkBtn('New entry', false);
    const dismissBtn = mkBtn('Not now', false);
    const neverBtn = mkBtn('Never', false);
    neverBtn.title = `Never offer to save for ${info.host}`;
    updateBtn.addEventListener('click', () => handlers.onUpdate(selectedId));
    newBtn.addEventListener('click', () => handlers.onSaveNew());
    dismissBtn.addEventListener('click', () => handlers.onDismiss());
    neverBtn.addEventListener('click', () => handlers.onNever());
    bar.appendChild(updateBtn);
    bar.appendChild(newBtn);
    bar.appendChild(dismissBtn);
    bar.appendChild(neverBtn);
  } else {
    const saveBtn = mkBtn('Save', true);
    const dismissBtn = mkBtn('Not now', false);
    const neverBtn = mkBtn('Never', false);
    neverBtn.title = `Never offer to save for ${info.host}`;
    saveBtn.addEventListener('click', () => handlers.onSaveNew());
    dismissBtn.addEventListener('click', () => handlers.onDismiss());
    neverBtn.addEventListener('click', () => handlers.onNever());
    bar.appendChild(saveBtn);
    bar.appendChild(dismissBtn);
    bar.appendChild(neverBtn);
  }

  shadow.appendChild(bar);
  document.body.appendChild(host);
  barHost = host;
}

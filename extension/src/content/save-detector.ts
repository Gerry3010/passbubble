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

// Intercepts form submissions to offer saving new credentials.
// Does NOT call preventDefault() — the save offer is non-blocking.

import browser from 'webextension-polyfill';
import { MessageType } from '../shared/constants.js';
import { normaliseHost } from '../shared/utils.js';
import { showSaveBar, removeSaveBar } from './save-bar.js';

interface SaveOffer {
  host: string;
  username: string;
  candidates?: { id: string; username: string }[];
  suggestUpdateId?: string;
}

interface DetectedCredentials {
  username: string;
  password: string;
}

const USERNAME_SELECTOR =
  'input[type="text"], input[type="email"], input[type="tel"], input[autocomplete="username"], input:not([type])';

// Heuristic: a field whose name/id/placeholder/aria-label looks like a login id.
function looksLikeUsername(i: HTMLInputElement): boolean {
  if (i.getAttribute('autocomplete') === 'username') return true;
  if (i.type === 'email') return true;
  const hay = `${i.name} ${i.id} ${i.placeholder} ${i.getAttribute('aria-label') ?? ''}`;
  return /user|e-?mail|login|account|phone|mobile|(^|[^a-z])name([^a-z]|$)/i.test(hay);
}

// Find the value the user actually typed as their username. The previous version
// grabbed the FIRST matching field regardless of value, which often picked an
// empty/unrelated input — so a username-ish field WITH a value wins, then the
// nearest non-empty text field before the password, then any non-empty field.
function findUsernameValue(form: HTMLFormElement, pw: HTMLInputElement): string {
  const scoped = Array.from(form.querySelectorAll<HTMLInputElement>(USERNAME_SELECTOR));
  const candidates = scoped.length
    ? scoped
    : Array.from(document.querySelectorAll<HTMLInputElement>(USERNAME_SELECTOR));
  const withValue = candidates.filter((i) => i.value.trim() !== '');

  const preferred = withValue.filter(looksLikeUsername);
  if (preferred.length) return preferred[preferred.length - 1].value.trim();

  // Nearest non-empty text-like field before the password field in DOM order.
  const all = Array.from((form.ownerDocument ?? document).querySelectorAll<HTMLInputElement>('input'));
  const pwIdx = all.indexOf(pw);
  for (let i = pwIdx - 1; i >= 0; i--) {
    const el = all[i];
    if (el !== pw && el.type !== 'password' && el.value.trim() !== '') return el.value.trim();
  }
  return withValue[0]?.value.trim() ?? '';
}

function extractCredentials(form: HTMLFormElement): DetectedCredentials | null {
  // Pick a password field that actually has a value (skips empty confirm fields).
  const pw =
    Array.from(form.querySelectorAll<HTMLInputElement>('input[type="password"]')).find((p) => p.value) ?? null;
  if (!pw) return null;
  return { username: findUsernameValue(form, pw), password: pw.value };
}

// Render the in-page "save this login?" bar for a given offer and wire its
// buttons to the background. No secrets are shown — the password lives in the
// background's pending-save and is only consumed by CONFIRM_SAVE/UPDATE_SAVE.
// Each action swallows its own errors so a failed background call never becomes
// an uncaught promise rejection in the page console. Shared by the submit-time
// path and the post-navigation recovery path.
export function renderSaveBar(offer: SaveOffer): void {
  const send = async (type: string, p: Record<string, unknown> = {}) => {
    try {
      await browser.runtime.sendMessage({ type, payload: p });
    } catch (err) {
      console.warn('[passbubble] save action failed:', err);
    } finally {
      removeSaveBar();
    }
  };
  showSaveBar(
    {
      host: offer.host,
      username: offer.username,
      candidates: offer.candidates,
      suggestUpdateId: offer.suggestUpdateId,
    },
    {
      onSaveNew: () => void send(MessageType.CONFIRM_SAVE),
      onUpdate: (entryId) => void send(MessageType.UPDATE_SAVE, { entryId }),
      onDismiss: () => void send(MessageType.DISMISS_SAVE, { host: offer.host }),
      onNever: () => void send(MessageType.BLOCKLIST_ADD, { host: offer.host }),
    },
  );
}

// On a fresh page load, re-show the save bar if a pending save survived a
// navigation (it lives in storage.session, which the DOM-only bar does not).
// Only re-show when the pending save belongs to the page we are actually on, so
// the bar never appears on an unrelated domain after a cross-site redirect.
export async function recoverPendingSave(): Promise<void> {
  let pending: {
    url?: string;
    username?: string;
    candidates?: { id: string; username: string }[];
    suggestUpdateId?: string;
  } | null = null;
  try {
    pending = (await browser.runtime.sendMessage({
      type: MessageType.GET_PENDING_SAVE,
      payload: {},
    })) as typeof pending;
  } catch {
    return; // background unreachable — nothing to restore
  }
  if (!pending?.url) return;
  const host = normaliseHost(location.href);
  if (!host || normaliseHost(pending.url) !== host) return;
  renderSaveBar({
    host,
    username: pending.username ?? '',
    candidates: pending.candidates,
    suggestUpdateId: pending.suggestUpdateId,
  });
}

export function initSaveDetector(): void {
  document.addEventListener(
    'submit',
    async (e) => {
      const form = e.target as HTMLFormElement;
      const creds = extractCredentials(form);
      if (!creds) return;

      const host = normaliseHost(location.href);

      // The background owns storage + crypto. It decides whether to offer at all
      // (vault locked, blocklisted/dismissed host, or credentials identical to an
      // existing entry → no offer) and, on a site we already know, returns the
      // matching entries so we can offer "update" instead of only "new". Content
      // scripts must not touch storage.session (untrusted; holds tokens + keys).
      const resp = (await browser.runtime.sendMessage({
        type: MessageType.OFFER_SAVE,
        payload: {
          name: document.title || host,
          url: location.href,
          username: creds.username,
          password: creds.password,
        },
      })) as {
        ok?: boolean;
        candidates?: { id: string; username: string }[];
        suggestUpdateId?: string;
      };
      if (!resp?.ok) return;

      renderSaveBar({
        host,
        username: creds.username,
        candidates: resp.candidates,
        suggestUpdateId: resp.suggestUpdateId,
      });
    },
    true, // capture phase — collect values before form handlers clear them
  );
}

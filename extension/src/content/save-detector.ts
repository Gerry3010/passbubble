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

// Detects login attempts to offer saving new credentials. SPAs (Flutter web,
// React, Vue) log in via click handlers + fetch and never fire a native
// `submit`, so a submit listener alone misses them. Instead, credentials are
// snapshotted from the live fields as the user types (capture-phase composed
// events also reach inputs inside shadow DOM — Flutter web renders its text
// fields inside flt-glass-pane's shadow root), and a save is offered on any
// submit-shaped user action: native submit, Enter, a press outside a text
// field, or leaving the page with a fresh snapshot.
// Does NOT call preventDefault() — the save offer is non-blocking.

import browser from 'webextension-polyfill';
import { MessageType } from '../shared/constants.js';
import { normaliseHost } from '../shared/utils.js';
import { showSaveBar, removeSaveBar, saveBarContains } from './save-bar.js';

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

// A snapshot older than this is considered stale on pagehide — the password in
// the fields was probably left over, not part of a just-attempted login.
const SNAPSHOT_FRESH_MS = 30_000;
// SPAs fire several submit-shaped actions per login (mousedown + Enter +
// pagehide); the same credentials are only offered once per window.
const REOFFER_SUPPRESS_MS = 15_000;

const USERNAME_SELECTOR =
  'input[type="text"], input[type="email"], input[type="tel"], input[autocomplete="username"], input:not([type])';

// Heuristic: a field whose name/id/placeholder/aria-label looks like a login id.
function looksLikeUsername(i: HTMLInputElement): boolean {
  if (i.getAttribute('autocomplete') === 'username') return true;
  if (i.type === 'email') return true;
  const hay = `${i.name} ${i.id} ${i.placeholder} ${i.getAttribute('aria-label') ?? ''}`;
  return /user|e-?mail|login|account|phone|mobile|(^|[^a-z])name([^a-z]|$)/i.test(hay);
}

// The event's real target, resolved through shadow boundaries. `event.target`
// seen from window is retargeted to the shadow host; composedPath()[0] is the
// actual input inside the shadow root (open shadow only — closed roots stay
// retargeted, which is fine: our own save bar uses a closed root on purpose).
function composedTarget(e: Event): EventTarget | null {
  const path = e.composedPath?.();
  return (path && path.length > 0 ? path[0] : e.target) ?? null;
}

// The nearest scope to search for the username belonging to `pw`: its <form>
// when there is one, otherwise its root node (document or shadow root) —
// SPA login fields often have no <form> at all.
function scopeFor(pw: HTMLInputElement): ParentNode {
  return pw.closest('form') ?? (pw.getRootNode() as ParentNode);
}

// Walk backwards from `pw` in `inputs` order and return the first non-password
// value before it — the classic "username field sits above the password" case.
function nearestValueBefore(inputs: HTMLInputElement[], pw: HTMLInputElement): string {
  const pwIdx = inputs.indexOf(pw);
  for (let i = pwIdx - 1; i >= 0; i--) {
    const el = inputs[i];
    if (el !== pw && el.type !== 'password' && el.value.trim() !== '') return el.value.trim();
  }
  return '';
}

// Find the value the user actually typed as their username: a username-ish
// field WITH a value wins, then the nearest non-empty field before the password
// (scope first, then document), then any non-empty candidate.
function findUsernameValue(scope: ParentNode, pw: HTMLInputElement): string {
  const scoped = Array.from(scope.querySelectorAll<HTMLInputElement>(USERNAME_SELECTOR));
  const candidates = scoped.length
    ? scoped
    : Array.from(document.querySelectorAll<HTMLInputElement>(USERNAME_SELECTOR));
  const withValue = candidates.filter((i) => i.value.trim() !== '');

  const preferred = withValue.filter(looksLikeUsername);
  if (preferred.length) return preferred[preferred.length - 1].value.trim();

  const inScope = nearestValueBefore(Array.from(scope.querySelectorAll<HTMLInputElement>('input')), pw);
  if (inScope) return inScope;
  const inDoc = nearestValueBefore(Array.from(document.querySelectorAll<HTMLInputElement>('input')), pw);
  if (inDoc) return inDoc;
  return withValue[0]?.value.trim() ?? '';
}

function extractCredentials(form: HTMLFormElement): DetectedCredentials | null {
  // Pick a password field that actually has a value (skips empty confirm fields).
  const pw =
    Array.from(form.querySelectorAll<HTMLInputElement>('input[type="password"]')).find((p) => p.value) ?? null;
  if (!pw) return null;
  return { username: findUsernameValue(form, pw), password: pw.value };
}

// ---- Credential snapshot ----------------------------------------------------
// Password fields are collected as the user focuses/types in them (composed
// events, so shadow-DOM fields are seen too). The snapshot holds the last
// known credentials so they survive the SPA clearing its fields on login.

let snapshot: (DetectedCredentials & { ts: number }) | null = null;
const trackedPw = new Set<HTMLInputElement>();
let lastOffered: { key: string; ts: number } | null = null;

// Re-read the tracked fields. Returns true when a connected password field
// currently holds a value ("live"). When fields still exist but are all empty
// the user cleared them — drop the snapshot; when they were REMOVED (SPA
// navigated away) the snapshot is kept, that's exactly the case it exists for.
function refreshSnapshot(): boolean {
  let anyConnected = false;
  let live = false;
  for (const pw of Array.from(trackedPw)) {
    if (!pw.isConnected) {
      trackedPw.delete(pw);
      continue;
    }
    anyConnected = true;
    if (pw.value) {
      snapshot = { username: findUsernameValue(scopeFor(pw), pw), password: pw.value, ts: Date.now() };
      live = true;
    }
  }
  if (!live && anyConnected) snapshot = null;
  return live;
}

function onFieldEvent(e: Event): void {
  const t = composedTarget(e);
  if (t instanceof HTMLInputElement && t.type === 'password') trackedPw.add(t);
  if (trackedPw.size > 0) refreshSnapshot();
}

// ---- Offer ------------------------------------------------------------------

// Render the in-page "save this login?" bar for a given offer and wire its
// buttons to the background. No secrets are shown — the password lives in the
// background's pending-save and is only consumed by CONFIRM_SAVE/UPDATE_SAVE.
// Each action swallows its own errors so a failed background call never becomes
// an uncaught promise rejection in the page console. Shared by the trigger
// paths and the post-navigation recovery path.
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

// Ask the background whether to offer saving `creds`. The background owns
// storage + crypto: it decides (vault locked, blocklisted/dismissed host, or
// credentials identical to an existing entry → no offer) and, on a known site,
// returns the matching entries so the bar can offer "update" instead of only
// "new". `render: false` is the pagehide path — the bar cannot survive the
// navigation anyway; recoverPendingSave() re-shows it on the next page.
function offer(creds: DetectedCredentials, render: boolean): void {
  const host = normaliseHost(location.href);
  const key = `${host}|${creds.username}|${creds.password}`;
  const now = Date.now();
  if (lastOffered && lastOffered.key === key && now - lastOffered.ts < REOFFER_SUPPRESS_MS) return;
  lastOffered = { key, ts: now };

  void (async () => {
    interface OfferResponse {
      ok?: boolean;
      candidates?: { id: string; username: string }[];
      suggestUpdateId?: string;
    }
    let resp: OfferResponse | null = null;
    try {
      resp = (await browser.runtime.sendMessage({
        type: MessageType.OFFER_SAVE,
        payload: {
          name: document.title || host,
          url: location.href,
          username: creds.username,
          password: creds.password,
        },
      })) as OfferResponse | null;
    } catch {
      return; // background unreachable (e.g. extension reloaded)
    }
    if (!render || !resp?.ok) return;
    renderSaveBar({
      host,
      username: creds.username,
      candidates: resp.candidates,
      suggestUpdateId: resp.suggestUpdateId,
    });
  })();
}

// On a fresh page load, re-show the save bar if a pending save survived a
// navigation (it lives in storage.session, which the DOM-only bar does not).
// Only re-show when the pending save belongs to the page we are actually on, so
// the bar never appears on an unrelated domain after a cross-site redirect.
export async function recoverPendingSave(): Promise<void> {
  interface PendingSave {
    url?: string;
    username?: string;
    candidates?: { id: string; username: string }[];
    suggestUpdateId?: string;
  }
  let pending: PendingSave | null = null;
  try {
    pending = (await browser.runtime.sendMessage({
      type: MessageType.GET_PENDING_SAVE,
      payload: {},
    })) as PendingSave | null;
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

// ---- Triggers ---------------------------------------------------------------

function onSubmit(e: Event): void {
  const form = e.target as HTMLFormElement;
  if (!(form instanceof HTMLFormElement)) return;
  const creds = extractCredentials(form);
  if (!creds) return;
  snapshot = { ...creds, ts: Date.now() };
  offer(creds, true);
}

function onEnterKey(e: KeyboardEvent): void {
  if (e.key !== 'Enter') return;
  if (!(composedTarget(e) instanceof HTMLInputElement)) return;
  if (refreshSnapshot() && snapshot) offer(snapshot, true);
}

// A press anywhere that is not a text-entry control counts as a possible
// "log in" action while credentials are sitting in the fields. mousedown, not
// click: it fires BEFORE the SPA's click handler clears the fields. This
// deliberately does not require a <button> — Flutter web paints its buttons
// onto a canvas, so there is no matchable element at all.
function onMouseDown(e: MouseEvent): void {
  const t = composedTarget(e);
  if (!(t instanceof Element)) return;
  if (saveBarContains(t)) return; // pressing our own save bar must not re-offer
  if (t.closest('input, textarea, select') || (t instanceof HTMLElement && t.isContentEditable)) return;
  if (refreshSnapshot() && snapshot) offer(snapshot, true);
}

// Leaving the page (full navigation after a classic login, or an SPA route
// change that replaces the document) with a fresh snapshot: offer without
// rendering — the pending save survives in storage.session and
// recoverPendingSave() re-shows the bar on the next load.
function onPageHide(): void {
  if (!snapshot?.password) return;
  if (Date.now() - snapshot.ts > SNAPSHOT_FRESH_MS) return;
  offer(snapshot, false);
}

let listenersRegistered = false;

export function initSaveDetector(): void {
  // Reset detector state; listeners are registered once per document.
  snapshot = null;
  trackedPw.clear();
  lastOffered = null;
  if (listenersRegistered) return;
  listenersRegistered = true;

  // All capture-phase on window so values are read before page handlers run
  // and composed events from open shadow roots are still seen.
  window.addEventListener('input', onFieldEvent, true);
  window.addEventListener('focusin', onFieldEvent, true);
  window.addEventListener('keydown', onEnterKey, true);
  window.addEventListener('mousedown', onMouseDown, true);
  window.addEventListener('pagehide', onPageHide);
  document.addEventListener('submit', onSubmit, true);
}

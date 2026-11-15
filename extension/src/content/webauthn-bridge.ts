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

/// <reference types="vite/client" />

// Bridge content script (isolated world, top frame, document_start; Chrome
// and Firefox). Carries passkey requests from the page script to the
// background and the outcome back. It accepts only messages the top window
// posted to itself and takes nothing from them but the request: the origin
// and rp id are worked out by the background from the browser's sender
// information. Background request ids and dialog tokens stay here; the page
// sees neither. Results arrive later via tabs.sendMessage, so no message
// channel to the service worker stays open while the dialog is up.

import browser from 'webextension-polyfill';
import { MessageType } from '../shared/constants.js';
import { readPasskeySettings, siteListed } from '../shared/passkey-settings.js';
import { decodeMessage, encodeMessage, type BridgeMessage, type RequestOutcome, type SerializedRequest } from '../webauthn/protocol.js';
import { showPasskeyDialog, type PasskeyDialogHandle } from './passkey-host.js';

interface Outstanding {
  pageId: string;
  /** The background's id, once PASSKEY_BEGIN answered. */
  bgId?: string;
  cancelled: boolean;
  dialog?: PasskeyDialogHandle;
}

type BeginReply =
  | { action: 'native' }
  | { action: 'ui'; id: string; uiToken: string }
  | { action: 'conditional'; id: string }
  | { action: 'error'; name: string; message: string };

const NOT_ALLOWED = 'The operation either timed out or was not allowed.';

/** https, or http on localhost; no IP address; not Firefox for Android (no
 * room for the dialog there). The background decides for real. */
function passkeyCapablePage(location: Location): boolean {
  const host = location.hostname;
  if (!host || host.startsWith('[') || /^\d{1,3}(\.\d{1,3}){3}$/.test(host)) return false;
  if (/Android/i.test(navigator.userAgent)) return false;
  return location.protocol === 'https:' || (location.protocol === 'http:' && host === 'localhost');
}

export interface BridgeHandle {
  stop: () => void;
}

export function startBridge(win: Window = window): BridgeHandle {
  const outstanding = new Map<string, Outstanding>();
  let enabled: boolean | null = null;
  let stopped = false;

  function alive(): boolean {
    try {
      return !!browser.runtime?.id;
    } catch {
      return false;
    }
  }

  function post(msg: BridgeMessage): void {
    try {
      win.postMessage(encodeMessage('bridge', msg), win.location.origin);
    } catch {
      // opaque origin: the page script cannot be reached either
    }
  }

  function finish(o: Outstanding, outcome: RequestOutcome, keepDialog = false): void {
    outstanding.delete(o.pageId);
    if (!keepDialog) o.dialog?.remove();
    if (outcome.kind === 'credential') post({ kind: 'credential', id: o.pageId, credential: outcome.credential });
    else if (outcome.kind === 'error') post({ kind: 'error', id: o.pageId, name: outcome.name, message: outcome.message });
    else post({ kind: 'native', id: o.pageId });
  }

  function cancelInBackground(bgId: string | undefined): void {
    if (!bgId || !alive()) return;
    browser.runtime.sendMessage({ type: MessageType.PASSKEY_CANCEL, payload: { id: bgId } }).catch(() => undefined);
  }

  async function handleRequest(pageId: string, request: SerializedRequest): Promise<void> {
    if (outstanding.has(pageId)) return;
    const o: Outstanding = { pageId, cancelled: false };
    outstanding.set(pageId, o);
    post({ kind: 'ack', id: pageId });
    let reply: BeginReply | null = null;
    try {
      reply = (await browser.runtime.sendMessage({
        type: MessageType.PASSKEY_BEGIN,
        payload: { request, selfOrigin: self.origin },
      })) as BeginReply | null;
    } catch {
      reply = null;
    }
    if (o.cancelled) {
      if (reply && 'id' in reply) cancelInBackground(reply.id);
      outstanding.delete(pageId);
      return;
    }
    switch (reply?.action) {
      case 'ui':
        o.bgId = reply.id;
        o.dialog = showPasskeyDialog(reply.uiToken, {
          onGone: () => {
            if (!outstanding.has(pageId)) return;
            cancelInBackground(o.bgId);
            finish(o, { kind: 'error', name: 'NotAllowedError', message: NOT_ALLOWED });
          },
          onClose: () => {
            o.dialog?.remove();
            if (!outstanding.has(pageId)) return;
            cancelInBackground(o.bgId);
            finish(o, { kind: 'error', name: 'NotAllowedError', message: NOT_ALLOWED });
          },
        });
        break;
      case 'conditional':
        o.bgId = reply.id;
        break;
      case 'error':
        finish(o, { kind: 'error', name: String(reply.name), message: String(reply.message) });
        break;
      default:
        finish(o, { kind: 'native' });
    }
  }

  function handleCancel(pageId: string): void {
    const o = outstanding.get(pageId);
    if (!o) return;
    o.cancelled = true;
    o.dialog?.remove();
    if (o.bgId) {
      cancelInBackground(o.bgId);
      outstanding.delete(pageId);
    }
    // Otherwise PASSKEY_BEGIN is still under way; handleRequest cleans up.
  }

  function onPageMessage(event: MessageEvent): void {
    if (stopped || event.source !== win) return;
    const msg = decodeMessage(event.data, 'page');
    if (!msg) return;
    if (!alive()) {
      // An orphaned bridge after an extension update: stay silent, so the
      // page script falls back to the browser (or the new bridge answers).
      stop();
      return;
    }
    if (msg.kind === 'hello') post({ kind: 'ready', enabled });
    else if (msg.kind === 'request' && typeof msg.id === 'string' && msg.id) void handleRequest(msg.id, msg.request);
    else if (msg.kind === 'cancel' && typeof msg.id === 'string') handleCancel(msg.id);
  }

  function onBackgroundMessage(message: unknown): undefined {
    const m = message as { type?: unknown; payload?: { id?: unknown; outcome?: RequestOutcome; keepDialog?: unknown } } | null;
    if (stopped || m?.type !== MessageType.PASSKEY_RESULT || !m.payload) return undefined;
    const { id, outcome, keepDialog } = m.payload;
    if (typeof id !== 'string' || !outcome || typeof outcome.kind !== 'string') return undefined;
    for (const o of outstanding.values()) {
      if (o.bgId === id) {
        finish(o, outcome, keepDialog === true);
        break;
      }
    }
    return undefined;
  }

  function onPageHide(): void {
    for (const o of [...outstanding.values()]) {
      o.cancelled = true;
      cancelInBackground(o.bgId);
      finish(o, { kind: 'error', name: 'NotAllowedError', message: NOT_ALLOWED });
    }
  }

  // Whether Passbubble may answer here, for the page script's static
  // PublicKeyCredential methods. Read from storage.local directly, so a page
  // load does not wake the service worker; PASSKEY_BEGIN checks it all again.
  async function fetchStatus(): Promise<void> {
    let on = false;
    try {
      const settings = await readPasskeySettings();
      on = settings.enabled && passkeyCapablePage(win.location) && !siteListed(win.location.hostname, settings.blocklist);
    } catch {
      on = false;
    }
    enabled = on;
    if (!stopped) post({ kind: 'status', enabled });
  }

  function stop(): void {
    if (stopped) return;
    stopped = true;
    win.removeEventListener('message', onPageMessage);
    win.removeEventListener('pagehide', onPageHide);
    try {
      browser.runtime.onMessage.removeListener(onBackgroundMessage);
    } catch {
      // runtime already gone
    }
    for (const o of outstanding.values()) o.dialog?.remove();
    outstanding.clear();
  }

  win.addEventListener('message', onPageMessage);
  win.addEventListener('pagehide', onPageHide);
  browser.runtime.onMessage.addListener(onBackgroundMessage);
  post({ kind: 'ready', enabled });
  void fetchStatus();
  return { stop };
}

// Started by the bundle; tests start their own.
if (import.meta.env.MODE !== 'test' && typeof window !== 'undefined' && window.top === window) startBridge(window);

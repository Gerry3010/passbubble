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

// The isolated-world bridge between the page script and the background.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const local = vi.hoisted(() => new Map<string, unknown>());
const runtime = vi.hoisted(() => ({
  id: 'ext' as string | undefined,
  getURL: (p: string) => `chrome-extension://ext/${p}`,
  sendMessage: vi.fn(),
  listeners: [] as Array<(msg: unknown) => unknown>,
}));

vi.mock('webextension-polyfill', () => ({
  default: {
    storage: {
      local: {
        get: async (keys: string[]) => Object.fromEntries(keys.filter((k) => local.has(k)).map((k) => [k, local.get(k)])),
      },
    },
    runtime: {
      get id() {
        return runtime.id;
      },
      getURL: runtime.getURL,
      sendMessage: runtime.sendMessage,
      onMessage: {
        addListener: (fn: (msg: unknown) => unknown) => runtime.listeners.push(fn),
        removeListener: (fn: (msg: unknown) => unknown) => {
          runtime.listeners = runtime.listeners.filter((l) => l !== fn);
        },
      },
    },
  },
}));

import { startBridge, type BridgeHandle } from '../webauthn-bridge.js';
import { MessageType, STORAGE_KEYS } from '../../shared/constants.js';
import { decodeMessage, encodeMessage, type BridgeMessage, type PageMessage, type SerializedRequest } from '../../webauthn/protocol.js';

// What the bridge posts to the page (with a real top window's event.source).
let posted: BridgeMessage[] = [];
let rawPosted: string[] = [];
window.postMessage = ((data: unknown) => {
  rawPosted.push(String(data));
  const msg = decodeMessage(data, 'bridge');
  if (msg) posted.push(msg);
  setTimeout(() => window.dispatchEvent(new MessageEvent('message', { data, source: window })), 0);
}) as typeof window.postMessage;

// The dialog's closed shadow root, for the tests to look into.
let shadowRoots: ShadowRoot[] = [];
const attachShadow = HTMLElement.prototype.attachShadow;
HTMLElement.prototype.attachShadow = function (this: HTMLElement, init: ShadowRootInit) {
  const root = attachShadow.call(this, init);
  shadowRoots.push(root);
  return root;
};

const REQUEST: SerializedRequest = {
  type: 'get',
  options: { challenge: 'AAAA', rpId: 'example.com', allowCredentials: [], hints: [], extensions: { names: [] } },
};

function fromPage(msg: PageMessage, source: MessageEventSource | null = window): void {
  window.dispatchEvent(new MessageEvent('message', { data: encodeMessage('page', msg), source }));
}

function pushResult(id: string, outcome: unknown, keepDialog = false): void {
  for (const l of runtime.listeners) l({ type: MessageType.PASSKEY_RESULT, payload: { id, outcome, keepDialog } });
}

const begins = () => runtime.sendMessage.mock.calls.filter(([m]) => m.type === MessageType.PASSKEY_BEGIN).map(([m]) => m.payload);
const cancels = () => runtime.sendMessage.mock.calls.filter(([m]) => m.type === MessageType.PASSKEY_CANCEL).map(([m]) => m.payload.id);
const host = () => document.querySelector('passbubble-passkey');
const EXTENSION_ORIGIN = new URL('chrome-extension://ext/').origin; // as passkey-host computes it

// jsdom gives an iframe inside a shadow root no window; lend it a real one.
function dialogFrame(): { iframe: HTMLIFrameElement; win: Window } {
  const iframe = shadowRoots.at(-1)!.querySelector('iframe')!;
  const stand = document.createElement('iframe');
  document.body.appendChild(stand);
  const win = stand.contentWindow!;
  Object.defineProperty(iframe, 'contentWindow', { value: win, configurable: true });
  return { iframe, win };
}

let beginReply: unknown;
let handle: BridgeHandle | null = null;

beforeEach(() => {
  local.clear();
  posted = [];
  rawPosted = [];
  shadowRoots = [];
  runtime.id = 'ext';
  runtime.listeners = [];
  beginReply = { action: 'native' };
  runtime.sendMessage.mockReset().mockImplementation(async (m: { type: string }) => {
    if (m.type === MessageType.PASSKEY_BEGIN) return beginReply;
    return { ok: true };
  });
});

afterEach(() => {
  handle?.stop();
  handle = null;
  document.body.innerHTML = '';
});

describe('handshake', () => {
  it('says it is ready, then reports the feature state; answers hello', async () => {
    handle = startBridge(window);
    expect(posted[0]).toEqual({ kind: 'ready', enabled: null });
    await vi.waitFor(() => expect(posted).toContainEqual({ kind: 'status', enabled: true }));
    fromPage({ kind: 'hello' });
    expect(posted.at(-1)).toEqual({ kind: 'ready', enabled: true });
  });

  it('reports the feature off when switched off or on a listed site, without waking the background', async () => {
    local.set(STORAGE_KEYS.PASSKEYS_ENABLED, false);
    handle = startBridge(window);
    await vi.waitFor(() => expect(posted).toContainEqual({ kind: 'status', enabled: false }));
    handle.stop();
    posted = [];
    local.set(STORAGE_KEYS.PASSKEYS_ENABLED, true);
    local.set(STORAGE_KEYS.PASSKEY_BLOCKLIST, ['example.com']);
    handle = startBridge(window);
    await vi.waitFor(() => expect(posted).toContainEqual({ kind: 'status', enabled: false }));
    expect(runtime.sendMessage).not.toHaveBeenCalled();
  });
});

describe('accepting page messages', () => {
  it('acknowledges and forwards only what the top window posted to itself', async () => {
    handle = startBridge(window);
    const frame = document.createElement('iframe');
    document.body.appendChild(frame);
    fromPage({ kind: 'request', id: 'p1', request: REQUEST }, frame.contentWindow);
    fromPage({ kind: 'request', id: 'p2', request: REQUEST }, null);
    window.dispatchEvent(new MessageEvent('message', { data: { kind: 'request', id: 'p3' }, source: window }));
    window.dispatchEvent(new MessageEvent('message', { data: 'x'.repeat(70_000), source: window }));
    await new Promise((r) => setTimeout(r, 5));
    expect(begins()).toEqual([]);
    expect(posted.filter((m) => m.kind === 'ack')).toEqual([]);

    fromPage({ kind: 'request', id: 'p4', request: REQUEST });
    expect(posted).toContainEqual({ kind: 'ack', id: 'p4' });
    await vi.waitFor(() => expect(begins()).toHaveLength(1));
  });

  it('sends the request and its own origin, never an origin from the page', async () => {
    handle = startBridge(window);
    const sneaky = { ...REQUEST, origin: 'https://bank.example', rpId: 'bank.example' } as unknown as SerializedRequest;
    window.dispatchEvent(
      new MessageEvent('message', {
        data: JSON.stringify({ channel: 'passbubble:webauthn:1', dir: 'page', msg: { kind: 'request', id: 'p1', request: sneaky, origin: 'https://bank.example' } }),
        source: window,
      }),
    );
    await vi.waitFor(() => expect(begins()).toHaveLength(1));
    const [payload] = begins();
    expect(Object.keys(payload).sort()).toEqual(['request', 'selfOrigin']);
    expect(payload.selfOrigin).toBe(self.origin);
    expect(payload.selfOrigin).toBe('https://example.com');
  });

  it('stays silent once the extension context is gone', async () => {
    handle = startBridge(window);
    runtime.id = undefined;
    fromPage({ kind: 'request', id: 'p1', request: REQUEST });
    fromPage({ kind: 'hello' });
    await new Promise((r) => setTimeout(r, 5));
    expect(posted.filter((m) => m.kind === 'ack' || (m.kind === 'ready' && m.enabled !== null))).toEqual([]);
    expect(begins()).toEqual([]);
  });
});

describe('outcomes', () => {
  it('passes "native" and errors from PASSKEY_BEGIN on', async () => {
    handle = startBridge(window);
    fromPage({ kind: 'request', id: 'p1', request: REQUEST });
    await vi.waitFor(() => expect(posted).toContainEqual({ kind: 'native', id: 'p1' }));
    beginReply = { action: 'error', name: 'NotAllowedError', message: 'busy' };
    fromPage({ kind: 'request', id: 'p2', request: REQUEST });
    await vi.waitFor(() => expect(posted).toContainEqual({ kind: 'error', id: 'p2', name: 'NotAllowedError', message: 'busy' }));
  });

  it('shows the dialog, keeps the token and background id to itself, and relays the result', async () => {
    beginReply = { action: 'ui', id: 'bg-secret-id', uiToken: 'token-secret-0123456789' };
    handle = startBridge(window);
    fromPage({ kind: 'request', id: 'p1', request: REQUEST });
    await vi.waitFor(() => expect(host()).not.toBeNull());
    expect(host()!.getAttribute('popover')).toBe('manual');
    const { iframe, win } = dialogFrame();
    expect(iframe.src).toBe('chrome-extension://ext/passkey-dialog/index.html');
    const toDialog = vi.spyOn(win, 'postMessage').mockImplementation(() => undefined);
    iframe.dispatchEvent(new Event('load'));
    expect(toDialog).toHaveBeenCalledWith('token-secret-0123456789', 'chrome-extension://ext/');
    // The dialog's "ready" (sent once it listens) gets the token again; the page's does not.
    window.dispatchEvent(new MessageEvent('message', { data: { type: 'passkey-dialog', action: 'ready' }, source: window, origin: EXTENSION_ORIGIN }));
    expect(toDialog).toHaveBeenCalledTimes(1);
    window.dispatchEvent(new MessageEvent('message', { data: { type: 'passkey-dialog', action: 'ready' }, source: win, origin: EXTENSION_ORIGIN }));
    expect(toDialog).toHaveBeenCalledTimes(2);

    pushResult('someone-else', { kind: 'native' });
    expect(posted.some((m) => m.kind === 'native')).toBe(false);

    const credential = { type: 'get', id: 'AQ', clientDataJSON: 'e30', authenticatorData: 'AA', signature: 'AA', transports: [], clientExtensionResults: {} };
    pushResult('bg-secret-id', { kind: 'credential', credential });
    expect(posted).toContainEqual({ kind: 'credential', id: 'p1', credential });
    expect(host()).toBeNull();
    expect(rawPosted.join('\n')).not.toMatch(/token-secret|bg-secret-id/);
  });

  it('keeps the dialog open for an error it shows, until the dialog asks to close', async () => {
    beginReply = { action: 'ui', id: 'bg1', uiToken: 'token-0123456789abcdef' };
    handle = startBridge(window);
    fromPage({ kind: 'request', id: 'p1', request: REQUEST });
    await vi.waitFor(() => expect(host()).not.toBeNull());
    pushResult('bg1', { kind: 'error', name: 'NotAllowedError', message: 'could not save' }, true);
    expect(posted).toContainEqual({ kind: 'error', id: 'p1', name: 'NotAllowedError', message: 'could not save' });
    expect(host()).not.toBeNull();
    const { win } = dialogFrame();
    // Only the dialog itself may close it.
    window.dispatchEvent(new MessageEvent('message', { data: { type: 'passkey-dialog', action: 'close' }, source: window, origin: EXTENSION_ORIGIN }));
    expect(host()).not.toBeNull();
    window.dispatchEvent(new MessageEvent('message', { data: { type: 'passkey-dialog', action: 'close' }, source: win, origin: EXTENSION_ORIGIN }));
    expect(host()).toBeNull();
    expect(cancels()).toEqual([]);
  });

  it('withdraws the request when the page cancels', async () => {
    beginReply = { action: 'ui', id: 'bg1', uiToken: 'token-0123456789abcdef' };
    handle = startBridge(window);
    fromPage({ kind: 'request', id: 'p1', request: REQUEST });
    await vi.waitFor(() => expect(host()).not.toBeNull());
    fromPage({ kind: 'cancel', id: 'p1' });
    expect(host()).toBeNull();
    expect(cancels()).toEqual(['bg1']);
  });

  it('withdraws a request the page cancelled while PASSKEY_BEGIN was under way', async () => {
    let answer: (v: unknown) => void = () => {};
    runtime.sendMessage.mockImplementation(async (m: { type: string }) =>
      m.type === MessageType.PASSKEY_BEGIN ? new Promise((r) => (answer = r)) : { ok: true },
    );
    handle = startBridge(window);
    fromPage({ kind: 'request', id: 'p1', request: REQUEST });
    await vi.waitFor(() => expect(begins()).toHaveLength(1));
    fromPage({ kind: 'cancel', id: 'p1' });
    answer({ action: 'ui', id: 'bg1', uiToken: 'token-0123456789abcdef' });
    await vi.waitFor(() => expect(cancels()).toEqual(['bg1']));
    expect(host()).toBeNull();
  });

  it('cancels with NotAllowedError when the page removes or restyles the dialog', async () => {
    beginReply = { action: 'ui', id: 'bg1', uiToken: 'token-0123456789abcdef' };
    handle = startBridge(window);
    fromPage({ kind: 'request', id: 'p1', request: REQUEST });
    await vi.waitFor(() => expect(host()).not.toBeNull());
    (host() as HTMLElement).style.opacity = '0';
    await vi.waitFor(() => expect(posted).toContainEqual(expect.objectContaining({ kind: 'error', id: 'p1', name: 'NotAllowedError' })));
    expect(cancels()).toEqual(['bg1']);
    expect(host()).toBeNull();

    beginReply = { action: 'ui', id: 'bg2', uiToken: 'token-0123456789abcdef' };
    fromPage({ kind: 'request', id: 'p2', request: REQUEST });
    await vi.waitFor(() => expect(host()).not.toBeNull());
    host()!.remove();
    await vi.waitFor(() => expect(cancels()).toEqual(['bg1', 'bg2']));
  });

  it('cancels everything open when the page goes away', async () => {
    beginReply = { action: 'conditional', id: 'bg-cond' };
    handle = startBridge(window);
    fromPage({ kind: 'request', id: 'p1', request: { ...REQUEST, mediation: 'conditional' } });
    await vi.waitFor(() => expect(begins()).toHaveLength(1));
    await new Promise((r) => setTimeout(r, 0));
    window.dispatchEvent(new Event('pagehide'));
    expect(cancels()).toEqual(['bg-cond']);
    expect(posted).toContainEqual(expect.objectContaining({ kind: 'error', id: 'p1' }));
  });
});

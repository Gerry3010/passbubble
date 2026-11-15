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

// Page script (MAIN world, top frame, document_start; Chrome and Firefox).
// Wraps navigator.credentials.create/get so Passbubble can answer passkey
// requests. It holds no secrets and makes no decisions about trust: it
// serializes the page's options, hands them to the bridge content script via
// window.postMessage and turns the answer back into a credential. Anything it
// cannot or should not answer goes to the browser with the page's original
// options and signal — including when no bridge answers (an orphaned page
// script after an extension update). The background checks the origin.
//
// Must not import webextension-polyfill or anything that touches `browser`.

import {
  ACK_TIMEOUT_MS,
  STATUS_TIMEOUT_MS,
  clampTimeout,
  decodeMessage,
  encodeMessage,
  passthroughReason,
  type PageMessage,
  type RequestOutcome,
  type SerializedRequest,
} from '../webauthn/protocol.js';
import {
  buildCredential,
  captureBuiltins,
  serializeCreationOptions,
  serializeRequestOptions,
  toBase64url,
  type PageBuiltins,
} from './webauthn-credential.js';

type PageWindow = Window & typeof globalThis;
type CredentialFn = (options?: unknown) => Promise<Credential | null>;
type StaticFn = (...args: unknown[]) => Promise<unknown>;

export interface ShimOptions {
  ackTimeoutMs?: number;
  statusTimeoutMs?: number;
}

const NATIVE: RequestOutcome = { kind: 'native' };
const TIMED_OUT = 'The operation either timed out or was not allowed. See: https://www.w3.org/TR/webauthn-2/#sctn-privacy-considerations-client.';

interface Waiting {
  acked: boolean;
  settle: (outcome: RequestOutcome) => void;
}

interface OpenRequest {
  promise: Promise<RequestOutcome>;
  /** Withdraws the request (tells the bridge); the promise settles as native. */
  cancel: () => void;
}

/**
 * Installs the wrapper on `win` and returns a function that removes it again,
 * or null when there is nothing to wrap (no WebAuthn, insecure context).
 */
export function installPasskeyShim(win: PageWindow = window, options: ShimOptions = {}): (() => void) | null {
  let container: CredentialsContainer | undefined;
  try {
    container = win.navigator?.credentials;
  } catch {
    return null;
  }
  const PKC = (win as { PublicKeyCredential?: unknown }).PublicKeyCredential as
    | (Record<string, unknown> & (new () => PublicKeyCredential))
    | undefined;
  if (!container || typeof container.get !== 'function' || typeof container.create !== 'function') return null;
  if (typeof PKC !== 'function' || win.isSecureContext === false) return null;

  const B: PageBuiltins = captureBuiltins(win);
  const creds = container;
  const ackTimeout = options.ackTimeoutMs ?? ACK_TIMEOUT_MS;
  const statusTimeout = options.statusTimeoutMs ?? STATUS_TIMEOUT_MS;
  // Possibly another password manager's wrapper; it stays in the chain.
  const origGet = creds.get as CredentialFn;
  const origCreate = creds.create as CredentialFn;
  const ownGet = B.getOwnPropertyDescriptor(creds, 'get');
  const ownCreate = B.getOwnPropertyDescriptor(creds, 'create');

  let bridgeReady = false;
  let enabled: boolean | null = null;
  const queued: string[] = [];
  const waiting = new Map<string, Waiting>();
  const statusWaiters: Array<() => void> = [];

  function post(data: string): void {
    try {
      B.postMessage(data, win.location.origin);
    } catch {
      // opaque origin: there is no bridge to talk to
    }
  }

  function send(msg: PageMessage): void {
    const data = encodeMessage('page', msg);
    if (!bridgeReady && msg.kind !== 'hello') queued.push(data);
    else post(data);
  }

  function setEnabled(value: boolean): void {
    enabled = value;
    for (const wake of statusWaiters.splice(0)) wake();
  }

  function onMessage(event: MessageEvent): void {
    if (event.source !== win) return;
    const msg = decodeMessage(event.data, 'bridge');
    if (!msg) return;
    switch (msg.kind) {
      case 'ready':
        if (typeof msg.enabled === 'boolean') setEnabled(msg.enabled);
        if (!bridgeReady) {
          bridgeReady = true;
          for (const data of queued.splice(0)) post(data);
        }
        break;
      case 'status':
        if (typeof msg.enabled === 'boolean') setEnabled(msg.enabled);
        break;
      case 'ack': {
        const w = waiting.get(msg.id);
        if (w) w.acked = true;
        break;
      }
      case 'native':
        waiting.get(msg.id)?.settle(NATIVE);
        break;
      case 'error':
        waiting.get(msg.id)?.settle({ kind: 'error', name: String(msg.name), message: String(msg.message) });
        break;
      case 'credential':
        waiting.get(msg.id)?.settle({ kind: 'credential', credential: msg.credential });
        break;
    }
  }
  B.addEventListener('message', onMessage as EventListener, true);

  function newId(): string {
    return toBase64url(B, B.getRandomValues(new B.Uint8Array(12)));
  }

  function abortReason(signal: AbortSignal): unknown {
    const reason = (signal as { reason?: unknown }).reason;
    return reason !== undefined ? reason : new B.DOMException('The operation was aborted.', 'AbortError');
  }

  /** Sends a request to the bridge. Settles as native when the bridge does not
   * acknowledge in time; rejects with the abort reason when the page aborts. */
  function openRequest(req: SerializedRequest, signal: AbortSignal | undefined, deadlineMs: number | null): OpenRequest {
    const id = newId();
    let done = false;
    let ackTimer: number | undefined;
    let deadlineTimer: number | undefined;
    let settle: (outcome: RequestOutcome) => void = () => {};
    let onAbort: () => void = () => {};
    const promise = new B.Promise<RequestOutcome>((resolve, reject) => {
      const cleanup = () => {
        done = true;
        waiting.delete(id);
        B.clearTimeout(ackTimer);
        B.clearTimeout(deadlineTimer);
        signal?.removeEventListener('abort', onAbort);
      };
      settle = (outcome) => {
        if (done) return;
        cleanup();
        resolve(outcome);
      };
      onAbort = () => {
        if (done) return;
        cleanup();
        send({ kind: 'cancel', id });
        reject(abortReason(signal!));
      };
      if (enabled === false) {
        settle(NATIVE);
        return;
      }
      const entry: Waiting = { acked: false, settle };
      waiting.set(id, entry);
      signal?.addEventListener('abort', onAbort);
      ackTimer = B.setTimeout(() => {
        if (done || entry.acked) return;
        send({ kind: 'cancel', id });
        settle(NATIVE);
      }, ackTimeout);
      if (deadlineMs !== null) {
        deadlineTimer = B.setTimeout(() => {
          if (done) return;
          send({ kind: 'cancel', id });
          settle({ kind: 'error', name: 'NotAllowedError', message: TIMED_OUT });
        }, deadlineMs);
      }
      send({ kind: 'request', id, request: req });
    });
    return {
      promise,
      cancel: () => {
        if (done) return;
        send({ kind: 'cancel', id });
        settle(NATIVE);
      },
    };
  }

  async function finish(outcome: RequestOutcome, type: 'create' | 'get', native: () => Promise<Credential | null>) {
    if (outcome.kind === 'native') return native();
    if (outcome.kind === 'error') throw new B.DOMException(outcome.message, outcome.name);
    if (outcome.credential?.type !== type) throw new B.DOMException(TIMED_OUT, 'NotAllowedError');
    return buildCredential(B, outcome.credential);
  }

  /** The page's options, or null when reading them already failed. */
  function readOptions(opts: unknown): { pk: unknown; signal?: AbortSignal; mediation?: string } | null {
    try {
      const o = opts as { publicKey?: unknown; signal?: AbortSignal | null; mediation?: unknown } | undefined | null;
      if (!o || typeof o !== 'object') return { pk: undefined };
      const mediation = o.mediation;
      return {
        pk: o.publicKey,
        signal: o.signal ?? undefined,
        ...(typeof mediation === 'string' ? { mediation } : {}),
      };
    } catch {
      return null;
    }
  }

  async function create(opts?: unknown): Promise<Credential | null> {
    const native = () => B.apply(origCreate, creds, [opts]) as Promise<Credential | null>;
    const read = readOptions(opts);
    if (!read?.pk) return native();
    let req: SerializedRequest;
    try {
      req = {
        type: 'create',
        ...(read.mediation ? { mediation: read.mediation } : {}),
        options: serializeCreationOptions(B, read.pk),
      };
    } catch {
      return native();
    }
    if (passthroughReason(req) !== null) return native();
    if (read.signal?.aborted) throw abortReason(read.signal);
    const outcome = await openRequest(req, read.signal, clampTimeout(req.options.timeout)).promise;
    return finish(outcome, 'create', native);
  }

  async function get(opts?: unknown): Promise<Credential | null> {
    const native = () => B.apply(origGet, creds, [opts]) as Promise<Credential | null>;
    const read = readOptions(opts);
    if (!read?.pk) return native();
    let req: SerializedRequest;
    try {
      req = {
        type: 'get',
        ...(read.mediation ? { mediation: read.mediation } : {}),
        options: serializeRequestOptions(B, read.pk),
      };
    } catch {
      return native();
    }
    if (passthroughReason(req) !== null) return native();
    if (read.signal?.aborted) throw abortReason(read.signal);
    if (read.mediation === 'conditional') return conditionalGet(opts as object, req, read.signal);
    const outcome = await openRequest(req, read.signal, clampTimeout(req.options.timeout)).promise;
    return finish(outcome, 'get', native);
  }

  /**
   * Autofill-style sign-in: the browser's own conditional request and ours run
   * side by side. Whichever produces a credential first wins; the other is
   * withdrawn. While ours is open, the browser's errors (other than the
   * page's abort) are held back.
   */
  function conditionalGet(opts: object, req: SerializedRequest, signal: AbortSignal | undefined): Promise<Credential | null> {
    return new B.Promise<Credential | null>((resolve, reject) => {
      const controller = new B.AbortController();
      let done = false;
      let oursOpen = true;
      let nativeFailure: { error: unknown } | null = null;
      const onPageAbort = () => {
        if (done) return;
        done = true;
        const reason = abortReason(signal!);
        controller.abort(reason);
        reject(reason);
      };
      signal?.addEventListener('abort', onPageAbort);
      const finishWith = (fn: () => void) => {
        done = true;
        signal?.removeEventListener('abort', onPageAbort);
        fn();
      };

      const ours = openRequest(req, signal, null);

      let nativePromise: Promise<Credential | null>;
      try {
        nativePromise = B.apply(origGet, creds, [B.assign({}, opts, { signal: controller.signal })]) as Promise<Credential | null>;
      } catch (err) {
        nativePromise = B.Promise.reject(err);
      }
      B.apply(B.promiseThen, nativePromise, [
        (credential: Credential | null) => {
          if (done) return;
          finishWith(() => {
            ours.cancel();
            resolve(credential);
          });
        },
        (error: unknown) => {
          if (done) return;
          if (oursOpen) {
            nativeFailure = { error };
            return;
          }
          finishWith(() => reject(error));
        },
      ]);

      B.apply(B.promiseThen, ours.promise, [
        (outcome: RequestOutcome) => {
          oursOpen = false;
          if (done) return;
          if (outcome.kind === 'credential' && outcome.credential?.type === 'get') {
            finishWith(() => {
              controller.abort(new B.DOMException('Answered by Passbubble.', 'AbortError'));
              resolve(buildCredential(B, outcome.credential));
            });
            return;
          }
          // Ours is out of the race: only the browser's request is left.
          if (nativeFailure) {
            const { error } = nativeFailure;
            finishWith(() => reject(error));
          }
        },
        () => {
          oursOpen = false; // the page aborted; onPageAbort settles
        },
      ]);
    });
  }

  B.defineProperty(creds, 'get', { value: get, writable: true, configurable: true, enumerable: true });
  B.defineProperty(creds, 'create', { value: create, writable: true, configurable: true, enumerable: true });

  // ─── Static PublicKeyCredential methods ───────────────────────────────────

  function waitForStatus(): Promise<boolean | null> {
    if (enabled !== null) return B.Promise.resolve(enabled);
    return new B.Promise<boolean | null>((resolve) => {
      const timer = B.setTimeout(() => resolve(enabled), statusTimeout);
      statusWaiters.push(() => {
        B.clearTimeout(timer);
        resolve(enabled);
      });
    });
  }

  const restoreStatics: Array<() => void> = [];
  function patchStatic(name: string, make: (orig: StaticFn) => StaticFn): void {
    const orig = PKC![name];
    if (typeof orig !== 'function') return;
    const desc = B.getOwnPropertyDescriptor(PKC!, name);
    B.defineProperty(PKC!, name, { value: make(orig as StaticFn), writable: true, configurable: true, enumerable: true });
    restoreStatics.push(() => {
      if (desc) B.defineProperty(PKC!, name, desc);
      else delete PKC![name];
    });
  }
  const alwaysWhenEnabled = (orig: StaticFn): StaticFn =>
    async function (this: unknown) {
      if ((await waitForStatus()) === true) return true;
      return B.apply(orig, PKC, []);
    };
  patchStatic('isUserVerifyingPlatformAuthenticatorAvailable', alwaysWhenEnabled);
  patchStatic('isConditionalMediationAvailable', alwaysWhenEnabled);
  patchStatic('getClientCapabilities', (orig) => async function () {
    const caps = (await B.apply(orig, PKC, [])) as Record<string, unknown>;
    if ((await waitForStatus()) !== true) return caps;
    return B.assign({}, caps, {
      passkeyPlatformAuthenticator: true,
      userVerifyingPlatformAuthenticator: true,
      conditionalGet: true,
    });
  });

  send({ kind: 'hello' });

  return () => {
    B.removeEventListener('message', onMessage as EventListener, true);
    for (const [key, desc] of [
      ['get', ownGet],
      ['create', ownCreate],
    ] as const) {
      if (desc) B.defineProperty(creds, key, desc);
      else delete (creds as unknown as Record<string, unknown>)[key];
    }
    for (const restore of restoreStatics.splice(0)) restore();
  };
}

// Installed by the bundle; tests install their own.
if (import.meta.env.MODE !== 'test' && typeof window !== 'undefined') installPasskeyShim(window);

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

// Page-script (MAIN world) helpers: reading the page's WebAuthn options into
// the JSON wire format, and turning a finished credential back into objects
// that behave like the browser's own PublicKeyCredential. Runs among page
// scripts, so every built-in used later is saved up front (captureBuiltins
// runs at document_start, before any page script). Must not import
// webextension-polyfill or anything else that touches `browser`.

import type {
  SerializedCreationOptions,
  SerializedCredential,
  SerializedDescriptor,
  SerializedExtensions,
  SerializedRequestOptions,
} from '../webauthn/protocol.js';

type PageWindow = Window & typeof globalThis;

export interface PageBuiltins {
  win: PageWindow;
  Uint8Array: Uint8ArrayConstructor;
  ArrayBuffer: ArrayBufferConstructor;
  isView: (value: unknown) => value is ArrayBufferView;
  toStringTag: (value: unknown) => string;
  atob: (s: string) => string;
  btoa: (s: string) => string;
  fromCharCode: (...codes: number[]) => string;
  charCodeAt: (s: string, i: number) => number;
  setPrototypeOf: (o: object, proto: object | null) => object;
  defineProperty: <T>(o: T, key: PropertyKey, desc: PropertyDescriptor) => T;
  getOwnPropertyDescriptor: (o: object, key: PropertyKey) => PropertyDescriptor | undefined;
  keys: (o: object) => string[];
  assign: <T extends object>(target: T, ...sources: object[]) => T;
  isArray: (value: unknown) => value is unknown[];
  apply: <T>(fn: (...args: never[]) => T, thisArg: unknown, args: unknown[]) => T;
  jsonParse: (s: string) => unknown;
  jsonStringify: (v: unknown) => string;
  DOMException: typeof DOMException;
  Promise: PromiseConstructor;
  AbortController: typeof AbortController;
  setTimeout: (fn: () => void, ms: number) => number;
  clearTimeout: (id: number | undefined) => void;
  postMessage: (message: unknown, targetOrigin: string) => void;
  addEventListener: Window['addEventListener'];
  removeEventListener: Window['removeEventListener'];
  getRandomValues: <T extends ArrayBufferView>(array: T) => T;
  /** Promise.prototype.then, called with B.apply. */
  promiseThen: Promise<unknown>['then'];
}

export function captureBuiltins(win: PageWindow): PageBuiltins {
  const objectToString = win.Object.prototype.toString;
  const stringCharCodeAt = win.String.prototype.charCodeAt;
  const reflectApply = win.Reflect.apply;
  const crypto = win.crypto;
  const getRandomValues = crypto.getRandomValues;
  return {
    win,
    Uint8Array: win.Uint8Array,
    ArrayBuffer: win.ArrayBuffer,
    isView: win.ArrayBuffer.isView as PageBuiltins['isView'],
    toStringTag: (value) => reflectApply(objectToString, value, []) as string,
    atob: win.atob.bind(win),
    btoa: win.btoa.bind(win),
    fromCharCode: win.String.fromCharCode,
    charCodeAt: (s, i) => reflectApply(stringCharCodeAt, s, [i]) as number,
    setPrototypeOf: win.Object.setPrototypeOf,
    defineProperty: win.Object.defineProperty as PageBuiltins['defineProperty'],
    getOwnPropertyDescriptor: win.Object.getOwnPropertyDescriptor,
    keys: win.Object.keys,
    assign: win.Object.assign,
    isArray: win.Array.isArray as PageBuiltins['isArray'],
    apply: reflectApply as PageBuiltins['apply'],
    jsonParse: win.JSON.parse,
    jsonStringify: win.JSON.stringify,
    DOMException: win.DOMException,
    Promise: win.Promise,
    AbortController: win.AbortController,
    setTimeout: win.setTimeout.bind(win) as PageBuiltins['setTimeout'],
    clearTimeout: win.clearTimeout.bind(win) as PageBuiltins['clearTimeout'],
    postMessage: win.postMessage.bind(win),
    addEventListener: win.addEventListener.bind(win),
    removeEventListener: win.removeEventListener.bind(win),
    getRandomValues: (array) => reflectApply(getRandomValues, crypto, [array]) as typeof array,
    promiseThen: win.Promise.prototype.then,
  };
}

// ─── base64url ──────────────────────────────────────────────────────────────

export function toBase64url(B: PageBuiltins, bytes: Uint8Array): string {
  let binary = '';
  for (let i = 0; i < bytes.length; i++) binary += B.fromCharCode(bytes[i]);
  let out = B.btoa(binary);
  out = out.split('+').join('-').split('/').join('_');
  while (out.endsWith('=')) out = out.slice(0, -1);
  return out;
}

export function fromBase64url(B: PageBuiltins, value: string): ArrayBuffer {
  const std = value.split('-').join('+').split('_').join('/');
  const binary = B.atob(std + '='.repeat((4 - (std.length % 4)) % 4));
  const bytes = new B.Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = B.charCodeAt(binary, i);
  return bytes.buffer as ArrayBuffer;
}

// ─── Reading the page's options ─────────────────────────────────────────────

/** The bytes of a BufferSource (copied), or a TypeError. */
function bufferBytes(B: PageBuiltins, value: unknown, what: string): Uint8Array {
  if (B.isView(value)) {
    const view = value as ArrayBufferView;
    return new B.Uint8Array(view.buffer, view.byteOffset, view.byteLength).slice();
  }
  if (B.toStringTag(value) === '[object ArrayBuffer]') return new B.Uint8Array(value as ArrayBuffer).slice();
  throw new TypeError(`${what} is not a BufferSource`);
}

function buffer64(B: PageBuiltins, value: unknown, what: string): string {
  return toBase64url(B, bufferBytes(B, value, what));
}

function optionalString(value: unknown): string | undefined {
  return value === undefined || value === null ? undefined : String(value);
}

function list(B: PageBuiltins, value: unknown): unknown[] {
  if (value === undefined || value === null) return [];
  if (!B.isArray(value)) throw new TypeError('expected a sequence');
  return value;
}

function descriptors(B: PageBuiltins, value: unknown): SerializedDescriptor[] {
  return list(B, value).map((item) => {
    const d = item as { type?: unknown; id?: unknown; transports?: unknown };
    const transports = d.transports === undefined ? undefined : list(B, d.transports).map(String);
    return { type: String(d.type ?? 'public-key'), id: buffer64(B, d.id, 'credential id'), ...(transports ? { transports } : {}) };
  });
}

function hints(B: PageBuiltins, value: unknown): string[] {
  return list(B, value).map(String).slice(0, 16);
}

function extensionInputs(B: PageBuiltins, value: unknown): SerializedExtensions {
  if (value === undefined || value === null) return { names: [] };
  if (typeof value !== 'object') throw new TypeError('extensions must be an object');
  const ext = value as { credProps?: unknown; largeBlob?: { support?: unknown } | null; appid?: unknown };
  const names = B.keys(ext).slice(0, 32).map((n) => n.slice(0, 64));
  const largeBlobSupport = ext.largeBlob && typeof ext.largeBlob === 'object' ? ext.largeBlob.support : undefined;
  return {
    names,
    ...(ext.credProps === true ? { credProps: true } : {}),
    ...(typeof largeBlobSupport === 'string' ? { largeBlobSupport } : {}),
    ...(typeof ext.appid === 'string' ? { appid: ext.appid } : {}),
  };
}

function timeoutOf(value: unknown): { timeout?: number } {
  return typeof value === 'number' && Number.isFinite(value) ? { timeout: value } : {};
}

/** PublicKeyCredentialCreationOptions → wire format. Throws a TypeError for
 * what the browser would reject; the caller then hands the call to it. */
export function serializeCreationOptions(B: PageBuiltins, pk: unknown): SerializedCreationOptions {
  if (typeof pk !== 'object' || pk === null) throw new TypeError('publicKey must be an object');
  const o = pk as Record<string, unknown>;
  const rp = o.rp as { id?: unknown; name?: unknown } | undefined;
  const user = o.user as { id?: unknown; name?: unknown; displayName?: unknown } | undefined;
  if (!rp || typeof rp !== 'object' || !user || typeof user !== 'object') throw new TypeError('rp and user are required');
  const sel = o.authenticatorSelection as Record<string, unknown> | undefined | null;
  const rpId = optionalString(rp.id);
  return {
    rp: { ...(rpId !== undefined ? { id: rpId } : {}), name: String(rp.name ?? '') },
    user: { id: buffer64(B, user.id, 'user.id'), name: String(user.name ?? ''), displayName: String(user.displayName ?? '') },
    challenge: buffer64(B, o.challenge, 'challenge'),
    pubKeyCredParams: list(B, o.pubKeyCredParams).map((p) => {
      const param = p as { type?: unknown; alg?: unknown };
      return { type: String(param.type), alg: Number(param.alg) };
    }),
    ...timeoutOf(o.timeout),
    excludeCredentials: descriptors(B, o.excludeCredentials),
    ...(sel && typeof sel === 'object'
      ? {
          authenticatorSelection: {
            ...(sel.authenticatorAttachment !== undefined ? { authenticatorAttachment: String(sel.authenticatorAttachment) } : {}),
            ...(sel.residentKey !== undefined ? { residentKey: String(sel.residentKey) } : {}),
            ...(sel.requireResidentKey !== undefined ? { requireResidentKey: !!sel.requireResidentKey } : {}),
            ...(sel.userVerification !== undefined ? { userVerification: String(sel.userVerification) } : {}),
          },
        }
      : {}),
    hints: hints(B, o.hints),
    ...(o.attestation !== undefined ? { attestation: String(o.attestation) } : {}),
    extensions: extensionInputs(B, o.extensions),
  };
}

/** PublicKeyCredentialRequestOptions → wire format (see above). */
export function serializeRequestOptions(B: PageBuiltins, pk: unknown): SerializedRequestOptions {
  if (typeof pk !== 'object' || pk === null) throw new TypeError('publicKey must be an object');
  const o = pk as Record<string, unknown>;
  const rpId = optionalString(o.rpId);
  return {
    challenge: buffer64(B, o.challenge, 'challenge'),
    ...timeoutOf(o.timeout),
    ...(rpId !== undefined ? { rpId } : {}),
    allowCredentials: descriptors(B, o.allowCredentials),
    ...(o.userVerification !== undefined ? { userVerification: String(o.userVerification) } : {}),
    hints: hints(B, o.hints),
    extensions: extensionInputs(B, o.extensions),
  };
}

// ─── Building the result ────────────────────────────────────────────────────

function own<T extends object>(B: PageBuiltins, target: T, key: string, value: unknown, enumerable = true): void {
  B.defineProperty(target, key, { value, enumerable, configurable: true, writable: false });
}

function copyJson<T>(B: PageBuiltins, value: T): T {
  return B.jsonParse(B.jsonStringify(value)) as T;
}

/**
 * A PublicKeyCredential for the page: a plain object whose prototype is the
 * page's PublicKeyCredential.prototype (so `instanceof` holds) carrying every
 * attribute and method as an own property, since the prototype's native
 * getters only work on real credentials. toJSON() returns what Chrome's does.
 */
export function buildCredential(B: PageBuiltins, cred: SerializedCredential): PublicKeyCredential {
  const win = B.win as PageWindow & Record<string, { prototype: object } | undefined>;
  const rawId = fromBase64url(B, cred.id);
  const clientDataJSON = fromBase64url(B, cred.clientDataJSON);
  const authenticatorData = fromBase64url(B, cred.authenticatorData);
  const transports = cred.transports.slice();
  const extensionResults = copyJson(B, cred.clientExtensionResults ?? {});

  const response = {};
  let responseJSON: Record<string, unknown>;
  if (cred.type === 'create') {
    const attestationObject = fromBase64url(B, cred.attestationObject ?? '');
    const publicKey = cred.publicKey ? fromBase64url(B, cred.publicKey) : null;
    const algorithm = cred.publicKeyAlgorithm ?? -7;
    B.setPrototypeOf(
      response,
      (win.AuthenticatorAttestationResponse ?? win.AuthenticatorResponse)?.prototype ?? B.win.Object.prototype,
    );
    own(B, response, 'clientDataJSON', clientDataJSON);
    own(B, response, 'attestationObject', attestationObject);
    own(B, response, 'getTransports', () => transports.slice(), false);
    own(B, response, 'getAuthenticatorData', () => authenticatorData.slice(0), false);
    own(B, response, 'getPublicKey', () => (publicKey ? publicKey.slice(0) : null), false);
    own(B, response, 'getPublicKeyAlgorithm', () => algorithm, false);
    // WebIDL dictionaries come out with their keys in lexicographic order.
    responseJSON = {
      attestationObject: cred.attestationObject,
      authenticatorData: cred.authenticatorData,
      clientDataJSON: cred.clientDataJSON,
      ...(cred.publicKey ? { publicKey: cred.publicKey } : {}),
      publicKeyAlgorithm: algorithm,
      transports: transports.slice(),
    };
  } else {
    const signature = fromBase64url(B, cred.signature ?? '');
    const userHandle = cred.userHandle ? fromBase64url(B, cred.userHandle) : null;
    B.setPrototypeOf(
      response,
      (win.AuthenticatorAssertionResponse ?? win.AuthenticatorResponse)?.prototype ?? B.win.Object.prototype,
    );
    own(B, response, 'clientDataJSON', clientDataJSON);
    own(B, response, 'authenticatorData', authenticatorData);
    own(B, response, 'signature', signature);
    own(B, response, 'userHandle', userHandle);
    responseJSON = {
      authenticatorData: cred.authenticatorData,
      clientDataJSON: cred.clientDataJSON,
      signature: cred.signature,
      ...(cred.userHandle ? { userHandle: cred.userHandle } : {}),
    };
  }

  const credential = {};
  B.setPrototypeOf(credential, win.PublicKeyCredential?.prototype ?? B.win.Object.prototype);
  own(B, credential, 'id', cred.id);
  own(B, credential, 'rawId', rawId);
  own(B, credential, 'type', 'public-key');
  own(B, credential, 'authenticatorAttachment', 'platform');
  own(B, credential, 'response', response);
  own(B, credential, 'getClientExtensionResults', () => copyJson(B, extensionResults), false);
  own(
    B,
    credential,
    'toJSON',
    () => ({
      authenticatorAttachment: 'platform',
      clientExtensionResults: copyJson(B, extensionResults),
      id: cred.id,
      rawId: cred.id,
      response: copyJson(B, responseJSON),
      type: 'public-key',
    }),
    false,
  );
  return credential as PublicKeyCredential;
}

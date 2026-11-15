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

// The wire format between the page script (MAIN world), the bridge (isolated
// content script) and the background, plus the rules every side applies
// before Passbubble answers a WebAuthn request. Pure code: no browser APIs, so
// the page script can bundle it. The page script evaluates this module before
// any page script runs, so the JSON functions captured here are the originals.

const jsonParse = JSON.parse;
const jsonStringify = JSON.stringify;

/** Marker on every window message of this protocol. */
export const CHANNEL = 'passbubble:webauthn:1';

/** Window messages longer than this are dropped unread. */
export const MAX_MESSAGE_CHARS = 64 * 1024;

/** The bridge must acknowledge a request within this time, or the page
 * script hands it to the browser (e.g. an orphaned page script after an
 * extension update, whose bridge is gone). */
export const ACK_TIMEOUT_MS = 500;

/** How long the page script waits for the bridge's feature state before the
 * static PublicKeyCredential methods answer like the browser. */
export const STATUS_TIMEOUT_MS = 600;

export const TIMEOUT_DEFAULT_MS = 5 * 60_000;
export const TIMEOUT_MIN_MS = 30_000;
export const TIMEOUT_MAX_MS = 10 * 60_000;

/** A request's publicKey.timeout, clamped to 30 s–10 min (default 5 min). */
export function clampTimeout(timeout: unknown): number {
  if (typeof timeout !== 'number' || !Number.isFinite(timeout) || timeout <= 0) return TIMEOUT_DEFAULT_MS;
  return Math.min(TIMEOUT_MAX_MS, Math.max(TIMEOUT_MIN_MS, timeout));
}

// ─── Serialized WebAuthn options and results ────────────────────────────────

export interface SerializedDescriptor {
  type: string;
  /** base64url */
  id: string;
  transports?: string[];
}

/** Only what Passbubble looks at. Extension inputs are reduced to their names
 * and the few values that decide anything. */
export interface SerializedExtensions {
  names: string[];
  credProps?: boolean;
  /** largeBlob.support on create ('required' | 'preferred'). */
  largeBlobSupport?: string;
  appid?: string;
}

export interface SerializedCreationOptions {
  rp: { id?: string; name: string };
  user: { id: string; name: string; displayName: string };
  challenge: string;
  pubKeyCredParams: Array<{ type: string; alg: number }>;
  timeout?: number;
  excludeCredentials: SerializedDescriptor[];
  authenticatorSelection?: {
    authenticatorAttachment?: string;
    residentKey?: string;
    requireResidentKey?: boolean;
    userVerification?: string;
  };
  hints: string[];
  attestation?: string;
  extensions: SerializedExtensions;
}

export interface SerializedRequestOptions {
  challenge: string;
  timeout?: number;
  rpId?: string;
  allowCredentials: SerializedDescriptor[];
  userVerification?: string;
  hints: string[];
  extensions: SerializedExtensions;
}

export type SerializedRequest =
  | { type: 'create'; mediation?: string; options: SerializedCreationOptions }
  | { type: 'get'; mediation?: string; options: SerializedRequestOptions };

/** A finished credential, every buffer as base64url. */
export interface SerializedCredential {
  type: 'create' | 'get';
  id: string;
  clientDataJSON: string;
  authenticatorData: string;
  /** create */
  attestationObject?: string;
  /** create: SubjectPublicKeyInfo DER */
  publicKey?: string;
  publicKeyAlgorithm?: number;
  /** get */
  signature?: string;
  /** get */
  userHandle?: string;
  transports: string[];
  clientExtensionResults: Record<string, unknown>;
}

/** What the background finally decides for a request. */
export type RequestOutcome =
  | { kind: 'credential'; credential: SerializedCredential }
  | { kind: 'error'; name: string; message: string }
  | { kind: 'native' };

// ─── Window messages between page script and bridge ─────────────────────────

export type PageMessage =
  | { kind: 'hello' }
  | { kind: 'request'; id: string; request: SerializedRequest }
  | { kind: 'cancel'; id: string };

export type BridgeMessage =
  /** The bridge listens; `enabled` is null until the background answered. */
  | { kind: 'ready'; enabled: boolean | null }
  | { kind: 'status'; enabled: boolean }
  | { kind: 'ack'; id: string }
  | { kind: 'native'; id: string }
  | { kind: 'error'; id: string; name: string; message: string }
  | { kind: 'credential'; id: string; credential: SerializedCredential };

type Direction = 'page' | 'bridge';

export function encodeMessage(dir: 'page', msg: PageMessage): string;
export function encodeMessage(dir: 'bridge', msg: BridgeMessage): string;
export function encodeMessage(dir: Direction, msg: PageMessage | BridgeMessage): string {
  return jsonStringify({ channel: CHANNEL, dir, msg });
}

/** The message inside a window message of this protocol sent in direction
 * `dir`, or null for anything else (other scripts' messages, oversized data,
 * the other direction). Only the envelope is checked here. */
export function decodeMessage(data: unknown, dir: 'page'): PageMessage | null;
export function decodeMessage(data: unknown, dir: 'bridge'): BridgeMessage | null;
export function decodeMessage(data: unknown, dir: Direction): PageMessage | BridgeMessage | null {
  if (typeof data !== 'string' || data.length > MAX_MESSAGE_CHARS) return null;
  let parsed: unknown;
  try {
    parsed = jsonParse(data);
  } catch {
    return null;
  }
  if (typeof parsed !== 'object' || parsed === null) return null;
  const env = parsed as { channel?: unknown; dir?: unknown; msg?: unknown };
  if (env.channel !== CHANNEL || env.dir !== dir) return null;
  const msg = env.msg as { kind?: unknown } | null;
  if (typeof msg !== 'object' || msg === null || typeof msg.kind !== 'string') return null;
  return msg as PageMessage | BridgeMessage;
}

// ─── Rules ──────────────────────────────────────────────────────────────────

export const LIMITS = {
  challengeBytes: 1024,
  credentialIdBytes: 1023,
  descriptors: 64,
  stringChars: 256,
  userIdBytes: 64,
} as const;

/** Transports that can only mean a hardware key; a sign-in that allows only
 * such credentials goes to the browser even while the vault is locked. */
export const HARDWARE_TRANSPORTS = new Set(['usb', 'nfc', 'ble', 'smart-card']);

const ES256 = -7;

/** Decoded length of a base64url string, or -1 when it is not one. */
export function base64urlLength(value: unknown): number {
  if (typeof value !== 'string' || !/^[A-Za-z0-9_-]*$/.test(value) || value.length % 4 === 1) return -1;
  return Math.floor((value.length * 3) / 4);
}

/**
 * Why a request has to go to the browser unchanged, or null when Passbubble
 * may answer it. Applied by the page script before anything leaves the page
 * and again by the background, which also checks the origin and the rp id.
 */
export function passthroughReason(request: SerializedRequest): string | null {
  const mediation = request.mediation;
  if (mediation === 'silent' || mediation === 'immediate') return `mediation ${mediation}`;
  if (request.type === 'create' && mediation === 'conditional') return 'conditional create';
  const opts = request.options;
  const firstHint = opts.hints[0];
  if (firstHint === 'security-key' || firstHint === 'hybrid') return `hint ${firstHint}`;
  if (opts.extensions.names.includes('payment')) return 'payment extension';
  if (request.type === 'create') {
    const o = request.options;
    if (o.authenticatorSelection?.authenticatorAttachment === 'cross-platform') return 'cross-platform attachment';
    if (o.attestation === 'enterprise') return 'enterprise attestation';
    if (o.extensions.largeBlobSupport === 'required') return 'largeBlob required';
    // An empty list means the defaults, which include ES256.
    if (o.pubKeyCredParams.length > 0 && !o.pubKeyCredParams.some((p) => p.type === 'public-key' && p.alg === ES256)) {
      return 'no ES256';
    }
    const userIdLength = base64urlLength(o.user.id);
    if (userIdLength <= 0 || userIdLength > LIMITS.userIdBytes) return 'bad user id';
  }
  return null;
}

function tooLong(s: unknown): boolean {
  return typeof s === 'string' && s.length > LIMITS.stringChars;
}

function descriptorsValid(list: SerializedDescriptor[]): boolean {
  if (!Array.isArray(list) || list.length > LIMITS.descriptors) return false;
  return list.every((d) => {
    const n = base64urlLength(d?.id);
    return n > 0 && n <= LIMITS.credentialIdBytes && (d.transports === undefined || Array.isArray(d.transports));
  });
}

/** Structural and size checks on a request coming from a page (through the
 * bridge). False means: not ours to answer, let the browser deal with it. */
export function requestWithinLimits(request: SerializedRequest): boolean {
  if (typeof request !== 'object' || request === null) return false;
  if (request.type !== 'create' && request.type !== 'get') return false;
  const opts = request.options as Partial<SerializedCreationOptions & SerializedRequestOptions> | undefined;
  if (typeof opts !== 'object' || opts === null) return false;
  if (!Array.isArray(opts.hints) || typeof opts.extensions !== 'object' || !Array.isArray(opts.extensions?.names)) {
    return false;
  }
  const challenge = base64urlLength(opts.challenge);
  if (challenge <= 0 || challenge > LIMITS.challengeBytes) return false;
  if (request.type === 'create') {
    const o = request.options;
    if (typeof o.rp !== 'object' || typeof o.user !== 'object' || !Array.isArray(o.pubKeyCredParams)) return false;
    if (typeof o.rp.name !== 'string' || typeof o.user.name !== 'string' || typeof o.user.displayName !== 'string') {
      return false;
    }
    if ([o.rp.id, o.rp.name, o.user.name, o.user.displayName].some(tooLong)) return false;
    if (o.pubKeyCredParams.length > LIMITS.descriptors) return false;
    return descriptorsValid(o.excludeCredentials);
  }
  const o = request.options;
  if (tooLong(o.rpId)) return false;
  return descriptorsValid(o.allowCredentials);
}

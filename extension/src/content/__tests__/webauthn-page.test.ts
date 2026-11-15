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

// The MAIN-world page script, against a stand-in for the browser's WebAuthn
// objects and a fake bridge on the window message channel.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { installPasskeyShim } from '../webauthn-page.js';
import {
  decodeMessage,
  encodeMessage,
  type BridgeMessage,
  type PageMessage,
  type SerializedCredential,
} from '../../webauthn/protocol.js';

// ─── Stand-ins for the browser ──────────────────────────────────────────────

class FakeCredential {
  get id(): string {
    throw new TypeError('Illegal invocation');
  }
}
class FakePublicKeyCredential extends FakeCredential {
  static isUserVerifyingPlatformAuthenticatorAvailable = vi.fn(async () => false);
  static isConditionalMediationAvailable = vi.fn(async () => false);
  static getClientCapabilities = vi.fn(async () => ({ hybridTransport: true, conditionalGet: false }));
}
const originalUvpaa = FakePublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable;
class FakeAuthenticatorResponse {}
class FakeAttestationResponse extends FakeAuthenticatorResponse {}
class FakeAssertionResponse extends FakeAuthenticatorResponse {}

const nativeGet = vi.fn();
const nativeCreate = vi.fn();
class FakeCredentialsContainer {
  get(this: unknown, options: unknown) {
    return nativeGet(this, options);
  }
  create(this: unknown, options: unknown) {
    return nativeCreate(this, options);
  }
}

const g = window as unknown as Record<string, unknown>;
Object.assign(g, {
  PublicKeyCredential: FakePublicKeyCredential,
  AuthenticatorResponse: FakeAuthenticatorResponse,
  AuthenticatorAttestationResponse: FakeAttestationResponse,
  AuthenticatorAssertionResponse: FakeAssertionResponse,
});

// jsdom's postMessage sets no event.source; a real top window's does.
window.postMessage = ((data: unknown) => {
  setTimeout(() => window.dispatchEvent(new MessageEvent('message', { data, source: window })), 0);
}) as typeof window.postMessage;

// ─── A fake bridge ──────────────────────────────────────────────────────────

interface FakeBridge {
  received: PageMessage[];
  requests: () => Array<Extract<PageMessage, { kind: 'request' }>>;
  cancels: () => string[];
  reply: (msg: BridgeMessage) => void;
  stop: () => void;
}

function fakeBridge({ ready = true, enabled = true, autoAck = true }: { ready?: boolean; enabled?: boolean | null; autoAck?: boolean } = {}): FakeBridge {
  const received: PageMessage[] = [];
  const reply = (msg: BridgeMessage) =>
    window.dispatchEvent(new MessageEvent('message', { data: encodeMessage('bridge', msg), source: window }));
  const onMessage = (e: MessageEvent) => {
    const msg = decodeMessage(e.data, 'page');
    if (!msg) return;
    received.push(msg);
    if (msg.kind === 'hello' && ready) reply({ kind: 'ready', enabled });
    if (msg.kind === 'request' && autoAck) reply({ kind: 'ack', id: msg.id });
  };
  window.addEventListener('message', onMessage);
  if (ready) reply({ kind: 'ready', enabled });
  return {
    received,
    requests: () => received.filter((m): m is Extract<PageMessage, { kind: 'request' }> => m.kind === 'request'),
    cancels: () => received.filter((m) => m.kind === 'cancel').map((m) => (m as { id: string }).id),
    reply,
    stop: () => window.removeEventListener('message', onMessage),
  };
}

const tick = (ms = 0) => new Promise((r) => setTimeout(r, ms));

const bytes = (...b: number[]) => new Uint8Array(b);

function creationOptions(over: Record<string, unknown> = {}): CredentialCreationOptions {
  return {
    publicKey: {
      rp: { id: 'example.com', name: 'Example' },
      user: { id: bytes(1, 2, 3), name: 'alice', displayName: 'Alice' },
      challenge: bytes(9, 9, 9, 9),
      pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
      ...over,
    } as PublicKeyCredentialCreationOptions,
  };
}

function requestOptions(over: Record<string, unknown> = {}, extra: Record<string, unknown> = {}): CredentialRequestOptions {
  return { publicKey: { challenge: bytes(7, 7, 7, 7), rpId: 'example.com', ...over } as PublicKeyCredentialRequestOptions, ...extra };
}

const ATTESTATION: SerializedCredential = {
  type: 'create',
  id: 'AQIDBA',
  clientDataJSON: 'eyJ9',
  authenticatorData: 'BQYH',
  attestationObject: 'CAkK',
  publicKey: 'MFkw',
  publicKeyAlgorithm: -7,
  transports: ['internal', 'hybrid'],
  clientExtensionResults: { credProps: { rk: true } },
};

const ASSERTION: SerializedCredential = {
  type: 'get',
  id: 'AQIDBA',
  clientDataJSON: 'eyJ9',
  authenticatorData: 'BQYH',
  signature: 'MEUC',
  userHandle: 'dXNlcg',
  transports: ['internal', 'hybrid'],
  clientExtensionResults: {},
};

const buf = (b: ArrayBuffer) => [...new Uint8Array(b)];

let container: FakeCredentialsContainer;
let uninstall: (() => void) | null = null;
let bridge: FakeBridge | null = null;

function install(): void {
  uninstall = installPasskeyShim(window as Window & typeof globalThis, { ackTimeoutMs: 40, statusTimeoutMs: 40 });
  expect(uninstall).toBeTypeOf('function');
}

beforeEach(() => {
  nativeGet.mockReset().mockResolvedValue('native-get');
  nativeCreate.mockReset().mockResolvedValue('native-create');
  originalUvpaa.mockClear();
  container = new FakeCredentialsContainer();
  Object.defineProperty(navigator, 'credentials', { value: container, configurable: true });
});

afterEach(() => {
  uninstall?.();
  uninstall = null;
  bridge?.stop();
  bridge = null;
});

// ─── Tests ──────────────────────────────────────────────────────────────────

describe('installing', () => {
  it('patches the container instance and puts it back on uninstall', () => {
    install();
    expect(Object.getOwnPropertyDescriptor(container, 'get')).toMatchObject({ writable: true, configurable: true });
    uninstall!();
    uninstall = null;
    expect(Object.getOwnPropertyDescriptor(container, 'get')).toBeUndefined();
    expect(container.get).toBe(FakeCredentialsContainer.prototype.get);
  });

  it('does nothing without WebAuthn', () => {
    Object.defineProperty(navigator, 'credentials', { value: undefined, configurable: true });
    expect(installPasskeyShim(window as Window & typeof globalThis)).toBeNull();
  });

  it("keeps another manager's wrapper in the chain and calls it with the page's own options", async () => {
    const other = vi.fn(function (this: unknown, options: unknown) {
      return FakeCredentialsContainer.prototype.get.call(this, options);
    });
    Object.defineProperty(container, 'get', { value: other, writable: true, configurable: true });
    install();
    bridge = fakeBridge();
    const options = requestOptions({ hints: ['security-key'] });
    await expect(navigator.credentials.get(options)).resolves.toBe('native-get');
    expect(other).toHaveBeenCalledWith(options);
    expect(other.mock.contexts[0]).toBe(container);
    expect(nativeGet).toHaveBeenCalledWith(container, options);
  });
});

describe('straight to the browser', () => {
  beforeEach(async () => {
    install();
    bridge = fakeBridge();
    await tick();
  });

  const createCases: Array<[string, CredentialCreationOptions]> = [
    ['no publicKey', { password: true } as unknown as CredentialCreationOptions],
    ['conditional create', { ...creationOptions(), mediation: 'conditional' } as CredentialCreationOptions],
    ['security-key hint', creationOptions({ hints: ['security-key'] })],
    ['hybrid hint', creationOptions({ hints: ['hybrid', 'client-device'] })],
    ['cross-platform', creationOptions({ authenticatorSelection: { authenticatorAttachment: 'cross-platform' } })],
    ['enterprise attestation', creationOptions({ attestation: 'enterprise' })],
    ['largeBlob required', creationOptions({ extensions: { largeBlob: { support: 'required' } } })],
    ['payment', creationOptions({ extensions: { payment: { isPayment: true } } })],
    ['no ES256', creationOptions({ pubKeyCredParams: [{ type: 'public-key', alg: -257 }] })],
    ['empty user id', creationOptions({ user: { id: new Uint8Array(0), name: 'a', displayName: 'a' } })],
    ['user id over 64 bytes', creationOptions({ user: { id: new Uint8Array(65), name: 'a', displayName: 'a' } })],
    ['unreadable options', creationOptions({ challenge: 'not a buffer' })],
  ];

  it.each(createCases)('create: %s', async (_name, options) => {
    await expect(navigator.credentials.create(options)).resolves.toBe('native-create');
    expect(nativeCreate).toHaveBeenCalledWith(container, options);
    expect(bridge!.requests()).toEqual([]);
  });

  const getCases: Array<[string, CredentialRequestOptions]> = [
    ['no publicKey', { password: true } as unknown as CredentialRequestOptions],
    ['silent', requestOptions({}, { mediation: 'silent' })],
    ['immediate', requestOptions({}, { mediation: 'immediate' })],
    ['security-key hint', requestOptions({ hints: ['security-key'] })],
    ['payment', requestOptions({ extensions: { payment: { isPayment: true } } })],
  ];

  it.each(getCases)('get: %s', async (_name, options) => {
    await expect(navigator.credentials.get(options)).resolves.toBe('native-get');
    expect(nativeGet).toHaveBeenCalledWith(container, options);
    expect(bridge!.requests()).toEqual([]);
  });

  it('an empty pubKeyCredParams list means the defaults, which include ES256', async () => {
    const p = navigator.credentials.create(creationOptions({ pubKeyCredParams: [] }));
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    bridge!.reply({ kind: 'native', id: bridge!.requests()[0].id });
    await expect(p).resolves.toBe('native-create');
  });
});

describe('talking to the bridge', () => {
  it('serializes buffers with their offsets', async () => {
    install();
    bridge = fakeBridge();
    const backing = new Uint8Array([0, 0, 0xfb, 0xff, 0, 0]);
    const p = navigator.credentials.create(
      creationOptions({
        challenge: new DataView(backing.buffer, 2, 2),
        user: { id: backing.subarray(2, 4), name: 'alice', displayName: 'Alice' },
        excludeCredentials: [{ type: 'public-key', id: backing.buffer, transports: ['usb'] }],
        extensions: { credProps: true, prf: { eval: { first: new Uint8Array(32) } } },
      }),
    );
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    const { request } = bridge!.requests()[0];
    expect(request.type).toBe('create');
    if (request.type !== 'create') return;
    expect(request.options.challenge).toBe('-_8');
    expect(request.options.user.id).toBe('-_8');
    expect(request.options.excludeCredentials).toEqual([{ type: 'public-key', id: 'AAD7_wAA', transports: ['usb'] }]);
    expect(request.options.extensions).toEqual({ names: ['credProps', 'prf'], credProps: true });
    bridge!.reply({ kind: 'error', id: bridge!.requests()[0].id, name: 'NotAllowedError', message: 'no' });
    await expect(p).rejects.toMatchObject({ name: 'NotAllowedError' });
  });

  it('queues requests until the bridge is ready', async () => {
    install();
    bridge = fakeBridge({ ready: false });
    await tick();
    const p = navigator.credentials.get(requestOptions());
    await tick();
    expect(bridge.requests()).toEqual([]);
    bridge.reply({ kind: 'ready', enabled: true });
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    bridge.reply({ kind: 'native', id: bridge.requests()[0].id });
    await expect(p).resolves.toBe('native-get');
  });

  it('falls back to the browser when no bridge acknowledges in time', async () => {
    install();
    bridge = fakeBridge({ autoAck: false });
    const options = requestOptions();
    await expect(navigator.credentials.get(options)).resolves.toBe('native-get');
    expect(nativeGet).toHaveBeenCalledWith(container, options);
    await vi.waitFor(() => expect(bridge!.cancels()).toHaveLength(1));
  });

  it('goes to the browser right away when the bridge says the feature is off', async () => {
    install();
    bridge = fakeBridge({ enabled: false });
    await tick();
    await expect(navigator.credentials.create(creationOptions())).resolves.toBe('native-create');
    expect(bridge.requests()).toEqual([]);
  });

  it('hands over to the browser with the original options on "native"', async () => {
    install();
    bridge = fakeBridge();
    const controller = new AbortController();
    const options = { ...creationOptions(), signal: controller.signal };
    const p = navigator.credentials.create(options);
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    bridge.reply({ kind: 'native', id: bridge.requests()[0].id });
    await expect(p).resolves.toBe('native-create');
    expect(nativeCreate.mock.calls[0][1]).toBe(options);
  });

  it('turns errors into DOMExceptions', async () => {
    install();
    bridge = fakeBridge();
    const p = navigator.credentials.create(creationOptions());
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    bridge.reply({ kind: 'error', id: bridge.requests()[0].id, name: 'InvalidStateError', message: 'already registered' });
    const err = await p.catch((e: unknown) => e);
    expect(err).toBeInstanceOf(DOMException);
    expect(err).toMatchObject({ name: 'InvalidStateError', message: 'already registered' });
  });

  it('ignores answers for unknown ids and from other windows', async () => {
    install();
    bridge = fakeBridge();
    const p = navigator.credentials.create(creationOptions());
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    const id = bridge.requests()[0].id;
    bridge.reply({ kind: 'native', id: 'other' });
    const frame = document.createElement('iframe');
    document.body.appendChild(frame);
    window.dispatchEvent(
      new MessageEvent('message', { data: encodeMessage('bridge', { kind: 'native', id }), source: frame.contentWindow }),
    );
    await tick(10);
    expect(nativeCreate).not.toHaveBeenCalled();
    bridge.reply({ kind: 'native', id });
    await expect(p).resolves.toBe('native-create');
  });
});

describe('abort', () => {
  beforeEach(() => {
    install();
    bridge = fakeBridge();
  });

  it('rejects at once with the reason of an already aborted signal', async () => {
    const controller = new AbortController();
    const reason = new Error('stop');
    controller.abort(reason);
    await expect(navigator.credentials.create({ ...creationOptions(), signal: controller.signal })).rejects.toBe(reason);
    expect(bridge!.requests()).toEqual([]);
    expect(nativeCreate).not.toHaveBeenCalled();
  });

  it('rejects with the reason and withdraws the request', async () => {
    const controller = new AbortController();
    const p = navigator.credentials.get({ ...requestOptions(), signal: controller.signal });
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    controller.abort();
    const err = await p.catch((e: unknown) => e);
    expect(err).toMatchObject({ name: 'AbortError' });
    await vi.waitFor(() => expect(bridge!.cancels()).toEqual([bridge!.requests()[0].id]));
    expect(nativeGet).not.toHaveBeenCalled();
  });
});

describe('the credential', () => {
  beforeEach(() => {
    install();
    bridge = fakeBridge();
  });

  async function answer<T>(p: Promise<T>, credential: SerializedCredential): Promise<T> {
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    bridge!.reply({ kind: 'credential', id: bridge!.requests()[0].id, credential });
    return p;
  }

  it('looks like a PublicKeyCredential from create()', async () => {
    const cred = (await answer(navigator.credentials.create(creationOptions()), ATTESTATION)) as PublicKeyCredential;
    expect(cred).toBeInstanceOf(FakePublicKeyCredential);
    expect(cred).toBeInstanceOf(FakeCredential);
    expect(cred.id).toBe('AQIDBA');
    expect(cred.type).toBe('public-key');
    expect(cred.authenticatorAttachment).toBe('platform');
    expect(cred.rawId).toBeInstanceOf(ArrayBuffer);
    expect(buf(cred.rawId)).toEqual([1, 2, 3, 4]);
    const response = cred.response as AuthenticatorAttestationResponse;
    expect(response).toBeInstanceOf(FakeAttestationResponse);
    expect(buf(response.clientDataJSON)).toEqual([0x7b, 0x22, 0x7d]);
    expect(buf(response.attestationObject)).toEqual([8, 9, 10]);
    expect(response.getTransports()).toEqual(['internal', 'hybrid']);
    expect(buf(response.getAuthenticatorData())).toEqual([5, 6, 7]);
    expect(buf(response.getPublicKey()!)).toEqual([0x30, 0x59, 0x30]);
    expect(response.getPublicKeyAlgorithm()).toBe(-7);
    expect(cred.getClientExtensionResults()).toEqual({ credProps: { rk: true } });
    const json = cred.toJSON() as Record<string, unknown>;
    expect(Object.keys(json)).toEqual(['authenticatorAttachment', 'clientExtensionResults', 'id', 'rawId', 'response', 'type']);
    expect(json).toEqual({
      authenticatorAttachment: 'platform',
      clientExtensionResults: { credProps: { rk: true } },
      id: 'AQIDBA',
      rawId: 'AQIDBA',
      response: {
        attestationObject: 'CAkK',
        authenticatorData: 'BQYH',
        clientDataJSON: 'eyJ9',
        publicKey: 'MFkw',
        publicKeyAlgorithm: -7,
        transports: ['internal', 'hybrid'],
      },
      type: 'public-key',
    });
    expect(JSON.parse(JSON.stringify(cred))).toEqual(json);
  });

  it('looks like a PublicKeyCredential from get()', async () => {
    const cred = (await answer(navigator.credentials.get(requestOptions()), ASSERTION)) as PublicKeyCredential;
    const response = cred.response as AuthenticatorAssertionResponse;
    expect(response).toBeInstanceOf(FakeAssertionResponse);
    expect(buf(response.authenticatorData)).toEqual([5, 6, 7]);
    expect(buf(response.signature)).toEqual([0x30, 0x45, 0x02]);
    expect(new TextDecoder().decode(response.userHandle!)).toBe('user');
    expect(Object.keys(cred.toJSON() as object)).toEqual(['authenticatorAttachment', 'clientExtensionResults', 'id', 'rawId', 'response', 'type']);
    expect((cred.toJSON() as { response: unknown }).response).toEqual({
      authenticatorData: 'BQYH',
      clientDataJSON: 'eyJ9',
      signature: 'MEUC',
      userHandle: 'dXNlcg',
    });
  });

  it('refuses a credential of the wrong kind', async () => {
    const p = navigator.credentials.get(requestOptions());
    await expect(answer(p, ATTESTATION)).rejects.toMatchObject({ name: 'NotAllowedError' });
  });
});

describe('conditional mediation', () => {
  let nativeSignal: AbortSignal | undefined;
  let resolveNative: (v: unknown) => void;
  let rejectNative: (e: unknown) => void;

  beforeEach(() => {
    nativeGet.mockImplementation((_this: unknown, options: { signal?: AbortSignal }) => {
      nativeSignal = options.signal;
      return new Promise((resolve, reject) => {
        resolveNative = resolve;
        rejectNative = reject;
        options.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
      });
    });
    install();
    bridge = fakeBridge();
  });

  const conditional = (signal?: AbortSignal) => navigator.credentials.get(requestOptions({}, { mediation: 'conditional', ...(signal ? { signal } : {}) }));

  it('resolves with ours when the user picks a Passbubble passkey, aborting the browser request', async () => {
    const p = conditional();
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    expect(nativeGet).toHaveBeenCalledTimes(1);
    expect(bridge!.requests()[0].request.mediation).toBe('conditional');
    bridge!.reply({ kind: 'credential', id: bridge!.requests()[0].id, credential: ASSERTION });
    const cred = (await p) as PublicKeyCredential;
    expect(cred.id).toBe('AQIDBA');
    expect(nativeSignal?.aborted).toBe(true);
  });

  it('resolves with the browser when it answers first and withdraws ours', async () => {
    const p = conditional();
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    resolveNative('browser-credential');
    await expect(p).resolves.toBe('browser-credential');
    await vi.waitFor(() => expect(bridge!.cancels()).toEqual([bridge!.requests()[0].id]));
  });

  it('ignores browser errors while ours is open', async () => {
    const p = conditional();
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    rejectNative(new DOMException('nope', 'NotAllowedError'));
    await tick(10);
    bridge!.reply({ kind: 'credential', id: bridge!.requests()[0].id, credential: ASSERTION });
    await expect(p).resolves.toMatchObject({ id: 'AQIDBA' });
  });

  it("reports the browser's error once ours drops out", async () => {
    const p = conditional();
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    rejectNative(new DOMException('nope', 'NotAllowedError'));
    await tick(10);
    bridge!.reply({ kind: 'native', id: bridge!.requests()[0].id });
    await expect(p).rejects.toMatchObject({ name: 'NotAllowedError', message: 'nope' });
  });

  it('rejects both on the page abort', async () => {
    const controller = new AbortController();
    const p = conditional(controller.signal);
    await vi.waitFor(() => expect(bridge!.requests()).toHaveLength(1));
    const reason = new Error('page gave up');
    controller.abort(reason);
    await expect(p).rejects.toBe(reason);
    expect(nativeSignal?.aborted).toBe(true);
    await vi.waitFor(() => expect(bridge!.cancels()).toHaveLength(1));
  });
});

describe('static methods', () => {
  it('report a platform authenticator while the feature is on', async () => {
    install();
    bridge = fakeBridge({ enabled: true });
    const PKC = window.PublicKeyCredential as unknown as typeof FakePublicKeyCredential;
    await expect(PKC.isUserVerifyingPlatformAuthenticatorAvailable()).resolves.toBe(true);
    await expect(PKC.isConditionalMediationAvailable()).resolves.toBe(true);
    await expect(PKC.getClientCapabilities()).resolves.toEqual({
      hybridTransport: true,
      conditionalGet: true,
      passkeyPlatformAuthenticator: true,
      userVerifyingPlatformAuthenticator: true,
    });
    expect(originalUvpaa).not.toHaveBeenCalled();
  });

  it('answer like the browser when the feature is off or the bridge stays silent', async () => {
    install();
    bridge = fakeBridge({ enabled: false });
    const PKC = window.PublicKeyCredential as unknown as typeof FakePublicKeyCredential;
    await expect(PKC.isUserVerifyingPlatformAuthenticatorAvailable()).resolves.toBe(false);
    await expect(PKC.getClientCapabilities()).resolves.toEqual({ hybridTransport: true, conditionalGet: false });
    bridge.stop();
    uninstall!();
    install();
    bridge = fakeBridge({ ready: false });
    await expect(PKC.isConditionalMediationAvailable()).resolves.toBe(false);
  });

  it('waits for the feature state that arrives after the bridge is ready', async () => {
    install();
    bridge = fakeBridge({ enabled: null });
    const PKC = window.PublicKeyCredential as unknown as typeof FakePublicKeyCredential;
    const answer = PKC.isUserVerifyingPlatformAuthenticatorAvailable();
    await tick(5);
    bridge.reply({ kind: 'status', enabled: true });
    await expect(answer).resolves.toBe(true);
  });

  it('are restored on uninstall', () => {
    install();
    uninstall!();
    uninstall = null;
    expect(window.PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable).toBe(originalUvpaa);
  });
});

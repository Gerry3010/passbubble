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

import { describe, expect, it } from 'vitest';
import { generateX25519, x25519PublicKey, x25519SharedSecret } from '../x25519.js';

// Test vectors verified by both Go (golang.org/x/crypto/curve25519)
// and @noble/curves/ed25519 x25519.
// Bob's key pair uses RFC 7748 §6.1 bytes which both implementations agree on.
const bobPrivHex = '5dab087e624a8a4b79e17f8b83800ee66f3bb1292618b6fd1c2f8b27ff88e0eb';
const bobPubHex = 'de9edb7d7b7dc1b4d35b61c2ece435373f8343c85b78674dadfc7e146f882b4f';
// Alice public key from RFC 7748 §6.1 (used to verify shared secret from Bob's side)
const alicePubHex = '8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a';
const sharedHex = '4a5d9d5ba4ce2de1728e3bf480350f25e07e21c947d19e3376f09b3c1e161742';

function fromHex(h: string): Uint8Array {
  return new Uint8Array(h.match(/../g)!.map((b) => parseInt(b, 16)));
}

describe('x25519', () => {
  it('RFC 7748 §6.1 Bob public key from private key', () => {
    const pub = x25519PublicKey(fromHex(bobPrivHex));
    expect(Buffer.from(pub).toString('hex')).toBe(bobPubHex);
  });

  it('RFC 7748 §6.1 shared secret from Bob perspective (Bob priv + Alice pub)', () => {
    const ss = x25519SharedSecret(fromHex(bobPrivHex), fromHex(alicePubHex));
    expect(Buffer.from(ss).toString('hex')).toBe(sharedHex);
  });

  it('ECDH is symmetric: (Alice priv, Bob pub) === (Bob priv, Alice pub)', () => {
    const { priv: aPriv, pub: aPub } = generateX25519();
    const { priv: bPriv, pub: bPub } = generateX25519();
    const ss1 = x25519SharedSecret(aPriv, bPub);
    const ss2 = x25519SharedSecret(bPriv, aPub);
    expect(ss1).toEqual(ss2);
  });

  it('public key derivation is deterministic', () => {
    const { priv } = generateX25519();
    const pub1 = x25519PublicKey(priv);
    const pub2 = x25519PublicKey(priv);
    expect(pub1).toEqual(pub2);
  });

  it('different key pairs produce different shared secrets', () => {
    const { priv: aPriv } = generateX25519();
    const { pub: bPub } = generateX25519();
    const { pub: cPub } = generateX25519();
    const ss1 = x25519SharedSecret(aPriv, bPub);
    const ss2 = x25519SharedSecret(aPriv, cPub);
    expect(ss1).not.toEqual(ss2);
  });

  it('generated key pair is 32 bytes', () => {
    const { priv, pub } = generateX25519();
    expect(priv.length).toBe(32);
    expect(pub.length).toBe(32);
  });
});

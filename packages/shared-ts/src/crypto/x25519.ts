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

// X25519 ECDH using @noble/curves.
// Mirrors cloudflare/circl dh/x25519 used in the Go backend.

import { x25519 } from '@noble/curves/ed25519';

export function x25519SharedSecret(myPriv: Uint8Array, theirPub: Uint8Array): Uint8Array {
  return x25519.getSharedSecret(myPriv, theirPub);
}

export function x25519PublicKey(priv: Uint8Array): Uint8Array {
  return x25519.getPublicKey(priv);
}

export function generateX25519(): { priv: Uint8Array; pub: Uint8Array } {
  const priv = x25519.utils.randomPrivateKey();
  const pub = x25519.getPublicKey(priv);
  return { priv, pub };
}

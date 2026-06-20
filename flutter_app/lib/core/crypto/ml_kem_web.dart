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

// Web hybrid KEM via dart:js_interop, calling the JS implementation bundled from
// packages/shared-ts (the same library the browser extension uses). The bundle
// exposes globalThis.passbubbleCrypto and is loaded from web/index.html. The
// wire format matches the native FFI / CLI / backend. dart:ffi is unavailable on
// web, which is why this separate implementation exists (see ml_kem.dart).

import 'dart:js_interop';
import 'dart:typed_data';

@JS('passbubbleCrypto.generateMlKem768')
external JSPromise<JSObject> _generate();

@JS('passbubbleCrypto.encryptDataKey')
external JSPromise<JSUint8Array> _encrypt(JSUint8Array dataKey, JSUint8Array pubX, JSUint8Array pubM);

@JS('passbubbleCrypto.decryptDataKey')
external JSPromise<JSUint8Array> _decrypt(JSUint8Array encKey, JSUint8Array privX, JSUint8Array privM);

extension type _KeyPair(JSObject _) implements JSObject {
  external JSUint8Array get priv;
  external JSUint8Array get pub;
}

/// Generates a fresh ML-KEM-768 keypair as (privateKey, publicKey).
Future<(Uint8List, Uint8List)> mlKemGenerate() async {
  final kp = _KeyPair(await _generate().toDart);
  return (kp.priv.toDart, kp.pub.toDart);
}

/// Wraps [dataKey] for a recipient (hybrid X25519 + ML-KEM, with X25519-only
/// fallback when [recipMlkemPub] is not a valid key).
Future<Uint8List> mlKemEncryptDataKey(
    Uint8List dataKey, Uint8List recipX25519Pub, Uint8List recipMlkemPub) async {
  final out = await _encrypt(dataKey.toJS, recipX25519Pub.toJS, recipMlkemPub.toJS).toDart;
  return out.toDart;
}

/// Unwraps a data key, auto-detecting hybrid vs. legacy X25519-only formats.
Future<Uint8List> mlKemDecryptDataKey(
    Uint8List encKey, Uint8List privX25519, Uint8List privMlkem) async {
  final out = await _decrypt(encKey.toJS, privX25519.toJS, privMlkem.toJS).toDart;
  return out.toDart;
}

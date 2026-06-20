// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version. See <https://www.gnu.org/licenses/>.

// Web entry point: exposes the shared-ts hybrid KEM (X25519 + ML-KEM-768) on
// globalThis.passbubbleCrypto for the Flutter web app to call via dart:js_interop
// (lib/core/crypto/ml_kem_web.dart). Bundled by build.sh into
// flutter_app/web/passbubble_crypto.js. Same library the browser extension uses,
// so the wire format matches the native FFI / CLI / backend.

import { encryptDataKey, decryptDataKey } from '../../packages/shared-ts/src/crypto/hybrid-kem.js';
import { generateMLKEM768 } from '../../packages/shared-ts/src/crypto/mlkem.js';

globalThis.passbubbleCrypto = {
  generateMlKem768: async () => {
    const { priv, pub } = await generateMLKEM768();
    return { priv, pub };
  },
  encryptDataKey: (dataKey, recipX25519Pub, recipMlkemPub) =>
    encryptDataKey(dataKey, recipX25519Pub, recipMlkemPub),
  decryptDataKey: (encKey, privX25519, privMlkem) =>
    decryptDataKey(encKey, privX25519, privMlkem),
};

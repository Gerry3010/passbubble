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

// Platform-agnostic hybrid KEM (X25519 + ML-KEM-768). Resolves to the native
// dart:ffi implementation on mobile/desktop and the dart:js_interop (JS) one on
// web. Both expose the same async API and identical wire format:
//   Future<(Uint8List priv, Uint8List pub)> mlKemGenerate()
//   Future<Uint8List> mlKemEncryptDataKey(dataKey, recipX25519Pub, recipMlkemPub)
//   Future<Uint8List> mlKemDecryptDataKey(encKey, privX25519, privMlkem)
export 'ml_kem_ffi.dart' if (dart.library.js_interop) 'ml_kem_web.dart';

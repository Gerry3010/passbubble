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

// Package main builds a c-shared library exposing the Passbubble hybrid KEM
// (X25519 + ML-KEM-768) to Flutter via dart:ffi on native platforms (Android,
// iOS, Linux, macOS, Windows). It is a thin wrapper around backend/pkg/crypto —
// the single source of truth — so the wire format stays identical to the CLI,
// backend and the browser extension. Flutter web cannot use FFI and instead
// calls the JS implementation (see lib/core/crypto/ml_kem_web.dart).
package main

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"unsafe"

	"github.com/Gerry3010/passbubble/backend/pkg/crypto"
)

// toC copies a Go byte slice into freshly C-allocated memory and returns the
// pointer + length through the out-params. The caller must free it with pb_free.
func toC(b []byte, out **C.uchar, outLen *C.int) {
	if len(b) == 0 {
		*out = nil
		*outLen = 0
		return
	}
	p := C.malloc(C.size_t(len(b)))
	C.memcpy(p, unsafe.Pointer(&b[0]), C.size_t(len(b)))
	*out = (*C.uchar)(p)
	*outLen = C.int(len(b))
}

func goBytes(p *C.uchar, n C.int) []byte {
	if p == nil || n <= 0 {
		return nil
	}
	return C.GoBytes(unsafe.Pointer(p), n)
}

//export pb_generate_mlkem768
//
// Generates a fresh ML-KEM-768 keypair. Returns 0 on success; the private and
// public keys are written to the out-params and must each be freed with pb_free.
func pb_generate_mlkem768(outPriv **C.uchar, outPrivLen *C.int, outPub **C.uchar, outPubLen *C.int) C.int {
	priv, pub, err := crypto.GenerateMLKEM768()
	if err != nil {
		return -1
	}
	toC(priv, outPriv, outPrivLen)
	toC(pub, outPub, outPubLen)
	return 0
}

//export pb_encrypt_data_key
//
// Wraps a data key for a recipient using the hybrid KEM. Falls back to the
// X25519-only legacy format inside crypto.EncryptDataKey when the ML-KEM public
// key is not a valid 1184-byte key. Returns 0 on success; `out` must be freed.
func pb_encrypt_data_key(
	dataKey *C.uchar, dataKeyLen C.int,
	pubX *C.uchar, pubXLen C.int,
	pubM *C.uchar, pubMLen C.int,
	out **C.uchar, outLen *C.int,
) C.int {
	enc, err := crypto.EncryptDataKey(goBytes(dataKey, dataKeyLen), goBytes(pubX, pubXLen), goBytes(pubM, pubMLen))
	if err != nil {
		return -1
	}
	toC(enc, out, outLen)
	return 0
}

//export pb_decrypt_data_key
//
// Unwraps a data key, auto-detecting hybrid vs. legacy X25519-only wire formats.
// Returns 0 on success; `out` must be freed with pb_free.
func pb_decrypt_data_key(
	encKey *C.uchar, encKeyLen C.int,
	privX *C.uchar, privXLen C.int,
	privM *C.uchar, privMLen C.int,
	out **C.uchar, outLen *C.int,
) C.int {
	dk, err := crypto.DecryptDataKey(goBytes(encKey, encKeyLen), goBytes(privX, privXLen), goBytes(privM, privMLen))
	if err != nil {
		return -1
	}
	toC(dk, out, outLen)
	return 0
}

//export pb_free
//
// Frees memory previously returned by one of the pb_* functions.
func pb_free(p *C.uchar) {
	C.free(unsafe.Pointer(p))
}

func main() {}

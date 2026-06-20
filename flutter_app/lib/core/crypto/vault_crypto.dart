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

import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';

import 'ml_kem.dart';

/// Client-side E2E crypto for Passbubble.
///
/// Uses X25519 + AES-256-GCM + Argon2id.
/// Note: ML-KEM-768 is not yet available in pure Dart; PQ layer is planned
/// via FFI in Phase 2. For now X25519 ECDH provides classical security.
class VaultCrypto {
  static final _aesGcm = AesGcm.with256bits();

  // ── Key derivation ─────────────────────────────────────────────────────────

  /// Derives a 32-byte master key from the user's master password using Argon2id.
  static Future<SecretKey> deriveMasterKey(
    String password,
    Uint8List salt, {
    int memory = 65536,
    int iterations = 3,
  }) async {
    final argon = Argon2id(
      memory: memory,
      parallelism: 4,
      iterations: iterations,
      hashLength: 32,
    );
    return argon.deriveKeyFromPassword(
      password: password,
      nonce: salt,
    );
  }

  // ── Symmetric encryption ───────────────────────────────────────────────────

  /// Encrypts plaintext with a 32-byte key using AES-256-GCM.
  /// Returns nonce (12 bytes) || ciphertext.
  static Future<Uint8List> encrypt(SecretKey key, Uint8List plaintext) async {
    final secretBox = await _aesGcm.encrypt(plaintext, secretKey: key);
    final nonce = Uint8List.fromList(secretBox.nonce);
    final ct = Uint8List.fromList([...secretBox.cipherText, ...secretBox.mac.bytes]);
    return Uint8List.fromList([...nonce, ...ct]);
  }

  /// Decrypts nonce||ciphertext produced by [encrypt].
  static Future<Uint8List> decrypt(SecretKey key, Uint8List data) async {
    const nonceLen = 12;
    const macLen = 16;
    if (data.length < nonceLen + macLen) {
      throw Exception('Ciphertext too short');
    }
    final nonce = data.sublist(0, nonceLen);
    final ctAndMac = data.sublist(nonceLen);
    final ct = ctAndMac.sublist(0, ctAndMac.length - macLen);
    final mac = ctAndMac.sublist(ctAndMac.length - macLen);

    final secretBox = SecretBox(ct, nonce: nonce, mac: Mac(mac));
    final plain = await _aesGcm.decrypt(secretBox, secretKey: key);
    return Uint8List.fromList(plain);
  }

  // ── Private key management ─────────────────────────────────────────────────

  /// Decrypts an encrypted private key using the master key.
  static Future<Uint8List> decryptPrivateKey(
    SecretKey masterKey,
    String encPrivKeyBase64,
  ) async {
    final encPrivKey = base64.decode(encPrivKeyBase64);
    return decrypt(masterKey, encPrivKey);
  }

  // ── Key generation ─────────────────────────────────────────────────────────

  /// Generates a fresh X25519 key pair.
  static Future<SimpleKeyPair> generateX25519KeyPair() async {
    return X25519().newKeyPair();
  }

  /// Generates 32 random bytes (for use as a data encryption key).
  static Uint8List randomKey() {
    final rng = Random.secure();
    return Uint8List.fromList(
      List.generate(32, (_) => rng.nextInt(256)),
    );
  }

  /// Encrypts a share-link payload (JSON map) with the given link key and returns
  /// the base64 ciphertext (nonce-prefixed), as the public share viewer expects.
  static Future<String> encryptShareLinkPayload(
    Uint8List linkKey,
    Map<String, dynamic> data,
  ) async {
    final plaintext = Uint8List.fromList(utf8.encode(jsonEncode(data)));
    final encrypted = await encrypt(SecretKey(linkKey), plaintext);
    return base64.encode(encrypted);
  }

  // ── Entry encryption ───────────────────────────────────────────────────────

  /// Encrypts entry data (JSON-encoded map) with a random data key.
  /// Returns (encryptedData, dataKey) as base64 strings.
  static Future<({String encryptedData, String dataNonce, Uint8List dataKey})>
      encryptEntryData(Map<String, dynamic> data) async {
    final dataKey = randomKey();
    final secretKey = SecretKey(dataKey);
    final plaintext = Uint8List.fromList(utf8.encode(jsonEncode(data)));
    final encrypted = await encrypt(secretKey, plaintext);
    final placeholder = Uint8List(12);
    return (
      encryptedData: base64.encode(encrypted),
      dataNonce: base64.encode(placeholder),
      dataKey: dataKey,
    );
  }

  /// Decrypts an entry's encrypted_data using the provided data key.
  static Future<Map<String, dynamic>> decryptEntryData(
    String encryptedDataBase64,
    Uint8List dataKey,
  ) async {
    final secretKey = SecretKey(dataKey);
    final ciphertext = base64.decode(encryptedDataBase64);
    final plaintext = await decrypt(secretKey, ciphertext);
    return jsonDecode(utf8.decode(plaintext)) as Map<String, dynamic>;
  }

  // ── Data key wrapping (hybrid KEM: X25519 + ML-KEM-768) ───────────────────
  // Delegates to the platform hybrid KEM (native dart:ffi → Go, or web JS), so
  // the wire format is identical to the CLI, backend and browser extension.
  // Recipients without a real ML-KEM key (X25519-only accounts) transparently
  // fall back to the legacy format, and decrypt auto-detects hybrid vs. legacy.

  /// Wraps a data key for a recipient using the hybrid KEM. Both public keys are
  /// base64-encoded; pass the recipient's `pub_x25519` and `pub_mlkem768`.
  static Future<String> encryptDataKey(
    Uint8List dataKey,
    String recipientPubX25519Base64,
    String recipientPubMlkem768Base64,
  ) async {
    final enc = await mlKemEncryptDataKey(
      dataKey,
      base64.decode(recipientPubX25519Base64),
      base64.decode(recipientPubMlkem768Base64),
    );
    return base64.encode(enc);
  }

  /// Unwraps a data key produced by [encryptDataKey] (or a legacy X25519-only
  /// blob). Pass the caller's `privX25519` and `privMlkem` private key bytes.
  static Future<Uint8List> decryptDataKey(
    String encryptedKeyBase64,
    Uint8List privX25519Bytes,
    Uint8List privMlkemBytes,
  ) async {
    return mlKemDecryptDataKey(
      base64.decode(encryptedKeyBase64),
      privX25519Bytes,
      privMlkemBytes,
    );
  }

  // ── Salt generation ────────────────────────────────────────────────────────

  static Uint8List randomSalt([int length = 32]) {
    final rng = Random.secure();
    return Uint8List.fromList(List.generate(length, (_) => rng.nextInt(256)));
  }
}

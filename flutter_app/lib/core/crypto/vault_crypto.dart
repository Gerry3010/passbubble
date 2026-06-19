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

  /// Derives a *deterministic* 32-byte share-link key for a resource from the
  /// owner's private key. Because it's deterministic, re-sharing the same entry
  /// always yields the same key (and therefore the same link URL) instead of a
  /// new one each time. It is distinct from the entry's data key (HKDF with a
  /// dedicated salt + the resource id as info), so the real data key is never
  /// exposed in the link.
  static Future<Uint8List> deriveShareLinkKey(
    Uint8List ownerPrivKey,
    String resourceId,
  ) async {
    final hkdf = Hkdf(hmac: Hmac.sha256(), outputLength: 32);
    final key = await hkdf.deriveKey(
      secretKey: SecretKey(ownerPrivKey),
      nonce: utf8.encode('passbubble-share-link-v1'),
      info: utf8.encode(resourceId),
    );
    return Uint8List.fromList(await key.extractBytes());
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

  // ── Data key wrapping (X25519 ECDH + AES-GCM) ─────────────────────────────
  // Note: This implements classical X25519 ECDH key encapsulation.
  // The full hybrid KEM (X25519 + ML-KEM-768) will be added via FFI.

  /// Encrypts a data key for a recipient using X25519 ECDH + AES-GCM.
  static Future<String> encryptDataKey(
    Uint8List dataKey,
    String recipientPubX25519Base64,
  ) async {
    final recipPubBytes = base64.decode(recipientPubX25519Base64);
    // Generate ephemeral key pair
    final ephKeyPair = await X25519().newKeyPair();
    final ephPubBytes =
        Uint8List.fromList((await ephKeyPair.extractPublicKey()).bytes);

    // ECDH
    final recipPubKey = SimplePublicKey(recipPubBytes, type: KeyPairType.x25519);
    final sharedSecret = await X25519().sharedSecretKey(
      keyPair: ephKeyPair,
      remotePublicKey: recipPubKey,
    );

    // Derive wrapping key from shared secret (HKDF-SHA256 simplified: use shared directly)
    final wrapKey = SecretKey(await sharedSecret.extractBytes());
    final encKey = await encrypt(wrapKey, dataKey);

    // Wire format: ephPub(32) || nonce+ciphertext
    final result = Uint8List.fromList([...ephPubBytes, ...encKey]);
    return base64.encode(result);
  }

  /// Decrypts a data key encrypted with [encryptDataKey].
  static Future<Uint8List> decryptDataKey(
    String encryptedKeyBase64,
    Uint8List privX25519Bytes,
  ) async {
    final encKey = base64.decode(encryptedKeyBase64);
    if (encKey.length < 32) throw Exception('Encrypted key too short');

    final ephPubBytes = encKey.sublist(0, 32);
    final remainder = encKey.sublist(32);

    final privKey = await X25519().newKeyPairFromSeed(privX25519Bytes);
    final ephPub = SimplePublicKey(ephPubBytes, type: KeyPairType.x25519);

    final sharedSecret = await X25519().sharedSecretKey(
      keyPair: privKey,
      remotePublicKey: ephPub,
    );

    final wrapKey = SecretKey(await sharedSecret.extractBytes());
    return decrypt(wrapKey, Uint8List.fromList(remainder));
  }

  // ── Salt generation ────────────────────────────────────────────────────────

  static Uint8List randomSalt([int length = 32]) {
    final rng = Random.secure();
    return Uint8List.fromList(List.generate(length, (_) => rng.nextInt(256)));
  }
}

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

import 'dart:typed_data';

import '../api/models.dart';
import 'vault_crypto.dart';

/// Why an existing entry cannot be re-encrypted. The entry has not been changed.
class EntryKeyException implements Exception {
  final String message;
  const EntryKeyException(this.message);

  @override
  String toString() => message;
}

/// Opens [entry] (fresh from the server) for an update: unwraps its data key
/// with the caller's private keys and decrypts its data with that key.
///
/// Re-encrypt with the returned key ([VaultCrypto.encryptEntryDataWithKey]) and
/// send no `entry_keys`: the server only upserts the keys it receives, so a new
/// data key would lock out every other recipient of the entry.
///
/// Throws [EntryKeyException] when the key is missing or cannot be unwrapped,
/// and also when it does not decrypt the stored data. That last case is an entry
/// another client has re-keyed; encrypting over it with the caller's stale key
/// would lock out whoever holds the current one.
Future<({Uint8List dataKey, Map<String, dynamic> data})> openEntryForUpdate(
  EntryResponse entry, {
  required Uint8List? privX25519,
  required Uint8List? privMlkem,
}) async {
  if (privX25519 == null || privMlkem == null) {
    throw const EntryKeyException('Vault is locked');
  }
  final wrapped = entry.entryKey;
  if (wrapped == null) {
    throw EntryKeyException(
        'This account has no key for "${entry.name}", so it was not changed');
  }
  final Uint8List dataKey;
  try {
    dataKey = await VaultCrypto.decryptDataKey(
        wrapped.encryptedKey, privX25519, privMlkem);
  } catch (_) {
    throw EntryKeyException(
        'Could not unlock the key of "${entry.name}", so it was not changed');
  }
  try {
    final data = await VaultCrypto.decryptEntryData(entry.encryptedData, dataKey);
    return (dataKey: dataKey, data: data);
  } catch (_) {
    throw EntryKeyException(
        'Could not decrypt "${entry.name}" with its key, so it was not changed');
  }
}

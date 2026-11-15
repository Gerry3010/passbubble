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

// Writing passkeys into the vault: the entry rules of docs/passkeys.md
// ("Creating a passkey", steps 4 and 5), moving and deleting them. Every write
// keeps the entry's data key, so the people it is shared with keep access.

import 'dart:typed_data';

import '../api/api_client.dart';
import '../api/models.dart';
import '../auth/auth_service.dart';
import '../crypto/entry_update.dart';
import '../crypto/vault_crypto.dart';
import 'passkey.dart';

/// A decrypted vault entry.
typedef DecryptedEntry = ({EntryResponse entry, Map<String, dynamic> data});

/// Re-encrypts the data of entry [entryId] after [mutate] changed it, read fresh
/// from the server, with the entry's own data key and without `entry_keys`, so
/// everyone it is shared with can still decrypt it. The folder is sent along,
/// since the server resets it otherwise. Returns the data as written.
Future<Map<String, dynamic>> rewriteEntryData(
  ApiClient api,
  AuthService auth,
  String entryId,
  Map<String, dynamic> Function(Map<String, dynamic> data) mutate,
) async {
  final full = await api.getEntry(entryId);
  final opened = await openEntryForUpdate(
    full,
    privX25519: auth.privX25519,
    privMlkem: auth.privMLKEM,
  );
  final next = mutate(Map.of(opened.data));
  final enc = await VaultCrypto.encryptEntryDataWithKey(next, opened.dataKey);
  await api.updateEntry(
    entryId,
    UpdateEntryRequest(
      folderId: full.folderId,
      encryptedData: enc.encryptedData,
      dataNonce: enc.dataNonce,
    ),
  );
  return next;
}

/// Every entry of the vault the caller can decrypt, optionally only those of
/// [type]. Entries that fail to decrypt are left out.
Future<List<DecryptedEntry>> loadDecryptedEntries(
  ApiClient api,
  AuthService auth, {
  String? type,
}) async {
  final privX = auth.privX25519;
  final privM = auth.privMLKEM;
  if (privX == null || privM == null) throw const EntryKeyException('Vault is locked');
  final out = <DecryptedEntry>[];
  for (final e in await api.listEntriesFull()) {
    if (type != null && e.type != type) continue;
    final key = e.entryKey;
    if (key == null || e.encryptedData.isEmpty) continue;
    try {
      final dataKey = await VaultCrypto.decryptDataKey(key.encryptedKey, privX, privM);
      final data = await VaultCrypto.decryptEntryData(e.encryptedData, Uint8List.fromList(dataKey));
      out.add((entry: e, data: data));
    } catch (_) {
      // Not readable with this account's key: not a candidate.
    }
  }
  return out;
}

/// [record] with its ids as base64url without padding, the rp id lowercased and
/// the fields a writer always sets. The private key goes to the long PKCS#8
/// form when it parses; otherwise it stays as it came, since the record exists
/// nowhere else.
Map<String, dynamic> normalizePasskeyRecord(Map<String, dynamic> record) {
  final out = Map<String, dynamic>.of(record);
  for (final key in const ['credential_id', 'user_handle']) {
    final v = out[key];
    if (v is String && v.isNotEmpty) {
      try {
        out[key] = normalizeBase64Url(v);
      } on FormatException {
        // Kept as it is.
      }
    }
  }
  final rpId = out['rp_id'];
  if (rpId is String) out['rp_id'] = normalizeRpId(rpId);
  final key = out['private_key'];
  if (key is String && key.isNotEmpty) {
    try {
      out['private_key'] = normalizePrivateKey(key);
    } on PasskeyKeyException {
      // Kept as it is.
    }
  }
  out.putIfAbsent('key_algorithm', () => es256);
  out['sign_count'] = 0;
  out.putIfAbsent('discoverable', () => true);
  out.putIfAbsent('created_at', () => rfc3339Utc(DateTime.now()));
  return out;
}

/// An rp id as the contract compares it: lowercased, without a trailing dot.
/// A pasted URL gives its host.
String normalizeRpId(String input) {
  var s = input.trim().toLowerCase();
  if (s.contains('://')) s = Uri.tryParse(s)?.host ?? s;
  while (s.endsWith('.')) {
    s = s.substring(0, s.length - 1);
  }
  return s;
}

/// The host of an entry URL, lowercased, or '' without one. A URL without a
/// scheme counts as https.
String entryHost(String url) {
  final u = url.trim();
  if (u.isEmpty) return '';
  final uri = Uri.tryParse(u.contains('://') ? u : 'https://$u');
  return normalizeRpId(uri?.host ?? '');
}

/// Whether [host] is [rpId] or a subdomain of it.
bool hostMatchesRpId(String host, String rpId) =>
    rpId.isNotEmpty && (host == rpId || host.endsWith('.$rpId'));

/// The stored passkey array of [data] with [record] in it: it replaces a record
/// with the same credential id, or, with [sameAccountReplaces], one with the
/// same rp id and user handle (the site registered that account again).
/// Otherwise it is appended.
List<Map<String, dynamic>> upsertPasskey(
  Map<String, dynamic> data,
  Map<String, dynamic> record, {
  bool sameAccountReplaces = true,
}) {
  final list = List<Map<String, dynamic>>.of(passkeyRecordsOf(data));
  final id = credentialIdOf(record);
  var at = list.indexWhere((r) => credentialIdOf(r) == id);
  if (at < 0 && sameAccountReplaces) at = list.indexWhere((r) => _sameAccount(r, record));
  if (at < 0) {
    list.add(record);
  } else {
    list[at] = record;
  }
  return list;
}

bool _sameAccount(Map<String, dynamic> a, Map<String, dynamic> b) {
  final pa = Passkey.fromJson(a);
  final pb = Passkey.fromJson(b);
  return pa.rpId.isNotEmpty &&
      pa.rpId == pb.rpId &&
      pa.userHandle.isNotEmpty &&
      pa.userHandle == pb.userHandle;
}

/// Where [attachPasskey] put a passkey.
typedef AttachResult = ({String entryId, bool created, Map<String, dynamic> data});

/// Saves the passkey [record] into the vault by the rules of docs/passkeys.md:
///
/// - into [entryId] when given;
/// - else into an entry already holding this credential id (a second save of
///   the same passkey changes nothing);
/// - else into a `password` entry whose URL host is the rp id or a subdomain of
///   it and whose username equals the passkey's user name (ignoring case),
///   preferring one that holds a passkey for the same account;
/// - else into a new `password` entry named after the site.
///
/// Inside the chosen entry, a record with the same rp id and user handle is
/// replaced.
Future<AttachResult> attachPasskey(
  ApiClient api,
  AuthService auth,
  Map<String, dynamic> record, {
  String? entryId,
}) async {
  final rec = normalizePasskeyRecord(record);
  final passkey = Passkey.fromJson(rec);
  if (passkey.rpId.isEmpty || passkey.credentialId.isEmpty) {
    throw ArgumentError('A passkey needs an rp id and a credential id');
  }

  var target = entryId;
  if (target == null) {
    final logins = await loadDecryptedEntries(api, auth, type: 'password');
    target = logins
        .where((e) => passkeyRecordsOf(e.data).any((r) => credentialIdOf(r) == passkey.credentialId))
        .map((e) => e.entry.id)
        .firstOrNull;
    if (target == null) {
      final candidates = logins.where((e) {
        final username = (e.data['username'] as String? ?? '').trim().toLowerCase();
        return hostMatchesRpId(entryHost(e.entry.url), passkey.rpId) &&
            username == passkey.userName.trim().toLowerCase();
      }).toList();
      target = (candidates
                  .where((e) => passkeyRecordsOf(e.data).any((r) => _sameAccount(r, rec)))
                  .firstOrNull ??
              candidates.firstOrNull)
          ?.entry
          .id;
    }
  }

  if (target != null) {
    final data = await rewriteEntryData(
        api, auth, target, (d) => d..[passkeysKey] = upsertPasskey(d, rec));
    return (entryId: target, created: false, data: data);
  }

  final data = <String, dynamic>{
    if (passkey.userName.isNotEmpty) 'username': passkey.userName,
    passkeysKey: [rec],
  };
  final id = await createPasswordEntry(
    api,
    auth,
    name: passkey.rpName.isNotEmpty ? passkey.rpName : passkey.rpId,
    url: 'https://${passkey.rpId}',
    data: data,
  );
  return (entryId: id, created: true, data: data);
}

/// Creates a `password` entry holding [data], encrypted for the caller alone
/// (the create path of the edit screen). Returns its id.
Future<String> createPasswordEntry(
  ApiClient api,
  AuthService auth, {
  required String name,
  required String url,
  required Map<String, dynamic> data,
  String? folderId,
}) async {
  final (:encryptedData, :dataNonce, :dataKey) = await VaultCrypto.encryptEntryData(data);
  final pubX = await auth.getPubX25519();
  final pubM = await auth.getPubMlkem768();
  if (pubX == null || pubM == null) throw Exception('Public key not found');
  final userId = await auth.getUserId();
  final wrapped = await VaultCrypto.encryptDataKey(dataKey, pubX, pubM);
  return api.createEntry(CreateEntryRequest(
    folderId: folderId,
    type: 'password',
    name: name,
    url: url,
    encryptedData: encryptedData,
    dataNonce: dataNonce,
    entryKeys: [EntryKey(userId: userId ?? '', encryptedKey: wrapped)],
  ));
}

/// Moves the passkey [credentialId] from entry [fromId] to entry [toId]. The
/// target is written first and the source after it, so a failure in between
/// leaves the passkey in both, never in neither. Returns the source's data as
/// written.
Future<Map<String, dynamic>> movePasskey(
  ApiClient api,
  AuthService auth, {
  required String fromId,
  required String toId,
  required String credentialId,
}) async {
  if (fromId == toId) throw ArgumentError('Source and target are the same entry');
  final id = normalizeBase64Url(credentialId);
  final source = await api.getEntry(fromId);
  final opened = await openEntryForUpdate(
    source,
    privX25519: auth.privX25519,
    privMlkem: auth.privMLKEM,
  );
  final record = passkeyRecordsOf(opened.data).where((r) => credentialIdOf(r) == id).firstOrNull;
  if (record == null) throw StateError('The passkey is no longer in "${source.name}"');
  await rewriteEntryData(api, auth, toId,
      (d) => d..[passkeysKey] = upsertPasskey(d, record, sameAccountReplaces: false));
  return removePasskey(api, auth, fromId, id);
}

/// Deletes the passkey [credentialId] from entry [entryId]. The key goes when
/// the last passkey does. Returns the data as written.
Future<Map<String, dynamic>> removePasskey(
  ApiClient api,
  AuthService auth,
  String entryId,
  String credentialId,
) {
  final id = normalizeBase64Url(credentialId);
  return rewriteEntryData(api, auth, entryId, (d) {
    final rest = passkeyRecordsOf(d).where((r) => credentialIdOf(r) != id).toList();
    if (rest.isEmpty) {
      d.remove(passkeysKey);
    } else {
      d[passkeysKey] = rest;
    }
    return d;
  });
}

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

// A small in-memory vault for tests that write entries with real crypto: the
// server keeps what is created and updated, so a later read sees it. The
// data-key wrapping needs the native hybrid-KEM lib ([nativeCryptoLib]).

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:passbubble/core/api/api_client.dart';
import 'package:passbubble/core/api/models.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/core/autofill/autofill_service.dart';
import 'package:passbubble/core/crypto/ml_kem.dart';
import 'package:passbubble/core/crypto/vault_crypto.dart';

const nativeCryptoLib = 'native/passbubble_crypto/build/libpassbubble_crypto.so';

bool get nativeCryptoBuilt => File(nativeCryptoLib).existsSync();

/// Points the hybrid KEM at the native lib; call in `setUpAll`.
void useNativeCrypto() => mlKemLibraryPathOverride = File(nativeCryptoLib).absolute.path;

/// A test account's key material.
class TestKeys {
  final Uint8List privX;
  final Uint8List privM;
  final String pubXB64;
  final String pubMB64;
  TestKeys(this.privX, this.privM, this.pubXB64, this.pubMB64);

  static Future<TestKeys> generate() async {
    final x = await VaultCrypto.generateX25519KeyPair();
    final privX = Uint8List.fromList(await x.extractPrivateKeyBytes());
    final pubX = Uint8List.fromList((await x.extractPublicKey()).bytes);
    final (privM, pubM) = await mlKemGenerate();
    return TestKeys(privX, privM, base64.encode(pubX), base64.encode(pubM));
  }

  Future<EntryResponse> encryptEntry(
    Map<String, dynamic> data, {
    required String id,
    String type = 'password',
    String name = 'Mail',
    String url = 'https://mail.example.com',
    String? folderId,
    String permission = 'owner',
  }) async {
    final enc = await VaultCrypto.encryptEntryData(data);
    final wrapped = await VaultCrypto.encryptDataKey(enc.dataKey, pubXB64, pubMB64);
    return EntryResponse(
      id: id,
      folderId: folderId,
      type: type,
      name: name,
      url: url,
      encryptedData: enc.encryptedData,
      dataNonce: enc.dataNonce,
      entryKey: EntryKey(userId: 'u1', encryptedKey: wrapped),
      permission: permission,
      createdAt: '',
      updatedAt: 'v0',
    );
  }

  Future<Map<String, dynamic>> decrypt(EntryResponse e) async {
    final dataKey = await VaultCrypto.decryptDataKey(e.entryKey!.encryptedKey, privX, privM);
    return VaultCrypto.decryptEntryData(e.encryptedData, dataKey);
  }
}

/// A server holding [entries]; creates and updates change them.
class FakeVaultApi extends ApiClient {
  final Map<String, EntryResponse> entries;
  final updates = <({String id, UpdateEntryRequest req})>[];
  final created = <CreateEntryRequest>[];

  /// Updates of these entry ids fail.
  final failUpdatesOf = <String>{};
  int _version = 0;

  FakeVaultApi(List<EntryResponse> vault) : entries = {for (final e in vault) e.id: e};

  @override
  Future<EntryResponse> getEntry(String id) async =>
      entries[id] ?? (throw StateError('no entry $id'));
  @override
  Future<List<EntryResponse>> listEntries() async => entries.values.toList();
  @override
  Future<List<EntryResponse>> listEntriesFull() async => entries.values.toList();
  @override
  Future<List<FolderResponse>> listFolders() async => const [];

  @override
  Future<void> updateEntry(String id, UpdateEntryRequest req) async {
    if (failUpdatesOf.contains(id)) throw StateError('update of $id failed');
    updates.add((id: id, req: req));
    final e = entries[id]!;
    entries[id] = EntryResponse(
      id: e.id,
      folderId: req.folderId,
      type: e.type,
      name: req.name ?? e.name,
      url: req.url ?? e.url,
      encryptedData: req.encryptedData ?? e.encryptedData,
      dataNonce: req.dataNonce ?? e.dataNonce,
      entryKey: e.entryKey,
      permission: e.permission,
      createdAt: e.createdAt,
      updatedAt: 'v${++_version}',
    );
  }

  @override
  Future<String> createEntry(CreateEntryRequest req) async {
    created.add(req);
    final id = 'new-${created.length}';
    entries[id] = EntryResponse(
      id: id,
      folderId: req.folderId,
      type: req.type,
      name: req.name,
      url: req.url ?? '',
      encryptedData: req.encryptedData,
      dataNonce: req.dataNonce,
      entryKey: req.entryKeys.single,
      permission: 'owner',
      createdAt: '',
      updatedAt: 'v${++_version}',
    );
    return id;
  }
}

/// An unlocked vault for [keys] without touching secure storage.
class FakeVaultAuth extends AuthService {
  final TestKeys keys;

  /// Whether [refreshAutofill] does the real sync; otherwise it only counts.
  final bool realSync;
  int autofillRefreshes = 0;

  FakeVaultAuth(super.api, this.keys, {super.autofill, this.realSync = false});

  @override
  Uint8List? get privX25519 => keys.privX;
  @override
  Uint8List? get privMLKEM => keys.privM;
  @override
  Future<String?> getPubX25519() async => keys.pubXB64;
  @override
  Future<String?> getPubMlkem768() async => keys.pubMB64;
  @override
  Future<String?> getUserId() async => 'u1';
  @override
  Future<void> refreshAutofill() {
    autofillRefreshes++;
    return realSync ? super.refreshAutofill() : Future.value();
  }
}

/// A bridge that records what the app hands the native side.
class FakeAppleBridge extends AutofillBridge {
  @override
  final bool isApple;
  final bool android;
  String pending;
  final cleared = <List<String>>[];
  String? syncedPasskeys;
  String? syncedCredentials;
  String? androidCredentials;

  FakeAppleBridge({this.isApple = true, this.android = false, this.pending = '[]'});

  @override
  Future<bool> isSupported() async => android;
  @override
  Future<String> readPendingPasskeys() async => pending;
  @override
  Future<void> clearPendingPasskeys(List<String> ids) async {
    if (ids.isNotEmpty) cleared.add(ids);
  }

  @override
  Future<void> syncPasskeys(String passkeysJson) async => syncedPasskeys = passkeysJson;
  @override
  Future<void> syncIosCredentials(String credentialsJson) async =>
      syncedCredentials = credentialsJson;
  @override
  Future<void> updateCredentials(String credentialsJson) async =>
      androidCredentials = credentialsJson;
}

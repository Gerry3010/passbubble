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

// The import/export jobs against a fake server, with real crypto: the data-key
// wrapping needs the native hybrid-KEM lib (`make native-crypto`, built in CI);
// without it these tests skip.
@TestOn('vm')
library;

import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/api/api_client.dart';
import 'package:passbubble/core/api/models.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/core/crypto/ml_kem.dart';
import 'package:passbubble/core/crypto/vault_crypto.dart';
import 'package:passbubble/core/importexport/bitwarden_format.dart';
import 'package:passbubble/core/importexport/csv_format.dart';
import 'package:passbubble/core/importexport/entry_record.dart';
import 'package:passbubble/core/importexport/onepassword_format.dart';
import 'package:passbubble/core/jobs/job_runner.dart';
import 'package:passbubble/core/jobs/local_job.dart';

const _libPath = 'native/passbubble_crypto/build/libpassbubble_crypto.so';

typedef EntryRecordView = ({bool hasTotpKey, String secret});

class _Keys {
  final Uint8List privX;
  final Uint8List privM;
  final String pubXB64;
  final String pubMB64;
  _Keys(this.privX, this.privM, this.pubXB64, this.pubMB64);

  static Future<_Keys> generate() async {
    final x = await VaultCrypto.generateX25519KeyPair();
    final privX = Uint8List.fromList(await x.extractPrivateKeyBytes());
    final pubX = Uint8List.fromList((await x.extractPublicKey()).bytes);
    final (privM, pubM) = await mlKemGenerate();
    return _Keys(privX, privM, base64.encode(pubX), base64.encode(pubM));
  }

  Future<Map<String, dynamic>> decrypt(String encryptedData, String wrappedKey) async {
    final dataKey = await VaultCrypto.decryptDataKey(wrappedKey, privX, privM);
    return VaultCrypto.decryptEntryData(encryptedData, dataKey);
  }
}

class _FakeAuth extends AuthService {
  final _Keys keys;
  _FakeAuth(super.api, this.keys);

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
}

/// One entry on the server. The ledger is down, which the jobs tolerate.
class _FakeApi extends ApiClient {
  EntryResponse entry;

  /// A second listed entry; [getEntry] answers with [entry] for it as well.
  EntryResponse? twin;

  /// When set, what [getEntry] returns once the import has read the vault.
  EntryResponse? entryDuringImport;
  bool _vaultRead = false;

  UpdateEntryRequest? updated;
  CreateEntryRequest? created;
  _FakeApi(this.entry);

  @override
  Future<List<FolderResponse>> listFolders() async => const [];
  @override
  Future<List<EntryResponse>> listEntries() async => [entry, ?twin];
  @override
  Future<EntryResponse> getEntry(String id) async {
    final during = entryDuringImport;
    return _vaultRead && during != null ? during : entry;
  }

  @override
  Future<JobResponse> createJob(CreateJobRequest req) async {
    _vaultRead = true; // the last call before the records are written
    throw StateError('no ledger');
  }

  @override
  Future<void> updateEntry(String id, UpdateEntryRequest req) async => updated = req;
  @override
  Future<String> createEntry(CreateEntryRequest req) async {
    created = req;
    return 'new-id';
  }
}

/// A small vault that remembers what the jobs create, so a later job sees it.
class _VaultApi extends ApiClient {
  final Map<String, EntryResponse> entries = {};
  final List<UpdateEntryRequest> updates = [];

  @override
  Future<List<FolderResponse>> listFolders() async => const [];
  @override
  Future<List<EntryResponse>> listEntries() async => entries.values.toList();
  @override
  Future<EntryResponse> getEntry(String id) async => entries[id]!;
  @override
  Future<JobResponse> createJob(CreateJobRequest req) async => throw StateError('no ledger');
  @override
  Future<void> updateEntry(String id, UpdateEntryRequest req) async => updates.add(req);
  @override
  Future<String> createEntry(CreateEntryRequest req) async {
    final id = 'new-${entries.length}';
    entries[id] = EntryResponse(
      id: id,
      type: req.type,
      name: req.name,
      url: req.url ?? '',
      encryptedData: req.encryptedData,
      dataNonce: req.dataNonce,
      entryKey: req.entryKeys.single,
      permission: 'owner',
      createdAt: '',
      updatedAt: '',
    );
    return id;
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final libExists = File(_libPath).existsSync();
  late _Keys owner;
  late _Keys other;

  setUpAll(() async {
    if (!libExists) return;
    mlKemLibraryPathOverride = File(_libPath).absolute.path;
    owner = await _Keys.generate();
    other = await _Keys.generate();
  });

  /// [data] on the server as the owner sees it, in folder f1, with the data key
  /// also wrapped for [other].
  Future<({EntryResponse entry, Uint8List dataKey, String otherKey})> sharedEntry(
      Map<String, dynamic> data,
      {String type = 'password'}) async {
    final enc = await VaultCrypto.encryptEntryData(data);
    return (
      entry: EntryResponse(
        id: 'e1',
        folderId: 'f1',
        type: type,
        name: 'Mail',
        url: 'https://mail.example.com',
        encryptedData: enc.encryptedData,
        dataNonce: enc.dataNonce,
        entryKey: EntryKey(
          userId: 'u1',
          encryptedKey: await VaultCrypto.encryptDataKey(enc.dataKey, owner.pubXB64, owner.pubMB64),
        ),
        permission: 'owner',
        createdAt: '',
        updatedAt: '',
      ),
      dataKey: enc.dataKey,
      otherKey: await VaultCrypto.encryptDataKey(enc.dataKey, other.pubXB64, other.pubMB64),
    );
  }

  /// A container running the jobs as [owner] against [api].
  ProviderContainer containerFor(ApiClient api) {
    final container = ProviderContainer(overrides: [
      apiClientProvider.overrideWithValue(api),
      authServiceProvider.overrideWithValue(_FakeAuth(api, owner)),
    ]);
    addTearDown(container.dispose);
    return container;
  }

  Future<LocalJob> import(
    ApiClient api,
    String file, {
    ImportFormat format = ImportFormat.csvGeneric,
    OverwriteMode? mode,
    DupStrategy dup = DupStrategy.overwrite,
    Uint8List? fileBytes,
  }) async {
    final container = containerFor(api);
    final runner = container.read(jobRunnerProvider.notifier);
    final bytes = fileBytes ?? Uint8List.fromList(utf8.encode(file));
    // Leaving the mode out is what merges by default.
    await (mode == null
        ? runner.startImport(bytes: bytes, format: format, dup: dup)
        : runner.startImport(bytes: bytes, format: format, dup: dup, overwriteMode: mode));
    return container.read(jobRunnerProvider).single;
  }

  /// What the job created, as the owner decrypts it.
  Future<Map<String, dynamic>> createdData(_FakeApi api) => owner.decrypt(
      api.created!.encryptedData, api.created!.entryKeys.single.encryptedKey);

  /// The job's request as the server would receive it.
  Future<Map<String, dynamic>> seenByOther(
          _FakeApi api, ({EntryResponse entry, Uint8List dataKey, String otherKey}) shared) =>
      other.decrypt(api.updated!.encryptedData!, shared.otherKey);

  const existing = {
    'username': 'alice',
    'password': 'old-pw',
    'tags': ['Existing'],
    'issuer': 'ACME', // managed by another client → must survive
  };

  group('overwrite import', () {
    test('keeps the data key and the folder; a second recipient can still decrypt',
        () async {
      final shared = await sharedEntry(existing);
      final api = _FakeApi(shared.entry);

      final job = await import(api, 'name,url,username,password,notes,tags\n'
          'Mail,https://new.example.com,alice,new-pw,hello,"new, Tag, new"\n');

      expect(job.updatedItems, 1);
      expect(job.failedItems, 0);
      expect(api.created, isNull);

      final req = api.updated!;
      expect(req.entryKeys, isNull, reason: 'nobody\'s wrapped key changes');
      expect(req.folderId, 'f1', reason: 'an overwrite does not move the entry');
      expect(req.url, 'https://new.example.com');

      final direct = await VaultCrypto.decryptEntryData(req.encryptedData!, shared.dataKey);
      expect(direct['password'], 'new-pw');

      final seen = await other.decrypt(req.encryptedData!, shared.otherKey);
      expect(seen['password'], 'new-pw');
      expect(seen['notes'], 'hello');
      expect(seen['tags'], ['new', 'Tag'], reason: 'the record has tags: they win');
      expect(seen['issuer'], 'ACME', reason: 'unknown entry keys are kept');
    });

    test('a record without tags keeps the existing ones', () async {
      final shared = await sharedEntry(existing);
      final api = _FakeApi(shared.entry);

      for (final csv in [
        'name,url,username,password,notes\nMail,,alice,new-pw,\n', // no column
        'name,url,username,password,notes,tags\nMail,,alice,new-pw,,\n', // empty cell
      ]) {
        final job = await import(api, csv);
        expect(job.updatedItems, 1, reason: csv);
        final seen = await other.decrypt(api.updated!.encryptedData!, shared.otherKey);
        expect(seen['password'], 'new-pw', reason: csv);
        expect(seen['tags'], ['Existing'], reason: csv);
      }
    });

    test('counts an entry as failed and leaves it alone when its key is unusable',
        () async {
      final shared = await sharedEntry(existing);
      final rekeyed = await VaultCrypto.encryptEntryData(existing);
      EntryResponse changed({String? wrapped, String? blob}) => EntryResponse(
            id: 'e1',
            type: 'password',
            name: 'Mail',
            url: '',
            encryptedData: blob ?? shared.entry.encryptedData,
            dataNonce: shared.entry.dataNonce,
            entryKey: wrapped == null ? null : EntryKey(userId: 'u1', encryptedKey: wrapped),
            permission: 'owner',
            createdAt: '',
            updatedAt: '',
          );
      final ownKey = shared.entry.entryKey!.encryptedKey;

      for (final broken in [
        changed(), // no key for this account any more
        changed(wrapped: base64.encode(Uint8List(64))), // does not unwrap
        changed(wrapped: ownKey, blob: rekeyed.encryptedData), // stale key
      ]) {
        final api = _FakeApi(shared.entry)..entryDuringImport = broken;

        final job = await import(api, 'name,url,username,password,notes\n'
            'Mail,,alice,new-pw,\n');

        expect(job.failedItems, 1);
        expect(job.updatedItems, 0);
        expect(api.updated, isNull);
        expect(api.created, isNull);
        expect(job.log.where((l) => l.startsWith('failed (update): Mail')), hasLength(1));
      }
    });

    test('leaves several matching entries alone instead of picking one', () async {
      final shared = await sharedEntry(existing);
      final e = shared.entry;
      final api = _FakeApi(e)
        ..twin = EntryResponse(
          id: 'e2',
          type: e.type,
          name: e.name,
          url: e.url,
          encryptedData: e.encryptedData,
          dataNonce: e.dataNonce,
          entryKey: e.entryKey,
          permission: 'owner',
          createdAt: '',
          updatedAt: '',
        );

      final job = await import(api, 'name,url,username,password,notes\n'
          'Mail,,alice,new-pw,\n');

      expect(job.failedItems, 1);
      expect(job.updatedItems, 0);
      expect(api.updated, isNull);
      expect(api.created, isNull);
      expect(job.log.where((l) => l.contains('2 entries match')), hasLength(1));
    });
  }, skip: libExists ? false : 'native crypto lib not built ($_libPath)');

  group('overwrite modes', () {
    const secret = 'JBSWY3DPEHPK3PXP';
    const rich = {
      'username': 'alice',
      'password': 'old-pw',
      'totp_secret': secret,
      'notes': 'old notes',
      'iban': 'AT00 1234',
      'tags': ['Existing'],
      'issuer': 'ACME',
      'custom_fields': [
        {'label': 'PIN', 'value': '1234', 'type': 'password', 'id': 'cf-1'},
        {
          'label': 'Doc',
          'value': 'QUJD',
          'type': 'file',
          'filename': 'a.pdf',
          'mime_type': 'application/pdf',
        },
        {'label': 'Kept', 'value': 'k', 'type': 'text'},
      ],
    };

    test('merge is the default: only values in the file change, and a missing url stays out',
        () async {
      final shared = await sharedEntry(rich);
      final api = _FakeApi(shared.entry);

      final job = await import(api, 'name,url,username,password,notes,tags\n'
          'Mail,,alice,,fresh notes,\n');

      expect(job.updatedItems, 1);
      expect(job.log, contains('Overwrite mode: merge'));
      final json = api.updated!.toJson();
      expect(json.containsKey('url'), isFalse, reason: 'missing means keep');
      expect(json.containsKey('entry_keys'), isFalse);
      expect(json['folder_id'], 'f1');
      expect(json.containsKey('match_patterns'), isFalse);
      expect(json.containsKey('favorite'), isFalse);

      final seen = await seenByOther(api, shared);
      expect(seen['notes'], 'fresh notes');
      expect(seen['password'], 'old-pw', reason: 'an empty cell leaves it alone');
      expect(seen['totp_secret'], secret);
      expect(seen['iban'], 'AT00 1234');
      expect(seen['tags'], ['Existing']);
      expect(seen['issuer'], 'ACME');
      expect(seen['custom_fields'], rich['custom_fields']);
    });

    test('merge: custom fields go by label, the rest stays in place', () async {
      final shared = await sharedEntry(rich);
      final api = _FakeApi(shared.entry);

      await import(
        api,
        jsonEncode({
          'items': [
            {
              'type': 1,
              'name': 'Mail',
              'login': {'username': 'alice', 'password': 'new-pw', 'uris': [{'uri': 'https://x.example'}]},
              'fields': [
                {'name': 'New', 'value': 'n', 'type': 1},
                {'name': 'PIN', 'value': '9999', 'type': 0},
              ],
            },
          ],
        }),
        format: ImportFormat.bitwarden,
      );

      expect(api.updated!.url, 'https://x.example');
      final seen = await seenByOther(api, shared);
      expect(seen['password'], 'new-pw');
      expect(seen['custom_fields'], [
        {'label': 'PIN', 'value': '9999', 'type': 'text', 'id': 'cf-1'},
        (rich['custom_fields']! as List)[1],
        (rich['custom_fields']! as List)[2],
        {'label': 'New', 'value': 'n', 'type': 'password'},
      ]);
    });

    test('complete: the data is exactly the file, and key and folder stay', () async {
      final shared = await sharedEntry(rich);
      final api = _FakeApi(shared.entry);

      final job = await import(
        api,
        'name,url,username,password,notes,tags\nMail,,alice,new-pw,,"a, b"\n',
        mode: OverwriteMode.complete,
      );

      expect(job.updatedItems, 1);
      expect(job.log, contains('Overwrite mode: complete'));
      final json = api.updated!.toJson();
      expect(json['url'], '', reason: 'an empty url clears it');
      expect(json['folder_id'], 'f1');
      expect(json.containsKey('entry_keys'), isFalse);
      expect(json.containsKey('match_patterns'), isFalse);
      expect(json.containsKey('favorite'), isFalse);

      expect(await seenByOther(api, shared), {
        'username': 'alice',
        'password': 'new-pw',
        'tags': ['a', 'b'],
      });
      expect(
          await VaultCrypto.decryptEntryData(api.updated!.encryptedData!, shared.dataKey),
          isA<Map<String, dynamic>>());
    });

    test('complete: a url in the file is sent, and custom fields come from the file',
        () async {
      final shared = await sharedEntry(rich);
      final api = _FakeApi(shared.entry);

      await import(
        api,
        jsonEncode({
          'items': [
            {
              'type': 1,
              'name': 'Mail',
              'login': {'username': 'alice', 'password': 'p', 'uris': [{'uri': 'https://x.example'}]},
              'fields': [
                {'name': 'Only', 'value': 'o', 'type': 0},
              ],
            },
          ],
        }),
        format: ImportFormat.bitwarden,
        mode: OverwriteMode.complete,
      );

      expect(api.updated!.url, 'https://x.example');
      expect(await seenByOther(api, shared), {
        'username': 'alice',
        'password': 'p',
        'custom_fields': [
          {'label': 'Only', 'value': 'o', 'type': 'text'},
        ],
      });
    });

    test('merge: a new secret in the file takes the old options with it', () async {
      final shared = await sharedEntry(const {
        'username': 'alice',
        'password': 'old-pw',
        'totp_secret': secret,
        'period': 60,
        'digits': 8,
        'algorithm': 'SHA256',
        'issuer': 'ACME',
      });
      final api = _FakeApi(shared.entry);

      await import(
        api,
        jsonEncode({
          'items': [
            {
              'type': 1,
              'name': 'Mail',
              'login': {'username': 'alice', 'password': 'old-pw', 'totp': 'GEZDGNBVGY3TQOJQ'},
            },
          ],
        }),
        format: ImportFormat.bitwarden,
      );

      final seen = await seenByOther(api, shared);
      expect(seen['totp_secret'], 'GEZDGNBVGY3TQOJQ');
      expect(seen.containsKey('period'), isFalse);
      expect(seen.containsKey('digits'), isFalse);
      expect(seen.containsKey('algorithm'), isFalse);
      expect(seen['issuer'], 'ACME');
    });

    test('merge: a stray totp_secret does not outlive the password it copied',
        () async {
      final shared = await sharedEntry(const {
        'username': 'alice',
        'password': 'old-pw',
        'totp_secret': 'old-pw',
      });
      final api = _FakeApi(shared.entry);

      await import(api, 'name,url,username,password,notes\nMail,,alice,new-pw,\n');

      final seen = await seenByOther(api, shared);
      expect(seen['password'], 'new-pw');
      expect(seen.containsKey('totp_secret'), isFalse);
    });
  }, skip: libExists ? false : 'native crypto lib not built ($_libPath)');

  group('a stray 2FA copy in the file', () {
    const real = 'JBSWY3DPEHPK3PXP';
    const pw = 'new-pw';

    /// Files of every format with a 2FA value, holding the login [name] whose
    /// secret is just its password.
    Map<String, (ImportFormat, String, Uint8List?)> filesFor(String name) => {
          'bitwarden': (
            ImportFormat.bitwarden,
            jsonEncode({
              'items': [
                {
                  'type': 1,
                  'name': name,
                  'login': {'username': 'alice', 'password': pw, 'totp': pw},
                },
              ],
            }),
            null,
          ),
          'lastpass': (
            ImportFormat.csvLastPass,
            'url,username,password,totp,extra,name,grouping,fav\n,alice,$pw,$pw,,$name,,\n',
            null,
          ),
          'psono': (
            ImportFormat.psono,
            jsonEncode({
              'items': [
                {
                  'type': 'website_password',
                  'website_password_title': name,
                  'website_password_username': 'alice',
                  'website_password_password': pw,
                  'website_password_totp_code': pw,
                },
              ],
            }),
            null,
          ),
          '1pux': (
            ImportFormat.onepassword1pux,
            '',
            exportOnePux([EntryRecord(name: name, username: 'alice', password: pw, totpSecret: pw)]),
          ),
        };

    for (final label in ['bitwarden', 'lastpass', 'psono', '1pux']) {
      test('$label: a merge leaves the real secret and its options alone', () async {
        final shared = await sharedEntry(const {
          'username': 'alice',
          'password': 'old-pw',
          'totp_secret': real,
          'period': 60,
        });
        final api = _FakeApi(shared.entry);
        final (format, file, bytes) = filesFor('Mail')[label]!;

        final job = await import(api, file, format: format, fileBytes: bytes);

        expect(job.updatedItems, 1);
        final seen = await seenByOther(api, shared);
        expect(seen['password'], pw);
        expect(seen['totp_secret'], real);
        expect(seen['period'], 60);
      });

      test('$label: a new entry is a plain login without a secret', () async {
        final shared = await sharedEntry(const {'username': 'alice', 'password': 'old-pw'});
        final api = _FakeApi(shared.entry);
        final (format, file, bytes) = filesFor('Fresh')[label]!;

        final job = await import(api, file, format: format, fileBytes: bytes);

        expect(job.createdItems, 1);
        expect(api.created!.type, 'password');
        final data = await createdData(api);
        expect(data['password'], pw);
        expect(data.containsKey('totp_secret'), isFalse);
      });
    }
  }, skip: libExists ? false : 'native crypto lib not built ($_libPath)');

  group('duplicates go by type family', () {
    final card = jsonEncode({
      'items': [
        {
          'type': 3,
          'name': 'Mail',
          'card': {'number': '4111'},
        },
      ],
    });
    const login = 'name,url,username,password,notes\nMail,,,new-pw,\n';
    final note = jsonEncode({
      'items': [
        {'type': 2, 'name': 'Mail', 'notes': 'new'},
      ],
    });

    for (final dup in DupStrategy.values) {
      test('${dup.name}: a card does not touch a login of the same name, it is created',
          () async {
        final shared = await sharedEntry(const {'password': 'old-pw'});
        final api = _FakeApi(shared.entry);

        final job = await import(api, card, format: ImportFormat.bitwarden, dup: dup);

        expect(job.createdItems, 1);
        expect(job.updatedItems, 0);
        expect(job.skippedItems, 0);
        expect(api.updated, isNull);
        expect(api.created!.type, 'credit-card');
      });

      test('${dup.name}: a login does not match another type of its name', () async {
        for (final type in ['api-key', 'ssh-key', 'note', 'license']) {
          final shared = await sharedEntry(const {'password': 'old-pw'}, type: type);
          final api = _FakeApi(shared.entry);

          final job = await import(api, login, dup: dup);

          expect(job.createdItems, 1, reason: type);
          expect(api.updated, isNull, reason: type);
          expect(api.created!.type, 'password', reason: type);
        }
      });

      test('${dup.name}: password, totp and an empty type are one family', () async {
        for (final type in ['password', 'totp', '']) {
          final shared = await sharedEntry(const {'password': 'old-pw'}, type: type);
          final api = _FakeApi(shared.entry);

          final job = await import(api, login, dup: dup);

          expect(job.createdItems, 0, reason: type);
          expect(api.created, isNull, reason: type);
          if (dup == DupStrategy.overwrite) {
            expect(job.updatedItems, 1, reason: type);
          } else {
            expect(job.skippedItems, 1, reason: type);
          }
        }
      });

      test('${dup.name}: any other type matches itself', () async {
        final shared = await sharedEntry(const {'notes': 'old'}, type: 'note');
        final api = _FakeApi(shared.entry);

        final job = await import(api, note, format: ImportFormat.bitwarden, dup: dup);

        expect(job.createdItems, 0);
        expect(api.created, isNull);
        expect(dup == DupStrategy.overwrite ? job.updatedItems : job.skippedItems, 1);
      });
    }
  }, skip: libExists ? false : 'native crypto lib not built ($_libPath)');

  group('twins in one file', () {
    const twins = 'name,url,username,password,notes\n'
        'Twin,,alice,first,\n'
        'Twin,,alice,second,\n';

    Future<Set<Object?>> passwords(_VaultApi api) async => {
          for (final e in api.entries.values)
            (await owner.decrypt(e.encryptedData, e.entryKey!.encryptedKey))['password'],
        };

    for (final dup in DupStrategy.values) {
      test('${dup.name}: both are created', () async {
        final api = _VaultApi();

        final job = await import(api, twins, dup: dup);

        expect(job.createdItems, 2);
        expect(job.updatedItems, 0);
        expect(job.skippedItems, 0);
        expect(job.failedItems, 0);
        expect(api.updates, isEmpty);
        expect(await passwords(api), {'first', 'second'});
      });
    }

    test('a later import in the session sees both', () async {
      final api = _VaultApi();
      final container = containerFor(api);
      final runner = container.read(jobRunnerProvider.notifier);
      final bytes = Uint8List.fromList(utf8.encode(twins));

      await runner.startImport(
          bytes: bytes, format: ImportFormat.csvGeneric, dup: DupStrategy.skip);
      await runner.startImport(
          bytes: bytes, format: ImportFormat.csvGeneric, dup: DupStrategy.skip);
      expect(container.read(jobRunnerProvider).first.skippedItems, 2);
      expect(api.entries, hasLength(2));

      await runner.startImport(
          bytes: Uint8List.fromList(utf8.encode('name,url,username,password,notes\nSolo,,bob,p,\n')),
          format: ImportFormat.csvGeneric,
          dup: DupStrategy.skip);
      expect(container.read(jobRunnerProvider).first.createdItems, 1);
      await runner.startImport(
          bytes: Uint8List.fromList(utf8.encode('name,url,username,password,notes\nSolo,,bob,p2,\n')),
          format: ImportFormat.csvGeneric,
          dup: DupStrategy.overwrite);
      expect(container.read(jobRunnerProvider).first.updatedItems, 1);
      expect(api.entries, hasLength(3));
    });

    test('a twin of a vault entry still counts as its duplicate', () async {
      final api = _VaultApi();
      final dataKeyEntry = await sharedEntry(const {'username': 'alice', 'password': 'old-pw'});
      final e = dataKeyEntry.entry;
      api.entries['e1'] = EntryResponse(
        id: 'e1',
        type: 'password',
        name: 'Twin',
        url: '',
        encryptedData: e.encryptedData,
        dataNonce: e.dataNonce,
        entryKey: e.entryKey,
        permission: 'owner',
        createdAt: '',
        updatedAt: '',
      );

      final job = await import(api, twins, dup: DupStrategy.skip);

      expect(job.skippedItems, 2);
      expect(job.createdItems, 0);
    });
  }, skip: libExists ? false : 'native crypto lib not built ($_libPath)');

  group('export', () {
    const tags = ['Work', 'client a'];

    Future<LocalJob> exportData(Map<String, dynamic> data, ExportFormat format,
        {String type = 'password'}) async {
      final shared = await sharedEntry(data, type: type);
      final container = containerFor(_FakeApi(shared.entry));
      await container.read(jobRunnerProvider.notifier).startExport(format: format);
      final job = container.read(jobRunnerProvider).single;
      expect(job.state, JobState.completed);
      return job;
    }

    Future<LocalJob> export(ExportFormat format) async {
      final shared = await sharedEntry(const {
        'username': 'alice',
        'password': 'pw',
        'tags': ['#Work', 'client   a', 'WORK'],
      });
      final container = containerFor(_FakeApi(shared.entry));
      await container.read(jobRunnerProvider.notifier).startExport(format: format);
      final job = container.read(jobRunnerProvider).single;
      expect(job.state, JobState.completed);
      return job;
    }

    test('tags reach every format, normalised', () async {
      Future<String> text(ExportFormat f) async => utf8.decode((await export(f)).resultBytes!);

      expect(parseCsv(await text(ExportFormat.csv), CsvImportFormat.generic).single.tags, tags);
      expect(
          parseCsv(await text(ExportFormat.onepasswordCsv), CsvImportFormat.onePassword)
              .single
              .tags,
          tags);
      expect(parseBitwarden(await text(ExportFormat.bitwarden)).single.tags, tags);
      expect(
          parseOnePux((await export(ExportFormat.onepassword1pux)).resultBytes!)
              .records
              .single
              .tags,
          tags);
    });

    test('a stray totp_secret is not exported, a real one is', () async {
      const secret = 'JBSWY3DPEHPK3PXP';
      const stray = {'username': 'alice', 'password': 'same', 'totp_secret': 'same'};
      const real = {'username': 'alice', 'password': 'pw', 'totp_secret': secret};

      Future<EntryRecordView> bitwarden(Map<String, dynamic> data) async {
        final job = await exportData(data, ExportFormat.bitwarden);
        final json = utf8.decode(job.resultBytes!);
        final login = ((jsonDecode(json) as Map)['items'] as List).single['login'] as Map;
        expect(login['password'], data['password']);
        return (hasTotpKey: login.containsKey('totp'), secret: parseBitwarden(json).single.totpSecret);
      }

      expect(await bitwarden(stray), (hasTotpKey: false, secret: ''));
      expect(await bitwarden(real), (hasTotpKey: true, secret: secret));

      Future<String> onePux(Map<String, dynamic> data) async =>
          parseOnePux((await exportData(data, ExportFormat.onepassword1pux)).resultBytes!)
              .records
              .single
              .totpSecret;

      expect(await onePux(stray), '');
      expect(await onePux(real), secret);
    });

    String leftOutLine(String count) => 'CSV has no column for $count (TOTP secrets, card '
        'and bank data, custom fields and the like), so they are not in the file. The '
        'Bitwarden JSON export keeps them.';

    const bank = {
      'username': 'alice',
      'password': 'pw',
      'totp_secret': 'JBSWY3DPEHPK3PXP',
      'name': 'stale copy',
      'url': 'stale copy',
      'bank_name': 'Erste',
      'iban': 'AT00 1234',
      'bic': 'GIBAATWW',
      'account_number': '1234',
      'account_type': 'savings',
      'issuer': 'ACME',
      'period': 60,
      'somebody_elses': 'who knows',
      'tags': ['Work'],
      'custom_fields': [
        {'label': 'PIN', 'value': '4711', 'type': 'password'},
        {'label': 'From the future', 'value': 'x', 'type': 'passkey'},
      ],
    };

    test('bitwarden: bank-account fields and unknown keys reach the file as custom fields',
        () async {
      final job = await exportData(bank, ExportFormat.bitwarden, type: 'bank-account');
      final item = ((jsonDecode(utf8.decode(job.resultBytes!)) as Map)['items'] as List).single
          as Map<String, dynamic>;

      expect(item['type'], 1);
      expect(item['login'], {
        'username': 'alice',
        'password': 'pw',
        'totp': 'JBSWY3DPEHPK3PXP',
        'uris': [
          {'uri': 'https://mail.example.com'},
        ],
      });
      expect(item['fields'], [
        {'name': 'PIN', 'value': '4711', 'type': 1},
        {'name': 'From the future', 'value': 'x', 'type': 1},
        {'name': 'issuer', 'value': 'ACME', 'type': 0},
        {'name': 'period', 'value': '60', 'type': 0},
        {'name': 'bank_name', 'value': 'Erste', 'type': 0},
        {'name': 'iban', 'value': 'AT00 1234', 'type': 1},
        {'name': 'bic', 'value': 'GIBAATWW', 'type': 0},
        {'name': 'account_number', 'value': '1234', 'type': 1},
        {'name': 'account_type', 'value': 'savings', 'type': 0},
        {'name': 'type', 'value': 'bank-account', 'type': 0},
        {'name': 'somebody_elses', 'value': 'who knows', 'type': 1},
        {'name': 'passbubble_tags', 'value': 'Work', 'type': 0},
      ]);
    });

    test('bitwarden: an identity export is an identity item with its URL', () async {
      final job = await exportData(const {
        'first_name': 'Ada',
        'last_name': 'Lovelace',
        'title': 'Dr',
        'totp_secret': 'JBSWY3DPEHPK3PXP',
      }, ExportFormat.bitwarden, type: 'identity');
      final item = ((jsonDecode(utf8.decode(job.resultBytes!)) as Map)['items'] as List).single
          as Map<String, dynamic>;

      expect(item['type'], 4);
      expect(item['identity']['firstName'], 'Ada');
      expect(item['identity']['title'], 'Dr');
      expect(item['fields'], [
        {'name': 'url', 'value': 'https://mail.example.com', 'type': 0},
        {'name': 'totp_secret', 'value': 'JBSWY3DPEHPK3PXP', 'type': 1},
      ]);
    });

    for (final format in [ExportFormat.csv, ExportFormat.onepasswordCsv]) {
      test('${format.name}: the log says what stayed behind, and the file has no extra columns',
          () async {
        final job = await exportData(bank, format, type: 'bank-account');

        // totp_secret, the two custom fields, and bank_name, iban, bic,
        // account_number, account_type, issuer, period, somebody_elses.
        expect(job.log, contains(leftOutLine('11 fields of 1 entry')));
        final text = utf8.decode(job.resultBytes!);
        expect(const LineSplitter().convert(text).first.split(',').length,
            format == ExportFormat.csv ? 6 : 7);
        expect(text, isNot(contains('AT00')));
        expect(text, isNot(contains('4711')));
      });

      test('${format.name}: counts across entries, and only those with something left out',
          () async {
        final shared = await sharedEntry(const {'password': 'pw', 'iban': 'AT00'});
        final e = shared.entry;
        final container = containerFor(_FakeApi(e)
          ..twin = EntryResponse(
            id: 'e2',
            type: e.type,
            name: 'Second',
            url: '',
            encryptedData: e.encryptedData,
            dataNonce: e.dataNonce,
            entryKey: e.entryKey,
            permission: 'owner',
            createdAt: '',
            updatedAt: '',
          ));
        await container.read(jobRunnerProvider.notifier).startExport(format: format);
        expect(container.read(jobRunnerProvider).single.log,
            contains(leftOutLine('2 fields of 2 entries')));
      });

      test('${format.name}: nothing left out, no line', () async {
        final job = await exportData(const {
          'username': 'alice',
          'password': 'pw',
          'notes': 'n',
          'tags': ['Work'],
        }, format);
        expect(job.log.where((l) => l.startsWith('CSV has no column')), isEmpty);
      });
    }

    test('1 field of 1 entry reads right', () async {
      final job = await exportData(const {'password': 'pw', 'iban': 'AT00'}, ExportFormat.csv);
      expect(job.log, contains(leftOutLine('1 field of 1 entry')));
    });

    test('other formats have no such line', () async {
      for (final format in [ExportFormat.bitwarden, ExportFormat.onepassword1pux]) {
        final job = await exportData(bank, format, type: 'bank-account');
        expect(job.log.where((l) => l.startsWith('CSV has no column')), isEmpty,
            reason: format.name);
      }
    });
  }, skip: libExists ? false : 'native crypto lib not built ($_libPath)');
}

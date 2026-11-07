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
import 'dart:typed_data';
import 'dart:ui' show Rect;

import 'package:cryptography/cryptography.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:share_plus/share_plus.dart';
import 'package:uuid/uuid.dart';

import '../api/api_client.dart';
import '../api/models.dart'
    show
        CreateEntryRequest,
        CreateFolderRequest,
        CreateJobRequest,
        EntryKey,
        EntryResponse,
        FolderResponse,
        UpdateEntryRequest,
        UpdateJobRequest;
import '../auth/auth_service.dart';
import '../crypto/entry_update.dart';
import '../crypto/vault_crypto.dart';
import '../importexport/bitwarden_format.dart';
import '../importexport/csv_format.dart';
import '../importexport/entry_record.dart';
import '../importexport/onepassword_format.dart';
import '../importexport/psono_format.dart';
import '../util/entry_tags.dart';
import '../util/entry_totp.dart';
import '../../features/entries/entries_list_screen.dart'
    show entriesProvider, foldersProvider;
import 'job_messenger.dart';
import 'local_job.dart';

/// App-root-scoped runner that owns all in-session import/export jobs.
///
/// Because it lives in the provider container (not a widget), the work keeps
/// running when the user leaves the Manage screen — fixing the bug where the
/// old in-widget loop aborted on `!mounted`, leaving a half-imported vault and a
/// ledger job stuck in `running`. Watching this provider gives the Job View live
/// progress without polling.
final jobRunnerProvider =
    NotifierProvider<JobRunner, List<LocalJob>>(JobRunner.new);

/// Number of jobs currently running — drives the tab/nav badges.
final activeJobCountProvider =
    Provider<int>((ref) => ref.watch(jobRunnerProvider).where((j) => j.isRunning).length);

/// A vault entry as duplicate detection sees it.
typedef _ExistingEntry = ({String id, String name, String username, String type});

/// An update cannot change an entry's type, so only entries of one family can be
/// duplicates: the login types are one, every other type stands alone.
String _typeFamily(String type) => switch (type) {
      '' || 'password' || 'totp' => 'login',
      _ => type,
    };

class JobRunner extends Notifier<List<LocalJob>> {
  static const _uuid = Uuid();

  @override
  List<LocalJob> build() => const [];

  // ── State helpers ──────────────────────────────────────────────────────────

  void _add(LocalJob job) => state = [job, ...state];

  void _update(String id, LocalJob Function(LocalJob) f) {
    state = [for (final j in state) j.id == id ? f(j) : j];
  }

  void _log(String id, String msg) {
    _update(id, (j) => j.copyWith(log: [...j.log, msg]));
  }

  LocalJob? _byId(String id) {
    for (final j in state) {
      if (j.id == id) return j;
    }
    return null;
  }

  // ── Import ───────────────────────────────────────────────────────────────

  /// Starts an import. Returns immediately after kicking off the async work; the
  /// job continues regardless of which screen is mounted.
  Future<void> startImport({
    required Uint8List bytes,
    required ImportFormat format,
    required DupStrategy dup,
    OverwriteMode overwriteMode = OverwriteMode.merge,
  }) async {
    final localId = _uuid.v4();
    _add(LocalJob(
      id: localId,
      type: 'import',
      format: format.apiFormat,
      state: JobState.running,
      createdAt: DateTime.now(),
      statusText: 'Parsing file…',
    ));

    String jobId = localId; // replaced with the server id once the ledger POSTs
    try {
      final parsed = _parse(bytes, format);
      final records = dropStrayTotpSecrets(parsed.records);
      _update(localId, (j) => j.copyWith(warnings: parsed.warnings));
      for (final w in parsed.warnings) {
        _log(localId, 'warning: $w');
      }

      if (records.isEmpty) {
        _finish(localId, JobState.failed,
            statusText: 'No records found in file.',
            errorMessage: 'No records found in the selected file.');
        return;
      }

      _update(localId, (j) => j.copyWith(
            totalItems: records.length,
            statusText: 'Found ${records.length} entries — importing…',
          ));
      _log(localId, 'Parsed ${records.length} record(s).');
      if (dup == DupStrategy.overwrite) {
        _log(localId, 'Overwrite mode: ${overwriteMode.name}');
      }
      final startBanner = _byId(localId);
      if (startBanner != null) showJobStartedBanner(ref, startBanner);

      final api = ref.read(apiClientProvider);
      final auth = ref.read(authServiceProvider);

      // Seed the folder resolver cache from the existing tree.
      _log(localId, 'Loading folders…');
      _folderIdCache.clear();
      _buildFolderCache(await api.listFolders(), '');

      // Decrypt existing entries for duplicate detection. Matching runs on this
      // copy: records in one file must not match each other, so what this import
      // creates goes into the cache alone.
      _log(localId, 'Loading existing vault…');
      _existingCache = [];
      final existing = List.of(await _loadExisting(api, auth));

      // Record the import in the server ledger (best-effort — keep the local
      // job under [localId] if the POST fails so it still shows and finishes).
      try {
        final job = await api.createJob(CreateJobRequest(
          type: 'import',
          format: format.apiFormat,
          dupStrategy: dup.name,
          totalItems: records.length,
          clientName: 'Flutter',
        ));
        jobId = job.id;
      } catch (_) {
        _log(localId, 'Ledger unavailable — tracking locally.');
      }

      int created = 0, updated = 0, skipped = 0, failed = 0, i = 0;
      for (final rec in records) {
        i++;
        _update(localId, (j) => j.copyWith(
              processedItems: i,
              statusText: '$i/${records.length}: ${rec.name}',
            ));

        final dupIds = _findDuplicates(rec, existing);
        if (dupIds.isNotEmpty) {
          if (dup == DupStrategy.overwrite && dupIds.length > 1) {
            // Which one the file means is anyone's guess, so none gets overwritten.
            failed++;
            _log(localId, 'failed (update): ${rec.name} — ${dupIds.length} entries match, none was changed');
          } else if (dup == DupStrategy.overwrite) {
            try {
              await _updateEntry(api, auth, dupIds.single, rec, overwriteMode);
              updated++;
              _log(localId, 'updated: ${rec.name}');
            } catch (e) {
              failed++;
              _log(localId, 'failed (update): ${rec.name} — $e');
            }
          } else {
            skipped++;
            _log(localId, 'skipped (duplicate): ${rec.name}');
          }
        } else {
          try {
            final folderId = await _resolveFolder(api, rec.folderPath);
            final entryId = await _createEntry(api, auth, rec, folderId: folderId);
            _existingCache.add(
                (id: entryId, name: rec.name, username: rec.username, type: rec.type));
            created++;
            _log(localId, 'created: ${rec.name}');
          } catch (e) {
            failed++;
            _log(localId, 'failed (create): ${rec.name} — $e');
          }
        }

        _update(localId, (j) => j.copyWith(
              createdItems: created,
              updatedItems: updated,
              skippedItems: skipped,
              failedItems: failed,
            ));
      }

      final doneMsg = failed > 0
          ? 'Done with errors: $created created, $updated updated, $skipped skipped, $failed failed'
          : 'Done: $created created, $updated updated, $skipped skipped';

      final finalState =
          (failed > 0 && created + updated == 0) ? JobState.failed : JobState.completed;

      // Finalize the ledger entry (best-effort).
      if (jobId != localId) {
        try {
          await api.updateJob(
            jobId,
            UpdateJobRequest(
              status: finalState == JobState.failed ? 'failed' : 'completed',
              processedItems: records.length,
              createdItems: created,
              updatedItems: updated,
              skippedItems: skipped,
              failedItems: failed,
            ),
          );
        } catch (_) {
          // Ledger update failed — non-fatal.
        }
      }

      // Refresh the vault so new entries appear immediately.
      ref.invalidate(entriesProvider);
      ref.invalidate(foldersProvider);

      _finish(localId, finalState, statusText: doneMsg);
    } catch (e) {
      _finish(localId, JobState.failed,
          statusText: 'Import failed: $e', errorMessage: '$e');
    }
  }

  // ── Export ───────────────────────────────────────────────────────────────

  Future<void> startExport({
    required ExportFormat format,
    ExportOptions options = const ExportOptions(),
  }) async {
    final localId = _uuid.v4();
    _add(LocalJob(
      id: localId,
      type: 'export',
      format: format.apiFormat,
      state: JobState.running,
      createdAt: DateTime.now(),
      statusText: 'Loading vault…',
    ));

    String jobId = localId;
    try {
      final api = ref.read(apiClientProvider);
      final auth = ref.read(authServiceProvider);

      _log(localId, 'Loading vault…');
      final entries = await api.listEntries();
      _update(localId, (j) => j.copyWith(totalItems: entries.length));
      final startBanner = _byId(localId);
      if (startBanner != null) showJobStartedBanner(ref, startBanner);

      // Record the export in the server ledger (best-effort).
      try {
        final job = await api.createJob(CreateJobRequest(
          type: 'export',
          format: format.apiFormat,
          dupStrategy: DupStrategy.skip.name,
          totalItems: entries.length,
          clientName: 'Flutter',
        ));
        jobId = job.id;
      } catch (_) {
        _log(localId, 'Ledger unavailable — tracking locally.');
      }

      _update(localId, (j) => j.copyWith(statusText: 'Decrypting entries…'));
      final records = <EntryRecord>[];
      int i = 0, failed = 0;
      for (final e in entries) {
        i++;
        _update(localId, (j) => j.copyWith(
              processedItems: i,
              statusText: '$i/${entries.length}: ${e.name}',
            ));
        try {
          final rec = await _entryToRecord(api, auth, e);
          if (rec != null) records.add(rec);
        } catch (err) {
          failed++;
          _log(localId, 'failed (decrypt): ${e.name} — $err');
          _update(localId, (j) => j.copyWith(failedItems: failed));
        }
      }

      _update(localId, (j) => j.copyWith(statusText: 'Generating file…'));
      final fileBytes = _generate(format, records, options);
      if (format == ExportFormat.csv || format == ExportFormat.onepasswordCsv) {
        _logCsvLeftOut(localId, records);
      }

      // Finalize ledger.
      if (jobId != localId) {
        try {
          await api.updateJob(
            jobId,
            UpdateJobRequest(
              status: 'completed',
              processedItems: entries.length,
              createdItems: records.length,
              failedItems: failed,
            ),
          );
        } catch (_) {}
      }

      _update(
          localId,
          (j) => j.copyWith(
                createdItems: records.length,
                resultBytes: fileBytes,
                resultFilename: format.filename,
                resultMime: format.mimeType,
              ));
      _finish(localId, JobState.completed,
          statusText: 'Exported ${records.length} entries');

      // Hand the file straight to the share sheet (same UX as before). Isolated
      // so a share failure can't flip the already-completed job to failed; the
      // result SnackBar / detail sheet still offer SHARE to retry.
      try {
        await Share.shareXFiles(
          [XFile.fromData(fileBytes, name: format.filename, mimeType: format.mimeType)],
          subject: 'Passbubble Export',
          // iOS needs a non-zero source rect; no widget context in the job runner.
          sharePositionOrigin: const Rect.fromLTWH(0, 0, 1, 1),
        );
      } catch (_) {}
    } catch (e) {
      _finish(localId, JobState.failed,
          statusText: 'Export failed: $e', errorMessage: '$e');
    }
  }

  /// A CSV has no place for most of an entry, and the job log is where the user
  /// learns how much stayed behind.
  void _logCsvLeftOut(String id, List<EntryRecord> records) {
    final counts = records.map(csvLeftOutFields).where((n) => n > 0).toList();
    if (counts.isEmpty) return;
    final fields = counts.reduce((a, b) => a + b);
    final entries = counts.length;
    _log(
        id,
        'CSV has no column for $fields ${fields == 1 ? 'field' : 'fields'} of '
        '$entries ${entries == 1 ? 'entry' : 'entries'} (TOTP secrets, card and bank '
        'data, custom fields and the like), so they are not in the file. The '
        'Bitwarden JSON export keeps them.');
  }

  // ── Finalization + notification ────────────────────────────────────────────

  void _finish(String id, JobState finalState,
      {required String statusText, String? errorMessage}) {
    _update(
        id,
        (j) => j.copyWith(
              state: finalState,
              statusText: statusText,
              errorMessage: errorMessage,
            ));
    final job = _byId(id);
    if (job != null) showJobDoneSnack(ref, job);
  }

  // ── Import/export internals (moved from the tab widgets) ─────────────────────

  ({List<EntryRecord> records, List<String> warnings}) _parse(
      Uint8List bytes, ImportFormat format) {
    switch (format) {
      case ImportFormat.csvGeneric:
        return (records: parseCsv(utf8.decode(bytes), CsvImportFormat.generic), warnings: const []);
      case ImportFormat.csvChrome:
        return (records: parseCsv(utf8.decode(bytes), CsvImportFormat.chrome), warnings: const []);
      case ImportFormat.csvLastPass:
        return (records: parseCsv(utf8.decode(bytes), CsvImportFormat.lastpass), warnings: const []);
      case ImportFormat.csv1Password:
        return (records: parseCsv(utf8.decode(bytes), CsvImportFormat.onePassword), warnings: const []);
      case ImportFormat.bitwarden:
        return (records: parseBitwarden(utf8.decode(bytes)), warnings: const []);
      case ImportFormat.psono:
        final r = parsePsono(utf8.decode(bytes));
        return (records: r.records, warnings: r.warnings);
      case ImportFormat.onepassword1pux:
        final r = parseOnePux(Uint8List.fromList(bytes));
        return (records: r.records, warnings: r.warnings);
    }
  }

  Uint8List _generate(
      ExportFormat format, List<EntryRecord> records, ExportOptions options) {
    switch (format) {
      case ExportFormat.csv:
        return Uint8List.fromList(utf8.encode(exportCsv(records)));
      case ExportFormat.bitwarden:
        final opts = BitwardenExportOptions(
          includeFiles: options.includeFiles,
          filesAsBase64: options.filesAsBase64,
        );
        return Uint8List.fromList(utf8.encode(exportBitwarden(records, opts)));
      case ExportFormat.onepasswordCsv:
        return Uint8List.fromList(utf8.encode(export1PasswordCsv(records)));
      case ExportFormat.onepassword1pux:
        return exportOnePux(records);
    }
  }

  // In-process dedupe list; rebuilt per import.
  List<_ExistingEntry> _existingCache = [];

  // Folder path → id cache built at import start from the existing tree.
  final Map<String, String> _folderIdCache = {};

  void _buildFolderCache(List<FolderResponse> folders, String prefix) {
    for (final f in folders) {
      final key = prefix.isEmpty ? f.name : '$prefix/${f.name}';
      _folderIdCache[key] = f.id;
      if (f.children.isNotEmpty) _buildFolderCache(f.children, key);
    }
  }

  Future<String?> _resolveFolder(ApiClient api, List<String> path) async {
    if (path.isEmpty) return null;
    String? parentId;
    for (int i = 0; i < path.length; i++) {
      final cacheKey = path.sublist(0, i + 1).join('/');
      if (_folderIdCache.containsKey(cacheKey)) {
        parentId = _folderIdCache[cacheKey];
      } else {
        try {
          final newId = await api
              .createFolder(CreateFolderRequest(name: path[i], parentId: parentId));
          _folderIdCache[cacheKey] = newId;
          parentId = newId;
        } catch (_) {
          return parentId; // best-effort: use closest ancestor
        }
      }
    }
    return parentId;
  }

  Future<List<_ExistingEntry>> _loadExisting(
    ApiClient api,
    AuthService auth,
  ) async {
    if (_existingCache.isNotEmpty) return _existingCache;
    final entries = await api.listEntries();
    final result = <_ExistingEntry>[];
    for (final e in entries) {
      try {
        final full = await api.getEntry(e.id);
        if (full.entryKey == null) continue;
        final dataKey = await VaultCrypto.decryptDataKey(
            full.entryKey!.encryptedKey, auth.privX25519!, auth.privMLKEM!);
        final ciphertext = base64.decode(full.encryptedData);
        final plaintext = await VaultCrypto.decrypt(SecretKey(dataKey), ciphertext);
        final data = jsonDecode(utf8.decode(plaintext)) as Map<String, dynamic>;
        result.add((
          id: e.id,
          name: e.name,
          username: data['username'] as String? ?? '',
          type: e.type,
        ));
      } catch (_) {}
    }
    _existingCache = result;
    return result;
  }

  List<String> _findDuplicates(
    EntryRecord rec,
    List<_ExistingEntry> existing,
  ) {
    String norm(String s) => s.trim().toLowerCase();
    final family = _typeFamily(rec.type);
    return [
      for (final e in existing)
        if (norm(e.name) == norm(rec.name) &&
            norm(e.username) == norm(rec.username) &&
            _typeFamily(e.type) == family)
          e.id,
    ];
  }

  /// The decrypted entry JSON for an imported record: what the file holds, with
  /// empty values left out.
  Map<String, dynamic> _recordToPayload(EntryRecord rec) {
    final m = <String, dynamic>{...rec.dataValues};
    if (rec.customFields.isNotEmpty) {
      m['custom_fields'] = rec.customFields.map((cf) => cf.toJson()).toList();
    }
    putTags(m, rec.tags);
    return m;
  }

  Future<String> _createEntry(ApiClient api, AuthService auth, EntryRecord rec,
      {String? folderId}) async {
    final dataKey = VaultCrypto.randomKey();
    final plaintext = utf8.encode(jsonEncode(_recordToPayload(rec)));
    final ciphertext =
        await VaultCrypto.encrypt(SecretKey(dataKey), Uint8List.fromList(plaintext));
    final pubKey = await auth.getPubX25519();
    final pubMlkem = await auth.getPubMlkem768();
    if (pubKey == null || pubMlkem == null) throw Exception('No public key');
    final encDataKey = await VaultCrypto.encryptDataKey(dataKey, pubKey, pubMlkem);
    final userId = await auth.getUserId();

    return api.createEntry(CreateEntryRequest(
      folderId: folderId,
      type: rec.type.isEmpty ? 'password' : rec.type,
      name: rec.name,
      url: rec.url.isNotEmpty ? rec.url : null,
      encryptedData: base64.encode(ciphertext),
      dataNonce: base64.encode(Uint8List(12)),
      entryKeys: [EntryKey(userId: userId!, encryptedKey: encDataKey)],
      createdAt: rec.createdAt,
      updatedAt: rec.updatedAt,
    ));
  }

  Future<void> _updateEntry(ApiClient api, AuthService auth, String id,
      EntryRecord rec, OverwriteMode mode) async {
    final full = await api.getEntry(id);
    // Same data key, so everyone the entry is shared with can still decrypt.
    final opened = await openEntryForUpdate(
      full,
      privX25519: auth.privX25519,
      privMlkem: auth.privMLKEM,
    );
    final payload = _recordToPayload(rec);
    final complete = mode == OverwriteMode.complete;
    final enc = await VaultCrypto.encryptEntryDataWithKey(
        complete ? payload : mergeImportedData(opened.data, payload), opened.dataKey);
    await api.updateEntry(
        id,
        UpdateEntryRequest(
          // The server overwrites folder_id on every update, so without this
          // the entry would move to the root.
          folderId: full.folderId,
          name: rec.name,
          // An empty url clears the entry's, a missing one keeps it.
          url: complete || rec.url.isNotEmpty ? rec.url : null,
          encryptedData: enc.encryptedData,
          dataNonce: enc.dataNonce,
        ));
  }

  Future<EntryRecord?> _entryToRecord(
      ApiClient api, AuthService auth, EntryResponse e) async {
    final full = await api.getEntry(e.id);
    if (full.entryKey == null) return null;
    final dataKey = await VaultCrypto.decryptDataKey(
        full.entryKey!.encryptedKey, auth.privX25519!, auth.privMLKEM!);
    final ciphertext = base64.decode(full.encryptedData);
    final plaintext = await VaultCrypto.decrypt(SecretKey(dataKey), ciphertext);
    final data = jsonDecode(utf8.decode(plaintext)) as Map<String, dynamic>;

    String s(String k) => data[k] as String? ?? '';

    final customFields = <CustomFieldRecord>[];
    final rawCf = data['custom_fields'];
    if (rawCf is List) {
      for (final cf in rawCf) {
        if (cf is Map<String, dynamic>) {
          customFields.add(CustomFieldRecord.fromJson(cf));
        }
      }
    }

    return EntryRecord(
      name: e.name,
      url: e.url,
      type: e.type,
      username: s('username'),
      password: s('password'),
      totpSecret: totpSecretOf(data) ?? '',
      notes: s('notes'),
      cardNumber: s('card_number'),
      holderName: s('holder_name'),
      expiryMonth: s('expiry_month'),
      expiryYear: s('expiry_year'),
      cvv: s('cvv'),
      title: s('title'),
      firstName: s('first_name'),
      lastName: s('last_name'),
      company: s('company'),
      email: s('email'),
      phone: s('phone'),
      street: s('street'),
      city: s('city'),
      state: s('state'),
      postalCode: s('postal_code'),
      country: s('country'),
      licenseKey: s('license_key'),
      productName: s('product_name'),
      customFields: customFields,
      extraData: extraEntryData(data),
      tags: tagsOf(data),
    );
  }
}

/// The data an "overwrite" import in [OverwriteMode.merge] saves over an existing
/// entry. [payload] holds only non-empty values, and each of them replaces the
/// existing one; anything the file does not hold stays, which covers
/// `totp_secret`, other clients' keys, and the tags unless the record has some.
/// Custom fields merge by label, see [_mergeCustomFields].
///
/// A stray `totp_secret` (see [isStrayTotpSecret]) goes: next to a new password
/// it would no longer be recognised and turn into a 2FA secret.
///
/// The stored TOTP options go when a different secret comes in without any, as
/// they belong to the old one. They stay as long as a custom `totp` field with
/// another key is left, which reads the same options ([TotpOptions.stored]).
Map<String, dynamic> mergeImportedData(
  Map<String, dynamic> existing,
  Map<String, dynamic> payload,
) {
  final merged = {...existing, ...payload};
  if (isStrayTotpSecret(existing) && !payload.containsKey(totpSecretKey)) {
    merged.remove(totpSecretKey);
  }
  final imported = payload['custom_fields'];
  if (imported is List) {
    merged['custom_fields'] = _mergeCustomFields(existing['custom_fields'], imported);
  }
  if (_bringsNewTotpKey(existing, payload) && !_hasOtherCustomTotpKey(merged)) {
    for (final key in totpOptionKeys) {
      if (!payload.containsKey(key)) merged.remove(key);
    }
  }
  return merged;
}

bool _bringsNewTotpKey(Map<String, dynamic> existing, Map<String, dynamic> payload) {
  final incoming = payload[totpSecretKey];
  if (incoming is! String || incoming.isEmpty) return false;
  final old = existing[totpSecretKey];
  return old is! String || !isSameTotpKey(old, incoming);
}

bool _hasOtherCustomTotpKey(Map<String, dynamic> data) {
  final fields = data['custom_fields'];
  final secret = data[totpSecretKey] as String;
  if (fields is! List) return false;
  return fields.any((f) {
    if (f is! Map || f['type'] != CustomFieldType.totp.apiValue) return false;
    final value = f['value'];
    return value is String && value.isNotEmpty && !isSameTotpKey(value, secret);
  });
}

/// The existing custom fields with the [imported] ones merged in: a field with
/// the same label gets the imported value and type and keeps its place and its
/// other keys, a new label is appended, and every other field stays, including
/// file fields. An imported field with an empty value changes nothing, like any
/// other empty value in the file. Each existing field is matched once, so a file
/// with a label twice loses neither.
List<dynamic> _mergeCustomFields(Object? existing, List<dynamic> imported) {
  final merged = existing is List ? List<dynamic>.of(existing) : <dynamic>[];
  final matched = <int>{};
  for (final field in imported.whereType<Map<String, dynamic>>()) {
    final slot = _indexOfLabel(merged, field['label'], skip: matched);
    if (slot < 0) {
      merged.add(field);
      matched.add(merged.length - 1);
      continue;
    }
    matched.add(slot);
    if ((field['value'] as String? ?? '').isEmpty) continue;
    final old = (merged[slot] as Map).cast<String, dynamic>();
    merged[slot] = {...old}
      // Belong to the old value; a file field in [field] brings its own.
      ..remove('filename')
      ..remove('mime_type')
      ..addAll(field);
  }
  return merged;
}

int _indexOfLabel(List<dynamic> fields, Object? label, {required Set<int> skip}) {
  for (var i = 0; i < fields.length; i++) {
    final f = fields[i];
    if (!skip.contains(i) && f is Map && f['label'] == label) return i;
  }
  return -1;
}

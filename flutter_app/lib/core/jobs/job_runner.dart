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
import '../crypto/vault_crypto.dart';
import '../importexport/bitwarden_format.dart';
import '../importexport/csv_format.dart';
import '../importexport/entry_record.dart';
import '../importexport/onepassword_format.dart';
import '../importexport/psono_format.dart';
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
      final records = parsed.records;
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
      final startBanner = _byId(localId);
      if (startBanner != null) showJobStartedBanner(ref, startBanner);

      final api = ref.read(apiClientProvider);
      final auth = ref.read(authServiceProvider);

      // Seed the folder resolver cache from the existing tree.
      _log(localId, 'Loading folders…');
      _folderIdCache.clear();
      _buildFolderCache(await api.listFolders(), '');

      // Decrypt existing entries for duplicate detection.
      _log(localId, 'Loading existing vault…');
      _existingCache = [];
      final existing = await _loadExisting(api, auth);

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

        final dupId = _findDuplicate(rec, existing);
        if (dupId != null) {
          if (dup == DupStrategy.overwrite) {
            try {
              await _updateEntry(api, auth, dupId, rec);
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
            existing.add((id: entryId, name: rec.name, username: rec.username));
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
        );
      } catch (_) {}
    } catch (e) {
      _finish(localId, JobState.failed,
          statusText: 'Export failed: $e', errorMessage: '$e');
    }
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

  // In-process dedupe list of (id, name, username); rebuilt per import.
  List<({String id, String name, String username})> _existingCache = [];

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

  Future<List<({String id, String name, String username})>> _loadExisting(
    ApiClient api,
    AuthService auth,
  ) async {
    if (_existingCache.isNotEmpty) return _existingCache;
    final entries = await api.listEntries();
    final result = <({String id, String name, String username})>[];
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
        ));
      } catch (_) {}
    }
    _existingCache = result;
    return result;
  }

  String? _findDuplicate(
    EntryRecord rec,
    List<({String id, String name, String username})> existing,
  ) {
    String norm(String s) => s.trim().toLowerCase();
    for (final e in existing) {
      if (norm(e.name) == norm(rec.name) && norm(e.username) == norm(rec.username)) {
        return e.id;
      }
    }
    return null;
  }

  Map<String, dynamic> _recordToPayload(EntryRecord rec) {
    final m = <String, dynamic>{};
    void set(String k, String v) {
      if (v.isNotEmpty) m[k] = v;
    }

    set('username', rec.username);
    set('password', rec.password);
    set('totp_secret', rec.totpSecret);
    set('notes', rec.notes);
    set('card_number', rec.cardNumber);
    set('holder_name', rec.holderName);
    set('expiry_month', rec.expiryMonth);
    set('expiry_year', rec.expiryYear);
    set('cvv', rec.cvv);
    set('first_name', rec.firstName);
    set('last_name', rec.lastName);
    set('company', rec.company);
    set('email', rec.email);
    set('phone', rec.phone);
    set('street', rec.street);
    set('city', rec.city);
    set('state', rec.state);
    set('postal_code', rec.postalCode);
    set('country', rec.country);
    set('license_key', rec.licenseKey);
    set('product_name', rec.productName);
    if (rec.customFields.isNotEmpty) {
      m['custom_fields'] = rec.customFields.map((cf) => cf.toJson()).toList();
    }
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

  Future<void> _updateEntry(
      ApiClient api, AuthService auth, String id, EntryRecord rec) async {
    final full = await api.getEntry(id);
    if (full.entryKey == null) return;
    final dataKey = await VaultCrypto.decryptDataKey(
        full.entryKey!.encryptedKey, auth.privX25519!, auth.privMLKEM!);
    final plaintext = utf8.encode(jsonEncode(_recordToPayload(rec)));
    final ciphertext =
        await VaultCrypto.encrypt(SecretKey(dataKey), Uint8List.fromList(plaintext));
    await api.updateEntry(
        id,
        UpdateEntryRequest(
          name: rec.name,
          url: rec.url.isNotEmpty ? rec.url : null,
          encryptedData: base64.encode(ciphertext),
          dataNonce: base64.encode(Uint8List(12)),
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
      totpSecret: s('totp_secret'),
      notes: s('notes'),
      cardNumber: s('card_number'),
      holderName: s('holder_name'),
      expiryMonth: s('expiry_month'),
      expiryYear: s('expiry_year'),
      cvv: s('cvv'),
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
    );
  }
}

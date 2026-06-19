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
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_client.dart';
import '../../core/api/models.dart' show CreateEntryRequest, CreateFolderRequest, EntryKey, FolderResponse, UpdateEntryRequest;
import '../../core/auth/auth_service.dart';
import '../../core/crypto/vault_crypto.dart';
import '../../core/importexport/bitwarden_format.dart';
import '../../core/importexport/csv_format.dart';
import '../../core/importexport/entry_record.dart';
import '../../core/importexport/onepassword_format.dart';
import '../../core/importexport/psono_format.dart';
import '../../core/theme/app_theme.dart';
import '../entries/entries_list_screen.dart' show entriesProvider, foldersProvider;
import '../../shared/widgets/pb_button.dart';

enum _ImportFormat {
  csvGeneric,
  csvChrome,
  csvLastPass,
  csv1Password,
  bitwarden,
  psono,
  onepassword1pux,
}

enum _DupStrategy { skip, overwrite }

extension on _ImportFormat {
  String get label => switch (this) {
        _ImportFormat.csvGeneric => 'CSV (Generic)',
        _ImportFormat.csvChrome => 'CSV (Chrome/Edge)',
        _ImportFormat.csvLastPass => 'CSV (LastPass)',
        _ImportFormat.csv1Password => 'CSV (1Password)',
        _ImportFormat.bitwarden => 'Bitwarden JSON',
        _ImportFormat.psono => 'Psono JSON',
        _ImportFormat.onepassword1pux => '1Password (1PUX)',
      };
}

class ImportTab extends ConsumerStatefulWidget {
  const ImportTab({super.key});

  @override
  ConsumerState<ImportTab> createState() => _ImportTabState();
}

class _ImportTabState extends ConsumerState<ImportTab> {
  _ImportFormat _format = _ImportFormat.csvGeneric;
  _DupStrategy _dupStrategy = _DupStrategy.skip;
  bool _running = false;
  double _progress = 0;
  String _statusText = '';
  int _created = 0, _updated = 0, _skipped = 0, _failed = 0;
  List<String> _warnings = [];

  void _showSnackBar(String message, {bool isError = false}) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(message),
        backgroundColor: isError ? AppTheme.error : null,
        duration: isError
            ? const Duration(seconds: 6)
            : const Duration(seconds: 4),
      ),
    );
  }

  Future<void> _pickAndImport() async {
    final result = await FilePicker.platform.pickFiles(withData: true);
    if (result == null || result.files.isEmpty) return;

    final file = result.files.first;
    final bytes = file.bytes;
    if (bytes == null) {
      _showSnackBar('Could not read file data. Please try again.', isError: true);
      return;
    }

    setState(() {
      _running = true;
      _progress = 0;
      _statusText = 'Parsing file…';
      _created = _updated = _skipped = _failed = 0;
      _warnings = [];
    });

    try {
      final parseResult = await _parse(bytes);
      final records = parseResult.records;
      final parseWarnings = parseResult.warnings;

      if (!mounted) return;

      if (records.isEmpty) {
        setState(() {
          _statusText = 'No records found in file.';
          _running = false;
          _warnings = parseWarnings;
        });
        _showSnackBar('No records found in the selected file.', isError: true);
        return;
      }

      setState(() {
        _warnings = parseWarnings;
        _statusText = 'Found ${records.length} entries — creating import job…';
      });

      _showSnackBar('Found ${records.length} entries — importing…');

      final api = ref.read(apiClientProvider);
      final auth = ref.read(authServiceProvider);

      if (!mounted) return;

      // Load existing folders to seed the resolver cache.
      setState(() => _statusText = 'Loading folders…');
      _folderIdCache.clear();
      _buildFolderCache(await api.listFolders(), '');

      if (!mounted) return;

      // Fetch+decrypt all existing entries for duplicate detection
      setState(() => _statusText = 'Loading existing vault…');
      final existing = await _loadExisting(api, auth);

      if (!mounted) return;

      // Process each record
      int i = 0;
      for (final rec in records) {
        i++;
        setState(() {
          _progress = i / records.length;
          _statusText = '$i/${records.length}: ${rec.name}';
        });

        final dupId = _findDuplicate(rec, existing);
        if (dupId != null) {
          if (_dupStrategy == _DupStrategy.overwrite) {
            try {
              await _updateEntry(api, auth, dupId, rec);
              _updated++;
            } catch (_) {
              _failed++;
            }
          } else {
            _skipped++;
          }
        } else {
          try {
            final folderId = await _resolveFolder(api, rec.folderPath);
            final entryId = await _createEntry(api, auth, rec, folderId: folderId);
            existing.add((id: entryId, name: rec.name, username: rec.username));
            _created++;
          } catch (_) {
            _failed++;
          }
        }

        if (!mounted) return;
      }

      if (!mounted) return;

      final doneMsg = _failed > 0
          ? 'Done with errors: $_created created, $_updated updated, $_skipped skipped, $_failed failed'
          : 'Done: $_created created, $_updated updated, $_skipped skipped';

      // Refresh vault so new entries appear immediately.
      ref.invalidate(entriesProvider);
      ref.invalidate(foldersProvider);

      setState(() {
        _running = false;
        _progress = 1;
        _statusText = doneMsg;
      });

      _showSnackBar(doneMsg, isError: _failed > 0 && _created + _updated == 0);
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _running = false;
        _statusText = 'Error: $e';
      });
      _showSnackBar('Import failed: $e', isError: true);
    }
  }

  Future<({List<EntryRecord> records, List<String> warnings})> _parse(
      Uint8List bytes) async {
    switch (_format) {
      case _ImportFormat.csvGeneric:
        return (
          records: parseCsv(utf8.decode(bytes), CsvImportFormat.generic),
          warnings: <String>[],
        );
      case _ImportFormat.csvChrome:
        return (
          records: parseCsv(utf8.decode(bytes), CsvImportFormat.chrome),
          warnings: <String>[],
        );
      case _ImportFormat.csvLastPass:
        return (
          records: parseCsv(utf8.decode(bytes), CsvImportFormat.lastpass),
          warnings: <String>[],
        );
      case _ImportFormat.csv1Password:
        return (
          records: parseCsv(utf8.decode(bytes), CsvImportFormat.onePassword),
          warnings: <String>[],
        );
      case _ImportFormat.bitwarden:
        return (
          records: parseBitwarden(utf8.decode(bytes)),
          warnings: <String>[],
        );
      case _ImportFormat.psono:
        final r = parsePsono(utf8.decode(bytes));
        return (records: r.records, warnings: r.warnings);
      case _ImportFormat.onepassword1pux:
        final r = parseOnePux(Uint8List.fromList(bytes));
        return (records: r.records, warnings: r.warnings);
    }
  }

  // In-process list of (id, name, username) for fast dedupe without re-fetching.
  List<({String id, String name, String username})> _existingCache = [];

  // Folder path → id cache built at import start from existing tree + newly created folders.
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
          final newId = await api.createFolder(
              CreateFolderRequest(name: path[i], parentId: parentId));
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
            full.entryKey!.encryptedKey, auth.privX25519!);
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
    void set(String k, String v) { if (v.isNotEmpty) m[k] = v; }
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

  Future<String> _createEntry(
      ApiClient api, AuthService auth, EntryRecord rec, {String? folderId}) async {
    final dataKey = VaultCrypto.randomKey();
    final plaintext = utf8.encode(jsonEncode(_recordToPayload(rec)));
    final ciphertext =
        await VaultCrypto.encrypt(SecretKey(dataKey), Uint8List.fromList(plaintext));
    final pubKey = await auth.getPubX25519();
    if (pubKey == null) throw Exception('No public key');
    final encDataKey = await VaultCrypto.encryptDataKey(dataKey, pubKey);
    final userId = await auth.getUserId();

    return api.createEntry(CreateEntryRequest(
      folderId: folderId,
      type: rec.type.isEmpty ? 'password' : rec.type,
      name: rec.name,
      url: rec.url.isNotEmpty ? rec.url : null,
      encryptedData: base64.encode(ciphertext),
      dataNonce: base64.encode(Uint8List(12)),
      entryKeys: [EntryKey(userId: userId!, encryptedKey: encDataKey)],
    ));
  }

  Future<void> _updateEntry(
      ApiClient api, AuthService auth, String id, EntryRecord rec) async {
    final full = await api.getEntry(id);
    if (full.entryKey == null) return;
    final dataKey = await VaultCrypto.decryptDataKey(
        full.entryKey!.encryptedKey, auth.privX25519!);
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

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        // Format selector
        const Text('Format', style: TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
        const SizedBox(height: 8),
        DropdownButtonFormField<_ImportFormat>(
          initialValue: _format,
          decoration: const InputDecoration(border: OutlineInputBorder()),
          items: _ImportFormat.values
              .map((f) => DropdownMenuItem(value: f, child: Text(f.label)))
              .toList(),
          onChanged: _running ? null : (v) => setState(() => _format = v!),
        ),
        const SizedBox(height: 16),

        // Duplicate strategy
        const Text('Duplicates', style: TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
        Row(
          children: [
            for (final s in _DupStrategy.values)
              Expanded(
                child: RadioListTile<_DupStrategy>(
                  title: Text(s == _DupStrategy.skip ? 'Skip' : 'Overwrite',
                      style: const TextStyle(fontSize: 14)),
                  value: s,
                  // ignore: deprecated_member_use
                  groupValue: _dupStrategy,
                  // ignore: deprecated_member_use
                  onChanged: _running ? null : (v) => setState(() => _dupStrategy = v!),
                  activeColor: AppTheme.green,
                  contentPadding: EdgeInsets.zero,
                ),
              ),
          ],
        ),
        const SizedBox(height: 16),

        // Progress
        if (_running || _progress > 0) ...[
          LinearProgressIndicator(
            value: _running ? _progress : 1,
            backgroundColor: AppTheme.border,
            valueColor: AlwaysStoppedAnimation(
                _failed > 0 && _created + _updated == 0 ? AppTheme.error : AppTheme.green),
          ),
          const SizedBox(height: 8),
          Text(_statusText, style: const TextStyle(fontSize: 12, color: AppTheme.onBgDim)),
          if (!_running && (_created > 0 || _updated > 0 || _skipped > 0 || _failed > 0)) ...[
            const SizedBox(height: 4),
            Wrap(
              spacing: 8,
              children: [
                if (_created > 0)
                  _StatChip(label: '$_created created', color: AppTheme.green),
                if (_updated > 0)
                  _StatChip(label: '$_updated updated', color: Colors.blue),
                if (_skipped > 0)
                  _StatChip(label: '$_skipped skipped', color: AppTheme.onBgDim),
                if (_failed > 0)
                  _StatChip(label: '$_failed failed', color: AppTheme.error),
              ],
            ),
          ],
          if (_warnings.isNotEmpty) ...[
            const SizedBox(height: 8),
            ..._warnings.map(
              (w) => Padding(
                padding: const EdgeInsets.only(bottom: 2),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Icon(Icons.warning_amber_rounded,
                        size: 14, color: Colors.amber),
                    const SizedBox(width: 4),
                    Expanded(
                      child: Text(w,
                          style: const TextStyle(fontSize: 11, color: Colors.amber)),
                    ),
                  ],
                ),
              ),
            ),
          ],
          const SizedBox(height: 16),
        ],

        SizedBox(
          width: double.infinity,
          child: PbButton(
            label: _running ? 'Importing…' : 'Select File & Import',
            onPressed: _running ? null : _pickAndImport,
            icon: Icons.upload_file,
          ),
        ),
      ],
    );
  }
}

class _StatChip extends StatelessWidget {
  final String label;
  final Color color;

  const _StatChip({required this.label, required this.color});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: color.withValues(alpha: 0.4)),
      ),
      child: Text(label, style: TextStyle(fontSize: 11, color: color)),
    );
  }
}

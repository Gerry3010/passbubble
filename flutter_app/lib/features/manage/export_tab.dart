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
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:share_plus/share_plus.dart';

import '../../core/api/api_client.dart';
import '../../core/api/models.dart';
import '../../core/auth/auth_service.dart';
import '../../core/crypto/vault_crypto.dart';
import '../../core/importexport/bitwarden_format.dart';
import '../../core/importexport/csv_format.dart';
import '../../core/importexport/entry_record.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_button.dart';

enum _ExportFormat { csv, bitwarden }

extension on _ExportFormat {
  String get label => switch (this) {
        _ExportFormat.csv => 'CSV (Generic)',
        _ExportFormat.bitwarden => 'Bitwarden JSON',
      };
  String get filename => switch (this) {
        _ExportFormat.csv => 'passbubble-export.csv',
        _ExportFormat.bitwarden => 'passbubble-export.json',
      };
  String get apiFormat => switch (this) {
        _ExportFormat.csv => 'csv',
        _ExportFormat.bitwarden => 'bitwarden',
      };
}

class ExportTab extends ConsumerStatefulWidget {
  const ExportTab({super.key});

  @override
  ConsumerState<ExportTab> createState() => _ExportTabState();
}

class _ExportTabState extends ConsumerState<ExportTab> {
  _ExportFormat _format = _ExportFormat.csv;
  bool _running = false;
  String _statusText = '';

  Future<void> _export() async {
    setState(() {
      _running = true;
      _statusText = 'Loading vault...';
    });

    try {
      final api = ref.read(apiClientProvider);
      final auth = ref.read(authServiceProvider);

      final entries = await api.listEntries();

      // Create job for visibility
      JobResponse? job;
      try {
        job = await api.createJob(CreateJobRequest(
          type: 'export',
          format: _format.apiFormat,
          dupStrategy: 'skip',
          totalItems: entries.length,
          clientName: 'flutter',
        ));
      } catch (_) {}

      setState(() => _statusText = 'Decrypting entries...');
      final records = <EntryRecord>[];

      for (final e in entries) {
        try {
          final full = await api.getEntry(e.id);
          if (full.entryKey == null) continue;
          final dataKey = await VaultCrypto.decryptDataKey(
              full.entryKey!.encryptedKey, auth.privX25519!);
          final ciphertext = base64.decode(full.encryptedData);
          final plaintext = await VaultCrypto.decrypt(SecretKey(dataKey), ciphertext);
          final data = jsonDecode(utf8.decode(plaintext)) as Map<String, dynamic>;

          records.add(EntryRecord(
            name: e.name,
            url: e.url,
            type: e.type,
            username: data['username'] as String? ?? '',
            password: data['password'] as String? ?? '',
            totpSecret: data['totp_secret'] as String? ?? '',
            notes: data['notes'] as String? ?? '',
          ));
        } catch (_) {}
      }

      setState(() => _statusText = 'Generating file...');
      final content = switch (_format) {
        _ExportFormat.csv => exportCsv(records),
        _ExportFormat.bitwarden => exportBitwarden(records),
      };

      // Finalize job
      if (job != null) {
        try {
          await api.patchJob(job.id, UpdateJobRequest(
            status: 'completed',
            processedItems: records.length,
            createdItems: records.length,
          ));
        } catch (_) {}
      }

      setState(() {
        _running = false;
        _statusText = 'Exported ${records.length} entries';
      });

      // Share the file
      await Share.shareXFiles(
        [XFile.fromData(utf8.encode(content), name: _format.filename)],
        subject: 'Passbubble Export',
      );
    } catch (e) {
      setState(() {
        _running = false;
        _statusText = 'Error: $e';
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        const Text(
          'Export your vault to a file you can import into another password manager.',
          style: TextStyle(color: AppTheme.onBgDim),
        ),
        const SizedBox(height: 16),

        const Text('Format', style: TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
        const SizedBox(height: 8),
        DropdownButtonFormField<_ExportFormat>(
          value: _format,
          decoration: const InputDecoration(border: OutlineInputBorder()),
          items: _ExportFormat.values
              .map((f) => DropdownMenuItem(value: f, child: Text(f.label)))
              .toList(),
          onChanged: _running ? null : (v) => setState(() => _format = v!),
        ),
        const SizedBox(height: 16),

        Container(
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            border: Border.all(color: Colors.orange.withValues(alpha: 0.5)),
            color: Colors.orange.withValues(alpha: 0.05),
          ),
          child: const Row(
            children: [
              Icon(Icons.warning_outlined, color: Colors.orange, size: 20),
              SizedBox(width: 8),
              Expanded(
                child: Text(
                  'Export contains plaintext passwords. Store the file securely and delete it after importing.',
                  style: TextStyle(fontSize: 12, color: Colors.orange),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 16),

        if (_statusText.isNotEmpty) ...[
          Text(_statusText, style: const TextStyle(fontSize: 12, color: AppTheme.onBgDim)),
          const SizedBox(height: 12),
        ],

        SizedBox(
          width: double.infinity,
          child: PbButton(
            label: _running ? 'Exporting…' : 'Export Vault',
            onPressed: _running ? null : _export,
            icon: Icons.download,
          ),
        ),
      ],
    );
  }
}

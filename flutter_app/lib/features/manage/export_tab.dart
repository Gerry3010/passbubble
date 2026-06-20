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

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/jobs/job_runner.dart';
import '../../core/jobs/local_job.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_button.dart';

/// Collects export options and hands the work to the [JobRunner]. The export
/// runs as a background job; on completion the file is shared and the result
/// SnackBar/detail sheet offer a SHARE action to re-share it.
class ExportTab extends ConsumerStatefulWidget {
  const ExportTab({super.key});

  @override
  ConsumerState<ExportTab> createState() => _ExportTabState();
}

class _ExportTabState extends ConsumerState<ExportTab> {
  ExportFormat _format = ExportFormat.csv;
  bool _includeFiles = false;
  bool _filesAsBase64 = false;

  void _export() {
    unawaited(ref.read(jobRunnerProvider.notifier).startExport(
          format: _format,
          options: ExportOptions(
            includeFiles: _includeFiles,
            filesAsBase64: _filesAsBase64,
          ),
        ));
    DefaultTabController.of(context).animateTo(3);
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
        DropdownButtonFormField<ExportFormat>(
          initialValue: _format,
          decoration: const InputDecoration(border: OutlineInputBorder()),
          items: ExportFormat.values
              .map((f) => DropdownMenuItem(value: f, child: Text(f.label)))
              .toList(),
          onChanged: (v) => setState(() => _format = v!),
        ),
        const SizedBox(height: 16),

        // File options (only for formats that support them)
        if (_format.supportsFiles) ...[
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            title: const Text('Include file attachments'),
            subtitle: const Text(
              'Embed file custom fields in the export',
              style: TextStyle(fontSize: 12, color: AppTheme.onBgDim),
            ),
            value: _includeFiles,
            activeThumbColor: AppTheme.green,
            onChanged: (v) => setState(() => _includeFiles = v),
          ),
          if (_includeFiles && _format == ExportFormat.bitwarden)
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('Encode files as Base64'),
              subtitle: const Text(
                'Files stored as data: URIs in hidden custom fields',
                style: TextStyle(fontSize: 12, color: AppTheme.onBgDim),
              ),
              value: _filesAsBase64,
              activeThumbColor: AppTheme.green,
              onChanged: (v) => setState(() => _filesAsBase64 = v),
            ),
          const SizedBox(height: 8),
        ],

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

        const Text(
          'The export runs as a background job — track progress in the Jobs tab. '
          'The file is shared when it is ready.',
          style: TextStyle(fontSize: 12, color: AppTheme.onBgDim),
        ),
        const SizedBox(height: 16),

        SizedBox(
          width: double.infinity,
          child: PbButton(
            label: 'Export Vault',
            onPressed: _export,
            icon: Icons.download,
          ),
        ),
      ],
    );
  }
}

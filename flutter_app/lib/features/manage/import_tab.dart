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

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/jobs/job_runner.dart';
import '../../core/jobs/local_job.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_button.dart';

/// Picks a file and hands the import to the [JobRunner], which runs it in the
/// app-root container so it survives navigating away from this screen. Live
/// progress and the log live in the Manage ▸ Jobs tab.
class ImportTab extends ConsumerStatefulWidget {
  const ImportTab({super.key});

  @override
  ConsumerState<ImportTab> createState() => _ImportTabState();
}

class _ImportTabState extends ConsumerState<ImportTab> {
  ImportFormat _format = ImportFormat.csvGeneric;
  DupStrategy _dupStrategy = DupStrategy.skip;

  Future<void> _pickAndImport() async {
    final result = await FilePicker.platform.pickFiles(withData: true);
    if (result == null || result.files.isEmpty) return;

    final bytes = result.files.first.bytes;
    if (bytes == null) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Could not read file data. Please try again.')),
      );
      return;
    }

    // Kick off the job (fire-and-forget — it keeps running across navigation).
    // The runner shows the start banner once the true item count is known.
    unawaited(ref.read(jobRunnerProvider.notifier).startImport(
          bytes: bytes,
          format: _format,
          dup: _dupStrategy,
        ));

    if (!mounted) return;
    // Jump to the Jobs tab for live progress.
    DefaultTabController.of(context).animateTo(3);
  }

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        const Text('Format', style: TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
        const SizedBox(height: 8),
        DropdownButtonFormField<ImportFormat>(
          initialValue: _format,
          decoration: const InputDecoration(border: OutlineInputBorder()),
          items: ImportFormat.values
              .map((f) => DropdownMenuItem(value: f, child: Text(f.label)))
              .toList(),
          onChanged: (v) => setState(() => _format = v!),
        ),
        const SizedBox(height: 16),

        const Text('Duplicates', style: TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
        Row(
          children: [
            for (final s in DupStrategy.values)
              Expanded(
                child: RadioListTile<DupStrategy>(
                  title: Text(s == DupStrategy.skip ? 'Skip' : 'Overwrite',
                      style: const TextStyle(fontSize: 14)),
                  value: s,
                  // ignore: deprecated_member_use
                  groupValue: _dupStrategy,
                  // ignore: deprecated_member_use
                  onChanged: (v) => setState(() => _dupStrategy = v!),
                  activeColor: AppTheme.green,
                  contentPadding: EdgeInsets.zero,
                ),
              ),
          ],
        ),
        const SizedBox(height: 16),

        const Text(
          'The import runs as a background job — it keeps going if you leave this '
          'screen. Track progress and the log in the Jobs tab.',
          style: TextStyle(fontSize: 12, color: AppTheme.onBgDim),
        ),
        const SizedBox(height: 16),

        SizedBox(
          width: double.infinity,
          child: PbButton(
            label: 'Select File & Import',
            onPressed: _pickAndImport,
            icon: Icons.upload_file,
          ),
        ),
      ],
    );
  }
}

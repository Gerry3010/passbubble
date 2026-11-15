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
import 'dart:convert';
import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/importexport/credential_exchange.dart';
import '../../core/importexport/cxf_format.dart';
import '../../core/jobs/job_runner.dart';
import '../../core/jobs/local_job.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_button.dart';

/// Picks a file and hands the import to the [JobRunner], which runs it in the
/// app-root container so it survives navigating away from this screen. Live
/// progress and the log live in the Manage ▸ Jobs tab. Credential Exchange
/// data handed over by the system ([pendingCredentialExchangeProvider]) is
/// offered at the top, with the same duplicate handling.
class ImportTab extends ConsumerStatefulWidget {
  const ImportTab({super.key});

  @override
  ConsumerState<ImportTab> createState() => _ImportTabState();
}

class _ImportTabState extends ConsumerState<ImportTab> {
  ImportFormat _format = ImportFormat.csvGeneric;
  DupStrategy _dupStrategy = DupStrategy.skip;
  bool _overwriteCompletely = false;

  @override
  void initState() {
    super.initState();
    if (ref.read(pendingCredentialExchangeProvider) != null) _format = ImportFormat.cxf;
  }

  void _setDupStrategy(DupStrategy strategy) => setState(() {
        _dupStrategy = strategy;
        // Removes data, so it never comes back switched on by itself.
        if (strategy != DupStrategy.overwrite) _overwriteCompletely = false;
      });

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

    _start(bytes, _format);
  }

  /// Kicks off the job (fire-and-forget — it keeps running across navigation)
  /// and jumps to the Jobs tab for live progress. The runner shows the start
  /// banner once the true item count is known.
  void _start(Uint8List bytes, ImportFormat format) {
    unawaited(ref.read(jobRunnerProvider.notifier).startImport(
          bytes: bytes,
          format: format,
          dup: _dupStrategy,
          overwriteMode:
              _overwriteCompletely ? OverwriteMode.complete : OverwriteMode.merge,
        ));
    if (!mounted) return;
    DefaultTabController.of(context).animateTo(3);
  }

  void _importExchange(String json) {
    ref.read(pendingCredentialExchangeProvider.notifier).state = null;
    _start(Uint8List.fromList(utf8.encode(json)), ImportFormat.cxf);
  }

  Widget _exchangeCard(String json) {
    String summary;
    try {
      final (:items, :exporter) = describeCxf(json);
      summary = '$items ${items == 1 ? 'item' : 'items'}'
          '${exporter.isEmpty ? '' : ' from $exporter'} are ready to import.';
    } on FormatException {
      summary = 'The data could not be read.';
    }
    return Container(
      key: const ValueKey('credential-exchange'),
      margin: const EdgeInsets.only(bottom: 16),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        border: Border.all(color: AppTheme.green),
        color: AppTheme.greenFaint,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('Credential Exchange',
              style: TextStyle(color: AppTheme.green, fontWeight: FontWeight.w600)),
          const SizedBox(height: 4),
          Text('$summary Choose how to handle duplicates below, then import.',
              style: const TextStyle(fontSize: 13)),
          const SizedBox(height: 8),
          Row(children: [
            Expanded(
              child: PbButton(
                key: const ValueKey('credential-exchange-import'),
                label: 'Import',
                icon: Icons.download_done_outlined,
                onPressed: () => _importExchange(json),
              ),
            ),
            const SizedBox(width: 8),
            TextButton(
              key: const ValueKey('credential-exchange-discard'),
              onPressed: () =>
                  ref.read(pendingCredentialExchangeProvider.notifier).state = null,
              child: const Text('Discard'),
            ),
          ]),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final exchange = ref.watch(pendingCredentialExchangeProvider);
    ref.listen<String?>(pendingCredentialExchangeProvider, (_, next) {
      if (next != null) setState(() => _format = ImportFormat.cxf);
    });
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        if (exchange != null) _exchangeCard(exchange),
        const Text('Format', style: TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
        const SizedBox(height: 8),
        DropdownButtonFormField<ImportFormat>(
          // Keyed so a hand-over can switch it to CXF.
          key: ValueKey(_format),
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
                  onChanged: (v) => _setDupStrategy(v!),
                  activeColor: AppTheme.green,
                  contentPadding: EdgeInsets.zero,
                ),
              ),
          ],
        ),
        if (_dupStrategy == DupStrategy.overwrite)
          CheckboxListTile(
            key: const ValueKey('overwrite-completely'),
            title: const Text('Overwrite completely', style: TextStyle(fontSize: 14)),
            subtitle: const Text(
              'Fields missing from the file are removed from the entry. '
              'Off: only values in the file replace existing ones.',
              style: TextStyle(fontSize: 12, color: AppTheme.onBgDim),
            ),
            value: _overwriteCompletely,
            onChanged: (v) => setState(() => _overwriteCompletely = v ?? false),
            activeColor: AppTheme.green,
            contentPadding: EdgeInsets.zero,
            controlAffinity: ListTileControlAffinity.leading,
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

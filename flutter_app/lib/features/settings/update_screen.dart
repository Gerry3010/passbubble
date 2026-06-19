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

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../core/theme/app_theme.dart';
import '../../shared/widgets/markdown_text.dart';
import 'providers/version_provider.dart';

const _updateCmd = 'docker compose pull && docker compose up -d';

class UpdateScreen extends ConsumerWidget {
  const UpdateScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final versionAsync = ref.watch(versionInfoProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('> VERSION & UPDATES'),
      ),
      body: versionAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => _ErrorView(error: e.toString(), onRetry: () => ref.invalidate(versionInfoProvider)),
        data: (info) => _VersionBody(info: info),
      ),
    );
  }
}

class _VersionBody extends StatelessWidget {
  final VersionInfo info;
  const _VersionBody({required this.info});

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        _VersionRow(label: 'SERVER VERSION', value: info.serverVersion),
        const SizedBox(height: 8),
        _VersionRow(
          label: 'LATEST RELEASE',
          value: info.latestVersion,
          badge: info.isUpToDate
              ? _StatusBadge(label: 'UP TO DATE', color: AppTheme.green)
              : const _StatusBadge(label: 'UPDATE AVAILABLE', color: Colors.amber),
        ),
        const SizedBox(height: 24),
        if (info.releaseNotes.isNotEmpty) ...[
          _SectionLabel(label: 'WHAT\'S NEW'),
          const SizedBox(height: 8),
          Container(
            width: double.infinity,
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              border: Border.all(color: AppTheme.green.withValues(alpha: 0.3)),
            ),
            child: MarkdownText(info.releaseNotes),
          ),
          const SizedBox(height: 24),
        ],
        _SectionLabel(label: 'UPDATE INSTRUCTIONS'),
        const SizedBox(height: 8),
        Container(
          padding: const EdgeInsets.all(12),
          color: Colors.black,
          child: SelectableText(
            _updateCmd,
            style: const TextStyle(
              fontFamily: 'monospace',
              color: AppTheme.green,
              fontSize: 13,
            ),
          ),
        ),
        const SizedBox(height: 12),
        Row(
          children: [
            Expanded(
              child: OutlinedButton.icon(
                icon: const Icon(Icons.copy, size: 16),
                label: const Text('COPY COMMAND'),
                onPressed: () {
                  Clipboard.setData(const ClipboardData(text: _updateCmd));
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('Command copied to clipboard')),
                  );
                },
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: OutlinedButton.icon(
                icon: const Icon(Icons.open_in_new, size: 16),
                label: const Text('VIEW RELEASE'),
                onPressed: () => launchUrl(
                  Uri.parse(info.releaseUrl),
                  mode: LaunchMode.externalApplication,
                ),
              ),
            ),
          ],
        ),
      ],
    );
  }
}

class _VersionRow extends StatelessWidget {
  final String label;
  final String value;
  final Widget? badge;

  const _VersionRow({required this.label, required this.value, this.badge});

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Text(
          label,
          style: const TextStyle(
            color: AppTheme.green,
            fontSize: 11,
            letterSpacing: 1.5,
          ),
        ),
        const Spacer(),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
          decoration: BoxDecoration(
            border: Border.all(color: AppTheme.green.withValues(alpha: 0.5)),
          ),
          child: Text(
            value,
            style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
          ),
        ),
        if (badge != null) ...[
          const SizedBox(width: 8),
          badge!,
        ],
      ],
    );
  }
}

class _StatusBadge extends StatelessWidget {
  final String label;
  final Color color;
  const _StatusBadge({required this.label, required this.color});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(border: Border.all(color: color)),
      child: Text(
        label,
        style: TextStyle(color: color, fontSize: 10, letterSpacing: 1),
      ),
    );
  }
}

class _SectionLabel extends StatelessWidget {
  final String label;
  const _SectionLabel({required this.label});

  @override
  Widget build(BuildContext context) {
    return Text(
      label,
      style: const TextStyle(
        color: AppTheme.green,
        fontSize: 11,
        letterSpacing: 2,
        fontWeight: FontWeight.w600,
      ),
    );
  }
}

class _ErrorView extends StatelessWidget {
  final String error;
  final VoidCallback onRetry;
  const _ErrorView({required this.error, required this.onRetry});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, color: AppTheme.error, size: 40),
            const SizedBox(height: 12),
            Text('Failed to load version info', style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 8),
            Text(error, style: Theme.of(context).textTheme.bodySmall, textAlign: TextAlign.center),
            const SizedBox(height: 16),
            OutlinedButton(onPressed: onRetry, child: const Text('RETRY')),
          ],
        ),
      ),
    );
  }
}

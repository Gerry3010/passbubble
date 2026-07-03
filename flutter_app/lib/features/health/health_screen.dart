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

// Vault-wide password health. Decrypts passwords in memory only (like the
// username search index) and analyses them locally; the optional breach check
// uses HIBP's k-anonymity range API.

import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/api/api_client.dart';
import '../../core/auth/auth_service.dart';
import '../../core/crypto/vault_crypto.dart';
import '../../core/health/health_service.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/prompt_title.dart';

class HealthScreen extends ConsumerStatefulWidget {
  const HealthScreen({super.key});

  @override
  ConsumerState<HealthScreen> createState() => _HealthScreenState();
}

class _HealthScreenState extends ConsumerState<HealthScreen> {
  HealthReport? _report;
  bool _checkBreaches = false;
  bool _busy = false;
  String? _error;

  Future<void> _run() async {
    final auth = ref.read(authServiceProvider);
    final privX = auth.privX25519;
    final privM = auth.privMLKEM;
    if (privX == null || privM == null) {
      setState(() => _error = 'Vault is locked — unlock first');
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final api = ref.read(apiClientProvider);
      final items = <HealthItemInput>[];
      for (final e in await api.listEntriesFull()) {
        final encKey = e.entryKey;
        if (encKey == null) continue;
        try {
          final dataKey = await VaultCrypto.decryptDataKey(
              encKey.encryptedKey, privX, privM);
          final data = await VaultCrypto.decryptEntryData(
              e.encryptedData, Uint8List.fromList(dataKey));
          final password = data['password'];
          if (password is String && password.isNotEmpty) {
            items.add(HealthItemInput(
              id: e.id,
              name: e.name,
              password: password,
              updatedAt: e.updatedAt,
            ));
          }
        } catch (_) {
          // skip entries we cannot read
        }
      }
      final report =
          await computeHealthReport(items, checkBreaches: _checkBreaches);
      if (mounted) setState(() => _report = report);
    } catch (e) {
      if (mounted) setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Color _scoreColor(int s) => s >= 80
      ? AppTheme.green
      : s >= 50
          ? AppTheme.amber
          : AppTheme.error;

  @override
  Widget build(BuildContext context) {
    final report = _report;
    return Scaffold(
      appBar: AppBar(title: const PromptTitle('health')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          SwitchListTile(
            value: _checkBreaches,
            onChanged: (v) => setState(() => _checkBreaches = v),
            title: const Text('Check known breaches (HIBP)'),
            subtitle: const Text(
              'Uses k-anonymity — your passwords never leave this device.',
              style: TextStyle(fontSize: 12, color: AppTheme.onBgDim),
            ),
            activeThumbColor: AppTheme.green,
            contentPadding: EdgeInsets.zero,
          ),
          const SizedBox(height: 8),
          FilledButton(
            onPressed: _busy ? null : _run,
            style: FilledButton.styleFrom(
              backgroundColor: AppTheme.green,
              foregroundColor: AppTheme.bg,
            ),
            child: Text(_busy
                ? 'Analyzing…'
                : report == null
                    ? 'Run health check'
                    : 'Re-run health check'),
          ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(top: 12),
              child:
                  Text(_error!, style: const TextStyle(color: AppTheme.error)),
            ),
          if (report != null) ...[
            const SizedBox(height: 16),
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: AppTheme.surface,
                border: Border.all(color: AppTheme.border),
                borderRadius: BorderRadius.circular(6),
              ),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Text('vault score\n(${report.total} passwords)',
                      style: const TextStyle(
                          color: AppTheme.onBgDim, fontSize: 12)),
                  Text('${report.score}',
                      style: TextStyle(
                          color: _scoreColor(report.score),
                          fontSize: 36,
                          fontWeight: FontWeight.bold)),
                ],
              ),
            ),
            _Section(
              title: 'breached (${report.breached.length})',
              color: AppTheme.error,
              findings: report.breached,
              emptyLabel: report.breachChecked ? 'none found' : 'not checked',
            ),
            _Section(
              title: 'reused (${report.reused.length})',
              color: AppTheme.amber,
              findings: report.reused,
            ),
            _Section(
              title: 'weak (${report.weak.length})',
              color: AppTheme.amber,
              findings: report.weak,
            ),
            _Section(
              title: 'old (${report.old.length})',
              color: AppTheme.onBgDim,
              findings: report.old,
            ),
          ],
        ],
      ),
    );
  }
}

class _Section extends StatelessWidget {
  final String title;
  final Color color;
  final List<HealthFinding> findings;
  final String emptyLabel;
  const _Section({
    required this.title,
    required this.color,
    required this.findings,
    this.emptyLabel = 'none',
  });

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(0, 16, 0, 6),
          child: Text('# $title',
              style: TextStyle(
                  color: color, fontWeight: FontWeight.bold, fontSize: 13)),
        ),
        if (findings.isEmpty)
          Padding(
            padding: const EdgeInsets.only(left: 8),
            child: Text(emptyLabel,
                style:
                    const TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
          )
        else
          for (final f in findings)
            Container(
              margin: const EdgeInsets.only(bottom: 4),
              decoration: BoxDecoration(
                color: AppTheme.surface,
                border: Border.all(color: AppTheme.border),
                borderRadius: BorderRadius.circular(4),
              ),
              child: ListTile(
                dense: true,
                visualDensity: VisualDensity.compact,
                title: Text(f.name,
                    maxLines: 1, overflow: TextOverflow.ellipsis),
                trailing: Text(f.detail,
                    style: const TextStyle(
                        color: AppTheme.onBgDim, fontSize: 11)),
                onTap: () => context.go('/entries/${f.id}'),
              ),
            ),
      ],
    );
  }
}

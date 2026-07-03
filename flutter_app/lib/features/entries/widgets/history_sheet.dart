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

// Bottom sheet listing an entry's version history. A tapped version is
// decrypted on demand (the version response carries the caller's
// contemporaneous wrapped key) and can be restored — the restore itself is
// server-side, so the current state is versioned first and nothing is lost.

import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/models.dart';
import '../../../core/auth/auth_service.dart';
import '../../../core/crypto/vault_crypto.dart';
import '../../../core/theme/app_theme.dart';

Future<void> showHistorySheet(
  BuildContext context,
  WidgetRef ref,
  EntryResponse entry, {
  VoidCallback? onRestored,
}) {
  return showModalBottomSheet<void>(
    context: context,
    backgroundColor: AppTheme.bg,
    isScrollControlled: true,
    builder: (_) => _HistorySheet(entry: entry, onRestored: onRestored),
  );
}

class _HistorySheet extends ConsumerStatefulWidget {
  final EntryResponse entry;
  final VoidCallback? onRestored;
  const _HistorySheet({required this.entry, this.onRestored});

  @override
  ConsumerState<_HistorySheet> createState() => _HistorySheetState();
}

class _HistorySheetState extends ConsumerState<_HistorySheet> {
  late Future<List<EntryVersionResponse>> _versions;

  @override
  void initState() {
    super.initState();
    _versions = ref.read(apiClientProvider).listVersions(widget.entry.id);
  }

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              '# history',
              style: TextStyle(
                  color: AppTheme.green,
                  fontWeight: FontWeight.bold,
                  fontSize: 14),
            ),
            const SizedBox(height: 4),
            const Text(
              'Previous states of this entry (kept for the last 20 changes).',
              style: TextStyle(color: AppTheme.onBgDim, fontSize: 12),
            ),
            const SizedBox(height: 8),
            Flexible(
              child: FutureBuilder<List<EntryVersionResponse>>(
                future: _versions,
                builder: (context, snap) {
                  if (snap.connectionState != ConnectionState.done) {
                    return const Padding(
                      padding: EdgeInsets.all(24),
                      child: Center(child: CircularProgressIndicator()),
                    );
                  }
                  if (snap.hasError) {
                    return Padding(
                      padding: const EdgeInsets.all(16),
                      child: Text('${snap.error}',
                          style: const TextStyle(color: AppTheme.error)),
                    );
                  }
                  final versions = snap.data ?? const [];
                  if (versions.isEmpty) {
                    return const Padding(
                      padding: EdgeInsets.all(16),
                      child: Text('No previous versions yet.',
                          style: TextStyle(color: AppTheme.onBgDim)),
                    );
                  }
                  return ListView.builder(
                    shrinkWrap: true,
                    itemCount: versions.length,
                    itemBuilder: (context, i) => _VersionTile(
                      entry: widget.entry,
                      version: versions[i],
                      onRestored: widget.onRestored,
                    ),
                  );
                },
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _VersionTile extends ConsumerStatefulWidget {
  final EntryResponse entry;
  final EntryVersionResponse version;
  final VoidCallback? onRestored;
  const _VersionTile({
    required this.entry,
    required this.version,
    this.onRestored,
  });

  @override
  ConsumerState<_VersionTile> createState() => _VersionTileState();
}

class _VersionTileState extends ConsumerState<_VersionTile> {
  Map<String, dynamic>? _decrypted;
  bool _busy = false;
  bool _showPassword = false;

  String get _when {
    final ts = DateTime.tryParse(widget.version.createdAt);
    if (ts == null) return widget.version.createdAt;
    final local = ts.toLocal();
    String two(int n) => n.toString().padLeft(2, '0');
    return '${local.year}-${two(local.month)}-${two(local.day)} '
        '${two(local.hour)}:${two(local.minute)}';
  }

  Future<void> _reveal() async {
    if (_decrypted != null) {
      setState(() => _decrypted = null); // collapse
      return;
    }
    final auth = ref.read(authServiceProvider);
    if (auth.privX25519 == null) return;
    setState(() => _busy = true);
    try {
      // Fetch the full version (blob + our contemporaneous key) and reuse the
      // normal entry decrypt path via the EntryResponse adapter.
      final full = await ref
          .read(apiClientProvider)
          .getVersion(widget.entry.id, widget.version.id);
      final asEntry = full.toEntryResponse(type: widget.entry.type);
      final key = asEntry.entryKey;
      if (key == null) {
        throw Exception('No key for this version (shared later?)');
      }
      final dataKey = await VaultCrypto.decryptDataKey(
        key.encryptedKey,
        auth.privX25519!,
        auth.privMLKEM!,
      );
      final data = await VaultCrypto.decryptEntryData(
        asEntry.encryptedData,
        Uint8List.fromList(dataKey),
      );
      if (mounted) setState(() => _decrypted = data);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('Decrypt failed: $e')));
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _restore() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Restore this version?'),
        content: Text(
            'The entry is reset to its state of $_when. The current state is '
            'kept in the history.'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Cancel')),
          TextButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Restore',
                  style: TextStyle(color: AppTheme.green))),
        ],
      ),
    );
    if (ok != true || !mounted) return;
    setState(() => _busy = true);
    try {
      await ref
          .read(apiClientProvider)
          .restoreVersion(widget.entry.id, widget.version.id);
      if (!mounted) return;
      Navigator.of(context).pop(); // close the sheet
      widget.onRestored?.call();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('Restore failed: $e')));
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final d = _decrypted;
    final username = d?['username'] as String? ?? '';
    final password = d?['password'] as String? ?? '';
    return Container(
      margin: const EdgeInsets.only(bottom: 6),
      decoration: BoxDecoration(
        color: AppTheme.surface,
        border: Border.all(color: AppTheme.border),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Column(
        children: [
          ListTile(
            dense: true,
            leading: const Icon(Icons.history, color: AppTheme.green, size: 20),
            title: Text(_when, style: const TextStyle(fontSize: 13)),
            subtitle: Text(widget.version.name,
                style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12),
                overflow: TextOverflow.ellipsis),
            trailing: _busy
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2))
                : Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      IconButton(
                        icon: Icon(
                            d == null
                                ? Icons.visibility_outlined
                                : Icons.visibility_off_outlined,
                            size: 18,
                            color: AppTheme.onBgDim),
                        tooltip: d == null ? 'Show contents' : 'Hide',
                        onPressed: _reveal,
                      ),
                      IconButton(
                        icon: const Icon(Icons.restore,
                            size: 18, color: AppTheme.green),
                        tooltip: 'Restore this version',
                        onPressed: _restore,
                      ),
                    ],
                  ),
          ),
          if (d != null)
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 10),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (username.isNotEmpty)
                    Text('user: $username',
                        style: const TextStyle(
                            color: AppTheme.onBg, fontSize: 12)),
                  if (password.isNotEmpty)
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            'pass: ${_showPassword ? password : '•' * 8}',
                            style: const TextStyle(
                                color: AppTheme.onBg, fontSize: 12),
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                        GestureDetector(
                          onTap: () =>
                              setState(() => _showPassword = !_showPassword),
                          child: Text(_showPassword ? 'hide' : 'show',
                              style: const TextStyle(
                                  color: AppTheme.green, fontSize: 12)),
                        ),
                      ],
                    ),
                  if ((d['notes'] as String? ?? '').isNotEmpty)
                    Text('notes: ${d['notes']}',
                        maxLines: 3,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                            color: AppTheme.onBgDim, fontSize: 12)),
                ],
              ),
            ),
        ],
      ),
    );
  }
}

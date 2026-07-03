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

// Trash: soft-deleted entries, restorable until the server purge removes them
// (30 days after deletion).

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_client.dart';
import '../../core/api/models.dart';
import '../../core/auth/auth_service.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/prompt_title.dart';
import 'entries_list_screen.dart' show entriesProvider;

final trashProvider = FutureProvider.autoDispose<List<EntryResponse>>(
  (ref) => ref.watch(apiClientProvider).listTrash(),
);

class TrashScreen extends ConsumerWidget {
  const TrashScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final trashAsync = ref.watch(trashProvider);
    return Scaffold(
      appBar: AppBar(title: const PromptTitle('trash')),
      body: trashAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(
            child:
                Text('$e', style: const TextStyle(color: AppTheme.error))),
        data: (entries) {
          if (entries.isEmpty) {
            return const Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(Icons.delete_outline, size: 48, color: AppTheme.onBgDim),
                  SizedBox(height: 12),
                  Text('Trash is empty',
                      style: TextStyle(color: AppTheme.onBgDim)),
                ],
              ),
            );
          }
          return Column(
            children: [
              Container(
                width: double.infinity,
                padding: const EdgeInsets.all(10),
                color: AppTheme.surfaceVariant,
                child: const Text(
                  'Entries in the trash are removed permanently after 30 days.',
                  style: TextStyle(color: AppTheme.onBgDim, fontSize: 12),
                ),
              ),
              Expanded(
                child: RefreshIndicator(
                  onRefresh: () async => ref.invalidate(trashProvider),
                  child: ListView.builder(
                    padding: const EdgeInsets.all(12),
                    itemCount: entries.length,
                    itemBuilder: (context, i) =>
                        _TrashTile(entry: entries[i]),
                  ),
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}

class _TrashTile extends ConsumerWidget {
  final EntryResponse entry;
  const _TrashTile({required this.entry});

  String get _deletedWhen {
    final ts = DateTime.tryParse(entry.deletedAt ?? '');
    if (ts == null) return '';
    final local = ts.toLocal();
    String two(int n) => n.toString().padLeft(2, '0');
    return 'deleted ${local.year}-${two(local.month)}-${two(local.day)}';
  }

  Future<void> _restore(BuildContext context, WidgetRef ref) async {
    try {
      await ref.read(apiClientProvider).restoreEntry(entry.id);
      ref.invalidate(trashProvider);
      ref.invalidate(entriesProvider);
      ref.read(authServiceProvider).refreshAutofill().ignore();
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text('"${entry.name}" restored')));
      }
    } catch (e) {
      if (context.mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('Restore failed: $e')));
      }
    }
  }

  Future<void> _purge(BuildContext context, WidgetRef ref) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Delete permanently?'),
        content: Text(
            '"${entry.name}" and its history will be gone for good. '
            'This cannot be undone.'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Cancel')),
          TextButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: const Text('Delete forever',
                  style: TextStyle(color: AppTheme.error))),
        ],
      ),
    );
    if (ok != true || !context.mounted) return;
    try {
      await ref.read(apiClientProvider).purgeEntry(entry.id);
      ref.invalidate(trashProvider);
    } catch (e) {
      if (context.mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('Delete failed: $e')));
      }
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Container(
      margin: const EdgeInsets.only(bottom: 6),
      decoration: BoxDecoration(
        color: AppTheme.surface,
        border: Border.all(color: AppTheme.border),
        borderRadius: BorderRadius.circular(6),
      ),
      child: ListTile(
        dense: true,
        leading:
            const Icon(Icons.delete_outline, color: AppTheme.onBgDim, size: 20),
        title: Text(entry.name, maxLines: 1, overflow: TextOverflow.ellipsis),
        subtitle: Text(
          [if (entry.url.isNotEmpty) entry.url, _deletedWhen]
              .where((s) => s.isNotEmpty)
              .join(' — '),
          style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12),
          overflow: TextOverflow.ellipsis,
        ),
        trailing: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            IconButton(
              icon: const Icon(Icons.restore_from_trash,
                  size: 20, color: AppTheme.green),
              tooltip: 'Restore',
              onPressed: () => _restore(context, ref),
            ),
            IconButton(
              icon: const Icon(Icons.delete_forever_outlined,
                  size: 20, color: AppTheme.error),
              tooltip: 'Delete permanently',
              onPressed: () => _purge(context, ref),
            ),
          ],
        ),
      ),
    );
  }
}

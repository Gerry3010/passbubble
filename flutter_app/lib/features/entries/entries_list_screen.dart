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
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/api/api_client.dart';
import '../../core/api/models.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/bottom_nav.dart';
import '../../shared/widgets/pb_button.dart';

final entriesProvider = FutureProvider<List<EntryResponse>>((ref) async {
  return ref.watch(apiClientProvider).listEntries();
});

final _searchQueryProvider = StateProvider<String>((ref) => '');

final _filteredEntriesProvider =
    FutureProvider<List<EntryResponse>>((ref) async {
  final q = ref.watch(_searchQueryProvider);
  if (q.isEmpty) return ref.watch(entriesProvider.future);
  return ref.watch(apiClientProvider).searchEntries(q);
});

class EntriesListScreen extends ConsumerStatefulWidget {
  const EntriesListScreen({super.key});

  @override
  ConsumerState<EntriesListScreen> createState() => _EntriesListScreenState();
}

class _EntriesListScreenState extends ConsumerState<EntriesListScreen> {
  final _searchCtrl = TextEditingController();

  @override
  void dispose() {
    _searchCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final entries = ref.watch(_filteredEntriesProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('> VAULT'),
        actions: [
          IconButton(
            icon: const Icon(Icons.settings_outlined),
            onPressed: () => context.go('/settings'),
          ),
        ],
        bottom: PreferredSize(
          preferredSize: const Size.fromHeight(56),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
            child: TextField(
              controller: _searchCtrl,
              onChanged: (v) =>
                  ref.read(_searchQueryProvider.notifier).state = v,
              decoration: InputDecoration(
                hintText: 'search entries...',
                prefixIcon: const Icon(Icons.search, size: 18),
                suffixIcon: _searchCtrl.text.isNotEmpty
                    ? IconButton(
                        icon: const Icon(Icons.clear, size: 18),
                        onPressed: () {
                          _searchCtrl.clear();
                          ref.read(_searchQueryProvider.notifier).state = '';
                        },
                      )
                    : null,
                contentPadding:
                    const EdgeInsets.symmetric(vertical: 8, horizontal: 12),
                isDense: true,
              ),
            ),
          ),
        ),
      ),
      body: entries.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => _ErrorState(error: e.toString()),
        data: (list) {
          if (list.isEmpty) {
            return _EmptyState(
              hasSearch: _searchCtrl.text.isNotEmpty,
            );
          }
          return RefreshIndicator(
            color: AppTheme.green,
            onRefresh: () async => ref.invalidate(entriesProvider),
            child: ListView.separated(
              itemCount: list.length,
              separatorBuilder: (_, _) => const Divider(height: 1),
              itemBuilder: (ctx, i) => _EntryTile(entry: list[i]),
            ),
          );
        },
      ),
      floatingActionButton: FloatingActionButton(
        onPressed: () => context.go('/entries/new'),
        child: const Icon(Icons.add),
      ),
      bottomNavigationBar: const PbBottomNav(currentIndex: 0),
    );
  }
}

class _EntryTile extends StatelessWidget {
  final EntryResponse entry;
  const _EntryTile({required this.entry});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: _typeIcon(entry.type),
      title: Text(
        entry.name,
        style: const TextStyle(
          fontWeight: FontWeight.w500,
          fontSize: 14,
        ),
      ),
      subtitle: entry.url.isNotEmpty
          ? Text(
              entry.url,
              style: const TextStyle(
                color: AppTheme.onBgDim,
                fontSize: 12,
              ),
              overflow: TextOverflow.ellipsis,
            )
          : null,
      trailing: const Icon(Icons.chevron_right, size: 18, color: AppTheme.onBgDim),
      onTap: () => context.go('/entries/${entry.id}'),
    );
  }

  Widget _typeIcon(String type) {
    final (icon, color) = switch (type) {
      'totp' => (Icons.schedule, AppTheme.green),
      'note' => (Icons.note_outlined, Colors.amber),
      'api-key' => (Icons.api_outlined, Colors.purple),
      'ssh-key' => (Icons.terminal_outlined, Colors.cyan),
      _ => (Icons.lock_outline, AppTheme.onBgDim),
    };
    return Container(
      width: 36,
      height: 36,
      decoration: BoxDecoration(
        border: Border.all(color: AppTheme.border),
        color: AppTheme.surfaceVariant,
      ),
      child: Icon(icon, size: 18, color: color),
    );
  }
}

class _EmptyState extends StatelessWidget {
  final bool hasSearch;
  const _EmptyState({required this.hasSearch});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Icon(Icons.lock_open_outlined,
              size: 64, color: AppTheme.onBgDim),
          const SizedBox(height: 16),
          Text(
            hasSearch ? 'No entries found' : 'Your vault is empty',
            style: const TextStyle(color: AppTheme.onBgDim),
          ),
          if (!hasSearch) ...[
            const SizedBox(height: 16),
            PbButton(
              label: 'Add First Entry',
              onPressed: () => context.go('/entries/new'),
              icon: Icons.add,
            ),
          ],
        ],
      ),
    );
  }
}

class _ErrorState extends StatelessWidget {
  final String error;
  const _ErrorState({required this.error});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, color: AppTheme.error, size: 48),
            const SizedBox(height: 12),
            Text(error,
                textAlign: TextAlign.center,
                style: const TextStyle(color: AppTheme.error)),
          ],
        ),
      ),
    );
  }
}


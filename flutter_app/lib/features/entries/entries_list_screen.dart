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

final foldersProvider = FutureProvider<List<FolderResponse>>((ref) {
  return ref.watch(apiClientProvider).listFolders();
});

class EntriesListScreen extends ConsumerStatefulWidget {
  const EntriesListScreen({super.key});

  @override
  ConsumerState<EntriesListScreen> createState() => _EntriesListScreenState();
}

class _EntriesListScreenState extends ConsumerState<EntriesListScreen> {
  final _searchCtrl = TextEditingController();
  String _searchQuery = '';

  // Folder navigation stack — null means root level.
  final List<FolderResponse?> _stack = [null];

  FolderResponse? get _currentFolder => _stack.last;
  bool get _atRoot => _stack.length == 1;

  @override
  void dispose() {
    _searchCtrl.dispose();
    super.dispose();
  }

  List<FolderResponse> _subfolders(List<FolderResponse> roots) {
    if (_currentFolder == null) return roots;
    return _currentFolder!.children;
  }

  List<EntryResponse> _entriesInFolder(List<EntryResponse> all) {
    final id = _currentFolder?.id;
    return all.where((e) => e.folderId == id).toList();
  }

  List<EntryResponse> _searchEntries(List<EntryResponse> all) {
    final q = _searchQuery.toLowerCase();
    return all
        .where((e) =>
            e.name.toLowerCase().contains(q) ||
            e.url.toLowerCase().contains(q))
        .toList();
  }

  @override
  Widget build(BuildContext context) {
    final foldersAsync = ref.watch(foldersProvider);
    final entriesAsync = ref.watch(entriesProvider);

    final isSearching = _searchQuery.isNotEmpty;

    final title = _currentFolder == null
        ? '> VAULT'
        : '> ${_currentFolder!.name.toUpperCase()}';

    return PopScope(
      canPop: _atRoot,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop && !_atRoot) setState(() => _stack.removeLast());
      },
      child: Scaffold(
        appBar: AppBar(
          title: Text(title),
          leading: _atRoot
              ? null
              : IconButton(
                  icon: const Icon(Icons.arrow_back),
                  onPressed: () => setState(() => _stack.removeLast()),
                ),
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
                onChanged: (v) => setState(() => _searchQuery = v),
                decoration: InputDecoration(
                  hintText: 'search entries...',
                  prefixIcon: const Icon(Icons.search, size: 18),
                  suffixIcon: _searchCtrl.text.isNotEmpty
                      ? IconButton(
                          icon: const Icon(Icons.clear, size: 18),
                          onPressed: () {
                            _searchCtrl.clear();
                            setState(() => _searchQuery = '');
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
        body: foldersAsync.when(
          loading: () => const Center(child: CircularProgressIndicator()),
          error: (e, _) => _ErrorState(error: e.toString()),
          data: (rootFolders) => entriesAsync.when(
            loading: () => const Center(child: CircularProgressIndicator()),
            error: (e, _) => _ErrorState(error: e.toString()),
            data: (allEntries) {
              if (isSearching) {
                final results = _searchEntries(allEntries);
                if (results.isEmpty) {
                  return const _EmptyState(mode: _EmptyMode.search);
                }
                return RefreshIndicator(
                  color: AppTheme.green,
                  onRefresh: () async => ref.invalidate(entriesProvider),
                  child: ListView.separated(
                    itemCount: results.length,
                    separatorBuilder: (_, _) => const Divider(height: 1),
                    itemBuilder: (ctx, i) => _EntryTile(entry: results[i]),
                  ),
                );
              }

              final folders = _subfolders(rootFolders);
              final entries = _entriesInFolder(allEntries);

              if (folders.isEmpty && entries.isEmpty) {
                return _EmptyState(
                  mode: _atRoot ? _EmptyMode.vault : _EmptyMode.folder,
                );
              }

              return RefreshIndicator(
                color: AppTheme.green,
                onRefresh: () async {
                  ref.invalidate(foldersProvider);
                  ref.invalidate(entriesProvider);
                },
                child: ListView.separated(
                  itemCount: folders.length + entries.length,
                  separatorBuilder: (_, _) => const Divider(height: 1),
                  itemBuilder: (ctx, i) {
                    if (i < folders.length) {
                      return _FolderTile(
                        folder: folders[i],
                        onTap: () =>
                            setState(() => _stack.add(folders[i])),
                      );
                    }
                    return _EntryTile(entry: entries[i - folders.length]);
                  },
                ),
              );
            },
          ),
        ),
        floatingActionButton: FloatingActionButton(
          onPressed: () {
            final fid = _currentFolder?.id;
            context.go(fid != null ? '/entries/new?folderId=$fid' : '/entries/new');
          },
          child: const Icon(Icons.add),
        ),
        bottomNavigationBar: const PbBottomNav(currentIndex: 0),
      ),
    );
  }
}

class _FolderTile extends StatelessWidget {
  final FolderResponse folder;
  final VoidCallback onTap;
  const _FolderTile({required this.folder, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: Container(
        width: 36,
        height: 36,
        decoration: BoxDecoration(
          border: Border.all(color: AppTheme.border),
          color: AppTheme.surfaceVariant,
        ),
        child: const Icon(Icons.folder_outlined, size: 18, color: AppTheme.green),
      ),
      title: Text(
        folder.name,
        style: const TextStyle(fontWeight: FontWeight.w500, fontSize: 14),
      ),
      subtitle: folder.children.isNotEmpty
          ? Text(
              '${folder.children.length} subfolder${folder.children.length == 1 ? '' : 's'}',
              style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12),
            )
          : null,
      trailing: const Icon(Icons.chevron_right, size: 18, color: AppTheme.onBgDim),
      onTap: onTap,
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
        style: const TextStyle(fontWeight: FontWeight.w500, fontSize: 14),
      ),
      subtitle: entry.url.isNotEmpty
          ? Text(
              entry.url,
              style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12),
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

enum _EmptyMode { vault, folder, search }

class _EmptyState extends StatelessWidget {
  final _EmptyMode mode;
  const _EmptyState({required this.mode});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            mode == _EmptyMode.search
                ? Icons.search_off
                : Icons.lock_open_outlined,
            size: 64,
            color: AppTheme.onBgDim,
          ),
          const SizedBox(height: 16),
          Text(
            switch (mode) {
              _EmptyMode.vault => 'Your vault is empty',
              _EmptyMode.folder => 'This folder is empty',
              _EmptyMode.search => 'No entries found',
            },
            style: const TextStyle(color: AppTheme.onBgDim),
          ),
          if (mode == _EmptyMode.vault) ...[
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

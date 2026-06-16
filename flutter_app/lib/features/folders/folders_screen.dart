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

final _foldersProvider = FutureProvider<List<FolderResponse>>((ref) {
  return ref.watch(apiClientProvider).listFolders();
});

class FoldersScreen extends ConsumerWidget {
  const FoldersScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final folders = ref.watch(_foldersProvider);
    return Scaffold(
      appBar: AppBar(title: const Text('> FOLDERS')),
      body: folders.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(
            child: Text(e.toString(),
                style: const TextStyle(color: AppTheme.error))),
        data: (list) {
          if (list.isEmpty) {
            return const Center(
              child: Text('No folders yet.',
                  style: TextStyle(color: AppTheme.onBgDim)),
            );
          }
          return ListView.separated(
            itemCount: list.length,
            separatorBuilder: (_, _) => const Divider(height: 1),
            itemBuilder: (ctx, i) => _FolderTile(folder: list[i]),
          );
        },
      ),
      bottomNavigationBar: _BottomNav(),
    );
  }
}

class _FolderTile extends StatelessWidget {
  final FolderResponse folder;
  const _FolderTile({required this.folder});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: const Icon(Icons.folder_outlined, color: AppTheme.green),
      title: Text(folder.name),
      subtitle:
          folder.children.isNotEmpty ? Text('${folder.children.length} subfolders') : null,
      trailing: folder.children.isNotEmpty
          ? const Icon(Icons.expand_more, color: AppTheme.onBgDim)
          : null,
      onTap: () {},
    );
  }
}

class _BottomNav extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return NavigationBar(
      backgroundColor: AppTheme.bg,
      selectedIndex: 1,
      indicatorColor: AppTheme.greenFaint,
      destinations: const [
        NavigationDestination(
          icon: Icon(Icons.lock_outline),
          selectedIcon: Icon(Icons.lock, color: AppTheme.green),
          label: 'Vault',
        ),
        NavigationDestination(
          icon: Icon(Icons.folder_outlined),
          selectedIcon: Icon(Icons.folder, color: AppTheme.green),
          label: 'Folders',
        ),
        NavigationDestination(
          icon: Icon(Icons.casino_outlined),
          selectedIcon: Icon(Icons.casino, color: AppTheme.green),
          label: 'Generate',
        ),
      ],
      onDestinationSelected: (i) {
        switch (i) {
          case 0:
            context.go('/entries');
          case 1:
            break;
          case 2:
            context.go('/generate');
        }
      },
    );
  }
}

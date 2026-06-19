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

/// Public so screens that create/revoke shares can `ref.invalidate(sharesProvider)`
/// to refresh the list immediately.
final sharesProvider = FutureProvider<MySharesResponse>((ref) {
  return ref.watch(apiClientProvider).listMyShares();
});

class SharesTab extends ConsumerWidget {
  const SharesTab({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final sharesAsync = ref.watch(sharesProvider);

    return sharesAsync.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(child: Text('Error: $e')),
      data: (shares) {
        final total = shares.shareLinks.length +
            shares.entryShares.length +
            shares.folderShares.length;
        if (total == 0) {
          return const Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.share_outlined, size: 48, color: AppTheme.onBgDim),
                SizedBox(height: 12),
                Text('No active shares', style: TextStyle(color: AppTheme.onBgDim)),
              ],
            ),
          );
        }

        return RefreshIndicator(
          color: AppTheme.green,
          onRefresh: () async => ref.invalidate(sharesProvider),
          child: ListView(
            children: [
              if (shares.shareLinks.isNotEmpty) ...[
                _SectionHeader(
                    title: 'SHARE LINKS', count: shares.shareLinks.length),
                for (final link in shares.shareLinks)
                  _ShareLinkTile(
                    link: link,
                    onRevoke: () async {
                      await ref.read(apiClientProvider).revokeShareLink(link.id);
                      ref.invalidate(sharesProvider);
                    },
                  ),
              ],
              if (shares.entryShares.isNotEmpty) ...[
                _SectionHeader(
                    title: 'ENTRY SHARES', count: shares.entryShares.length),
                for (final share in shares.entryShares)
                  _DirectShareTile(
                    share: share,
                    icon: Icons.lock_outline,
                    onRevoke: () async {
                      await ref.read(apiClientProvider).revokeEntryShare(
                          share.resourceId, share.userId);
                      ref.invalidate(sharesProvider);
                    },
                  ),
              ],
              if (shares.folderShares.isNotEmpty) ...[
                _SectionHeader(
                    title: 'FOLDER SHARES', count: shares.folderShares.length),
                for (final share in shares.folderShares)
                  _DirectShareTile(
                    share: share,
                    icon: Icons.folder_outlined,
                    onRevoke: () async {
                      await ref.read(apiClientProvider).revokeFolderShare(
                          share.resourceId, share.userId);
                      ref.invalidate(sharesProvider);
                    },
                  ),
              ],
            ],
          ),
        );
      },
    );
  }
}

class _SectionHeader extends StatelessWidget {
  final String title;
  final int count;
  const _SectionHeader({required this.title, required this.count});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 4),
      child: Row(
        children: [
          Text(title,
              style: const TextStyle(
                color: AppTheme.green,
                fontSize: 11,
                letterSpacing: 2,
                fontWeight: FontWeight.w600,
              )),
          const SizedBox(width: 8),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
            decoration: BoxDecoration(
              color: AppTheme.greenFaint,
              borderRadius: BorderRadius.circular(4),
            ),
            child: Text('$count',
                style: const TextStyle(color: AppTheme.green, fontSize: 11)),
          ),
        ],
      ),
    );
  }
}

class _ShareLinkTile extends StatelessWidget {
  final ShareLinkResponse link;
  final VoidCallback onRevoke;
  const _ShareLinkTile({required this.link, required this.onRevoke});

  @override
  Widget build(BuildContext context) {
    final isRevoked = link.revokedAt != null;
    return ListTile(
      leading: Icon(
        Icons.link,
        color: isRevoked ? AppTheme.onBgDim : AppTheme.green,
      ),
      title: Text(
        link.resourceName.isNotEmpty ? link.resourceName : 'Shared item',
        style: const TextStyle(fontSize: 14),
      ),
      subtitle: Text(
        'Expires ${link.expiresAt.length > 10 ? link.expiresAt.substring(0, 10) : link.expiresAt}'
        '${isRevoked ? ' · REVOKED' : ''}',
        style: TextStyle(color: isRevoked ? AppTheme.error : AppTheme.onBgDim, fontSize: 11),
      ),
      trailing: isRevoked
          ? null
          : IconButton(
              icon: const Icon(Icons.delete_outline, color: AppTheme.error),
              onPressed: () => _confirmRevoke(context),
            ),
    );
  }

  void _confirmRevoke(BuildContext context) {
    showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Revoke share link?'),
        content: const Text('Anyone with this link will lose access immediately.'),
        actions: [
          TextButton(onPressed: () => ctx.pop(), child: const Text('Cancel')),
          TextButton(
              onPressed: () {
                ctx.pop();
                onRevoke();
              },
              child: const Text('Revoke', style: TextStyle(color: AppTheme.error))),
        ],
      ),
    );
  }
}

class _DirectShareTile extends StatelessWidget {
  final DirectShareResponse share;
  final IconData icon;
  final VoidCallback onRevoke;
  const _DirectShareTile({
    required this.share,
    required this.icon,
    required this.onRevoke,
  });

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: Icon(icon, color: AppTheme.green),
      title: Text(share.resourceName, style: const TextStyle(fontSize: 14)),
      subtitle: Text(
        '${share.userEmail} · ${share.permission}',
        style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12),
      ),
      trailing: IconButton(
        icon: const Icon(Icons.person_remove_outlined, color: AppTheme.error),
        onPressed: () => _confirmRevoke(context),
      ),
    );
  }

  void _confirmRevoke(BuildContext context) {
    showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Revoke share?'),
        content: Text('${share.userEmail} will lose access to "${share.resourceName}".'),
        actions: [
          TextButton(onPressed: () => ctx.pop(), child: const Text('Cancel')),
          TextButton(
              onPressed: () {
                ctx.pop();
                onRevoke();
              },
              child: const Text('Revoke', style: TextStyle(color: AppTheme.error))),
        ],
      ),
    );
  }
}

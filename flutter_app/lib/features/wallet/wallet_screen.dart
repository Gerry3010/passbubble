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

// Wallet: manage the non-login entries (credit cards, identities, bank
// accounts, licenses) in one dedicated place, instead of hunting for them
// between logins in the vault list.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/api/models.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/bottom_nav.dart';
import '../../shared/widgets/prompt_title.dart';
import '../entries/entries_list_screen.dart' show entriesProvider;

/// The entry types the wallet manages, in display order:
/// (type, section label, singular label, icon).
const _walletSections = [
  ('credit-card', 'Cards', 'card', Icons.credit_card),
  ('identity', 'Identities', 'identity', Icons.person_outline),
  ('bank-account', 'Bank accounts', 'bank account', Icons.account_balance_outlined),
  ('license', 'Licenses', 'license', Icons.workspace_premium_outlined),
];

class WalletScreen extends ConsumerWidget {
  const WalletScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final entriesAsync = ref.watch(entriesProvider);

    return Scaffold(
      appBar: AppBar(title: const PromptTitle('wallet')),
      body: entriesAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(
          child: Text('$e', style: const TextStyle(color: AppTheme.error)),
        ),
        data: (entries) {
          final sections = _walletSections
              .map((s) => (
                    s,
                    entries.where((e) => e.type == s.$1).toList()
                      ..sort((a, b) =>
                          a.name.toLowerCase().compareTo(b.name.toLowerCase())),
                  ))
              .where((pair) => pair.$2.isNotEmpty)
              .toList();

          if (sections.isEmpty) {
            return Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  const Icon(Icons.wallet_outlined,
                      size: 48, color: AppTheme.onBgDim),
                  const SizedBox(height: 12),
                  const Text('No cards or identities yet',
                      style: TextStyle(color: AppTheme.onBgDim)),
                  const SizedBox(height: 4),
                  const Text(
                    'Stored cards & identities can be filled into\n'
                    'checkout and address forms by the extension.',
                    textAlign: TextAlign.center,
                    style: TextStyle(color: AppTheme.onBgDim, fontSize: 12),
                  ),
                ],
              ),
            );
          }

          return RefreshIndicator(
            onRefresh: () async => ref.invalidate(entriesProvider),
            child: ListView(
              padding: const EdgeInsets.all(12),
              children: [
                for (final (section, items) in sections) ...[
                  Padding(
                    padding: const EdgeInsets.fromLTRB(4, 12, 4, 6),
                    child: Text(
                      '# ${section.$2.toLowerCase()} (${items.length})',
                      style: const TextStyle(
                        color: AppTheme.green,
                        fontWeight: FontWeight.bold,
                        fontSize: 13,
                      ),
                    ),
                  ),
                  for (final entry in items)
                    _WalletTile(entry: entry, icon: section.$4),
                ],
              ],
            ),
          );
        },
      ),
      floatingActionButton: FloatingActionButton(
        backgroundColor: AppTheme.green,
        foregroundColor: AppTheme.bg,
        onPressed: () => _showCreateMenu(context),
        child: const Icon(Icons.add),
      ),
      bottomNavigationBar: const PbBottomNav(currentIndex: 1),
    );
  }

  void _showCreateMenu(BuildContext context) {
    showModalBottomSheet<void>(
      context: context,
      backgroundColor: AppTheme.bg,
      builder: (sheetCtx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            for (final (type, _, singular, icon) in _walletSections)
              ListTile(
                leading: Icon(icon, color: AppTheme.green),
                title: Text('New $singular'),
                onTap: () {
                  Navigator.of(sheetCtx).pop();
                  context.go('/entries/new?type=$type');
                },
              ),
          ],
        ),
      ),
    );
  }
}

class _WalletTile extends StatelessWidget {
  final EntryResponse entry;
  final IconData icon;
  const _WalletTile({required this.entry, required this.icon});

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(bottom: 6),
      decoration: BoxDecoration(
        color: AppTheme.surface,
        border: Border.all(color: AppTheme.border),
        borderRadius: BorderRadius.circular(6),
      ),
      child: ListTile(
        dense: true,
        leading: Icon(icon, color: AppTheme.green),
        title: Text(entry.name,
            maxLines: 1, overflow: TextOverflow.ellipsis),
        trailing: IconButton(
          icon: const Icon(Icons.edit_outlined, size: 18, color: AppTheme.onBgDim),
          tooltip: 'Edit',
          onPressed: () => context.go('/entries/${entry.id}/edit'),
        ),
        onTap: () => context.go('/entries/${entry.id}'),
      ),
    );
  }
}

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
import 'package:go_router/go_router.dart';

import '../../core/api/api_client.dart';
import '../../core/api/models.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_button.dart';

final _usersProvider = FutureProvider<List<UserResponse>>((ref) {
  return ref.watch(apiClientProvider).adminListUsers();
});

final _invitationsProvider = FutureProvider<List<InvitationResponse>>((ref) {
  return ref.watch(apiClientProvider).adminListInvitations();
});

class AdminScreen extends ConsumerStatefulWidget {
  const AdminScreen({super.key});

  @override
  ConsumerState<AdminScreen> createState() => _AdminScreenState();
}

class _AdminScreenState extends ConsumerState<AdminScreen>
    with SingleTickerProviderStateMixin {
  late final TabController _tabs;

  @override
  void initState() {
    super.initState();
    _tabs = TabController(length: 2, vsync: this);
  }

  @override
  void dispose() {
    _tabs.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('> ADMIN'),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => context.go('/entries'),
        ),
        bottom: TabBar(
          controller: _tabs,
          labelColor: AppTheme.green,
          indicatorColor: AppTheme.green,
          unselectedLabelColor: AppTheme.onBgDim,
          tabs: const [
            Tab(text: 'USERS'),
            Tab(text: 'INVITATIONS'),
          ],
        ),
      ),
      body: TabBarView(
        controller: _tabs,
        children: [
          _UsersTab(),
          _InvitationsTab(),
        ],
      ),
    );
  }
}

class _UsersTab extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final users = ref.watch(_usersProvider);
    return users.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) =>
          Center(child: Text(e.toString(), style: const TextStyle(color: AppTheme.error))),
      data: (list) => ListView.separated(
        itemCount: list.length,
        separatorBuilder: (_, _) => const Divider(height: 1),
        itemBuilder: (ctx, i) => _UserTile(user: list[i]),
      ),
    );
  }
}

class _UserTile extends StatelessWidget {
  final UserResponse user;
  const _UserTile({required this.user});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: CircleAvatar(
        backgroundColor: AppTheme.surfaceVariant,
        child: Text(
          user.name.isNotEmpty ? user.name[0].toUpperCase() : '?',
          style: const TextStyle(color: AppTheme.green),
        ),
      ),
      title: Text(user.name),
      subtitle: Text(user.email, style: const TextStyle(fontSize: 12)),
      trailing: Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        decoration: BoxDecoration(
          border: Border.all(
            color: user.role == 'admin' ? AppTheme.green : AppTheme.border,
          ),
        ),
        child: Text(
          user.role.toUpperCase(),
          style: TextStyle(
            fontSize: 11,
            color: user.role == 'admin' ? AppTheme.green : AppTheme.onBgDim,
          ),
        ),
      ),
    );
  }
}

class _InvitationsTab extends ConsumerStatefulWidget {
  @override
  ConsumerState<_InvitationsTab> createState() => _InvitationsTabState();
}

class _InvitationsTabState extends ConsumerState<_InvitationsTab> {
  final _emailCtrl = TextEditingController();
  bool _sending = false;
  String? _lastToken;

  @override
  void dispose() {
    _emailCtrl.dispose();
    super.dispose();
  }

  Future<void> _invite() async {
    final email = _emailCtrl.text.trim();
    if (email.isEmpty) return;
    setState(() { _sending = true; _lastToken = null; });
    try {
      final inv = await ref.read(apiClientProvider).adminInvite(email);
      ref.invalidate(_invitationsProvider);
      setState(() {
        _lastToken = inv.token;
        _emailCtrl.clear();
      });
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(e.toString())));
      }
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final invitations = ref.watch(_invitationsProvider);
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _emailCtrl,
                  keyboardType: TextInputType.emailAddress,
                  decoration: const InputDecoration(
                    labelText: 'Invite by email',
                    prefixIcon: Icon(Icons.email_outlined),
                  ),
                ),
              ),
              const SizedBox(width: 12),
              PbButton(
                label: 'Invite',
                onPressed: _sending ? null : _invite,
                loading: _sending,
              ),
            ],
          ),
        ),
        if (_lastToken != null)
          Container(
            margin: const EdgeInsets.symmetric(horizontal: 16),
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              border: Border.all(color: AppTheme.green),
              color: AppTheme.greenFaint,
            ),
            child: Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const Text('Invitation token:',
                          style: TextStyle(color: AppTheme.onBgDim, fontSize: 11)),
                      Text(
                        _lastToken!,
                        style: const TextStyle(
                            color: AppTheme.green, fontFamily: 'monospace'),
                      ),
                    ],
                  ),
                ),
                IconButton(
                  icon: const Icon(Icons.copy, color: AppTheme.green),
                  onPressed: () =>
                      Clipboard.setData(ClipboardData(text: _lastToken!)),
                ),
              ],
            ),
          ),
        const Divider(height: 24),
        Expanded(
          child: invitations.when(
            loading: () => const Center(child: CircularProgressIndicator()),
            error: (e, _) => Center(child: Text(e.toString())),
            data: (list) => ListView.separated(
              itemCount: list.length,
              separatorBuilder: (_, _) => const Divider(height: 1),
              itemBuilder: (ctx, i) {
                final inv = list[i];
                return ListTile(
                  title: Text(inv.email),
                  subtitle: Text(
                    inv.used ? 'Used' : 'Pending',
                    style: TextStyle(
                      color: inv.used ? AppTheme.onBgDim : AppTheme.green,
                      fontSize: 12,
                    ),
                  ),
                  trailing: !inv.used
                      ? IconButton(
                          icon: const Icon(Icons.copy, size: 18),
                          onPressed: () =>
                              Clipboard.setData(ClipboardData(text: inv.token)),
                        )
                      : null,
                );
              },
            ),
          ),
        ),
      ],
    );
  }
}

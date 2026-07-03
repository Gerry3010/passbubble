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

import '../../core/auth/auth_service.dart';
import '../../core/auth/auto_lock_provider.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/bottom_nav.dart';
import '../../shared/widgets/pb_button.dart';
import '../../shared/widgets/prompt_title.dart';
import 'autofill_settings_tile.dart';
import 'biometric_settings_tile.dart';
import 'pin_settings_tile.dart';
import 'post_quantum_tile.dart';
import 'providers/version_provider.dart';
import 'two_factor_screen.dart';

class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final auth = ref.watch(authStateProvider);

    return Scaffold(
      appBar: AppBar(
        title: const PromptTitle('settings'),
      ),
      body: ListView(
        children: [
          // Account
          _SectionHeader(title: 'ACCOUNT'),
          ListTile(
            leading: const Icon(Icons.person_outline),
            title: Text(auth.name ?? ''),
            subtitle: Text(auth.email ?? ''),
          ),
          ListTile(
            leading: Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
              decoration: BoxDecoration(
                border: Border.all(color: AppTheme.green),
              ),
              child: Text(
                (auth.role ?? 'user').toUpperCase(),
                style: const TextStyle(
                    color: AppTheme.green, fontSize: 11),
              ),
            ),
            title: const Text('Role'),
          ),

          const Divider(),

          // Security
          _SectionHeader(title: 'SECURITY'),
          ListTile(
            leading: const Icon(Icons.lock_outline),
            title: const Text('Lock vault'),
            subtitle: const Text('Clears private keys from memory'),
            onTap: () {
              ref.read(authStateProvider.notifier).lock();
              // Router redirects to the unlock screen automatically.
            },
          ),
          ListTile(
            leading: const Icon(Icons.timer_outlined),
            title: const Text('Auto-lock'),
            subtitle: Text(
              ref.watch(autoLockProvider) <= 0
                  ? 'Disabled — vault stays unlocked'
                  : 'Lock after ${autoLockLabel(ref.watch(autoLockProvider))} of inactivity',
            ),
            trailing: Text(
              autoLockLabel(ref.watch(autoLockProvider)),
              style: const TextStyle(color: AppTheme.green),
            ),
            onTap: () => _showAutoLockPicker(context, ref),
          ),
          const PinSettingsTile(),
          const PostQuantumTile(),
          ListTile(
            leading: const Icon(Icons.shield_outlined),
            title: const Text('Two-factor authentication'),
            subtitle: const Text('Require a TOTP code at login'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => Navigator.of(context).push(
              MaterialPageRoute(builder: (_) => const TwoFactorScreen()),
            ),
          ),
          const BiometricSettingsTile(),
          const AutofillSettingsTile(),

          const Divider(),

          // Vault
          _SectionHeader(title: 'VAULT'),
          ListTile(
            leading: const Icon(Icons.delete_outline),
            title: const Text('Trash'),
            subtitle: const Text('Deleted entries — restorable for 30 days'),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => context.go('/trash'),
          ),

          const Divider(),

          // Server
          _SectionHeader(title: 'SERVER'),
          ListTile(
            leading: const Icon(Icons.dns_outlined),
            title: const Text('Change server'),
            onTap: () => context.go('/setup'),
          ),

          const Divider(),

          // Version & Updates
          _SectionHeader(title: 'VERSION & UPDATES'),
          Consumer(builder: (context, ref, _) {
            final versionAsync = ref.watch(versionInfoProvider);
            final badge = versionAsync.whenOrNull(
              data: (info) => info.isUpToDate ? null : const Icon(Icons.circle, color: Colors.amber, size: 10),
            );
            return ListTile(
              leading: const Icon(Icons.system_update_outlined),
              title: const Text('Check for updates'),
              subtitle: versionAsync.when(
                loading: () => const Text('Checking…'),
                error: (_, _) => const Text('Tap to check'),
                data: (info) => Text('Server: ${info.serverVersion}'),
              ),
              trailing: badge,
              onTap: () => context.push('/settings/update'),
            );
          }),

          const Divider(),

          // Danger zone
          _SectionHeader(title: 'DANGER ZONE'),
          Padding(
            padding: const EdgeInsets.all(16),
            child: PbButton(
              label: 'Sign Out',
              onPressed: () async {
                final ok = await showDialog<bool>(
                  context: context,
                  builder: (ctx) => AlertDialog(
                    title: const Text('Sign out?'),
                    content: const Text(
                        'Your encrypted keys will be removed from this device.'),
                    actions: [
                      TextButton(
                        onPressed: () => ctx.pop(false),
                        child: const Text('Cancel'),
                      ),
                      TextButton(
                        onPressed: () => ctx.pop(true),
                        child: const Text('Sign Out',
                            style: TextStyle(color: AppTheme.error)),
                      ),
                    ],
                  ),
                );
                if (ok == true) {
                  await ref.read(authStateProvider.notifier).logout();
                }
              },
              icon: Icons.logout,
              outlined: true,
            ),
          ),
        ],
      ),
      bottomNavigationBar: const PbBottomNav(currentIndex: 4),
    );
  }

  Future<void> _showAutoLockPicker(BuildContext context, WidgetRef ref) async {
    final current = ref.read(autoLockProvider);
    final choice = await showDialog<int>(
      context: context,
      builder: (ctx) => SimpleDialog(
        title: const Text('Auto-lock after'),
        children: [
          for (final minutes in autoLockPresets)
            ListTile(
              title: Text(autoLockLabel(minutes)),
              trailing: minutes == current
                  ? const Icon(Icons.check, color: AppTheme.green)
                  : null,
              onTap: () => ctx.pop(minutes),
            ),
        ],
      ),
    );
    if (choice != null) {
      await ref.read(autoLockProvider.notifier).set(choice);
    }
  }
}

class _SectionHeader extends StatelessWidget {
  final String title;
  const _SectionHeader({required this.title});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 4),
      child: Text(
        title,
        style: const TextStyle(
          color: AppTheme.green,
          fontSize: 11,
          letterSpacing: 2,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }
}

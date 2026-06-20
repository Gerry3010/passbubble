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

import '../../core/auth/auth_service.dart';
import '../entries/entries_list_screen.dart' show entriesProvider;

/// Settings tile offering the one-time post-quantum (hybrid) key upgrade. Only
/// visible while the account is still X25519-only; it disappears once upgraded.
class PostQuantumTile extends ConsumerStatefulWidget {
  const PostQuantumTile({super.key});

  @override
  ConsumerState<PostQuantumTile> createState() => _PostQuantumTileState();
}

class _PostQuantumTileState extends ConsumerState<PostQuantumTile> {
  bool _needed = false;
  bool _busy = false;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    final needed = await ref.read(authServiceProvider).needsKeyUpgrade();
    if (mounted) setState(() => _needed = needed);
  }

  Future<void> _upgrade() async {
    final password = await _promptPassword();
    if (password == null || password.isEmpty) return;
    setState(() => _busy = true);
    try {
      final res = await ref.read(authServiceProvider).upgradeToHybrid(password);
      ref.invalidate(entriesProvider); // entry keys changed
      if (!mounted) return;
      setState(() => _needed = false);
      final msg = res.failed == 0
          ? 'Post-quantum enabled — ${res.rewrapped} entries upgraded.'
          : 'Post-quantum enabled — ${res.rewrapped} upgraded, ${res.failed} skipped (still readable).';
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Upgrade failed: $e')),
      );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<String?> _promptPassword() {
    final ctrl = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Enable post-quantum encryption'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Adds ML-KEM-768 (post-quantum) protection and re-encrypts your '
              'entries. Enter your master password to continue.',
            ),
            const SizedBox(height: 12),
            TextField(
              controller: ctrl,
              obscureText: true,
              autofocus: true,
              decoration: const InputDecoration(labelText: 'Master password'),
              onSubmitted: (v) => Navigator.of(ctx).pop(v),
            ),
          ],
        ),
        actions: [
          TextButton(onPressed: () => Navigator.of(ctx).pop(), child: const Text('Cancel')),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(ctrl.text),
            child: const Text('Upgrade'),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    if (!_needed) return const SizedBox.shrink();
    return ListTile(
      leading: _busy
          ? const SizedBox(
              width: 24, height: 24, child: CircularProgressIndicator(strokeWidth: 2))
          : const Icon(Icons.enhanced_encryption_outlined),
      title: const Text('Enable post-quantum encryption'),
      subtitle: const Text('Upgrade this account to ML-KEM-768 hybrid encryption'),
      trailing: const Icon(Icons.chevron_right),
      onTap: _busy ? null : _upgrade,
    );
  }
}

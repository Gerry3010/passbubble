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

import '../../core/api/api_client.dart';
import '../../core/api/models.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/prompt_title.dart';

/// Manages account-level two-factor authentication (TOTP for login).
class TwoFactorScreen extends ConsumerStatefulWidget {
  const TwoFactorScreen({super.key});

  @override
  ConsumerState<TwoFactorScreen> createState() => _TwoFactorScreenState();
}

class _TwoFactorScreenState extends ConsumerState<TwoFactorScreen> {
  bool _loading = true;
  bool _enabled = false;
  String? _error;

  // Enrollment in progress (setup returned a secret to confirm).
  SetupTotpResponse? _setup;
  final _codeCtrl = TextEditingController();

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  @override
  void dispose() {
    _codeCtrl.dispose();
    super.dispose();
  }

  Future<void> _refresh() async {
    setState(() { _loading = true; _error = null; });
    try {
      final me = await ref.read(apiClientProvider).me();
      setState(() => _enabled = me.totpEnabled);
    } catch (e) {
      setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _beginSetup() async {
    setState(() { _loading = true; _error = null; });
    try {
      final setup = await ref.read(apiClientProvider).setupTotp();
      setState(() => _setup = setup);
    } catch (e) {
      setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _confirm() async {
    final code = _codeCtrl.text.trim();
    if (code.isEmpty || _setup == null) return;
    setState(() { _loading = true; _error = null; });
    try {
      await ref.read(apiClientProvider).confirmTotp(_setup!.secret, code);
      _codeCtrl.clear();
      setState(() { _setup = null; _enabled = true; });
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Two-factor authentication enabled')),
        );
      }
    } catch (e) {
      setState(() => _error = 'Could not enable: $e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _disable() async {
    final codeCtrl = TextEditingController();
    final passCtrl = TextEditingController();
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Disable 2FA'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text('Confirm with a current code or your password.'),
            const SizedBox(height: 12),
            TextField(
              controller: codeCtrl,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(labelText: '6-digit code'),
            ),
            TextField(
              controller: passCtrl,
              obscureText: true,
              decoration: const InputDecoration(labelText: 'or password'),
            ),
          ],
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
          TextButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Disable')),
        ],
      ),
    );
    if (ok != true) return;
    setState(() { _loading = true; _error = null; });
    try {
      await ref.read(apiClientProvider).disableTotp(
            code: codeCtrl.text.trim(),
            password: passCtrl.text,
          );
      setState(() => _enabled = false);
    } catch (e) {
      setState(() => _error = 'Could not disable: $e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const PromptTitle('2fa')),
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : ListView(
              padding: const EdgeInsets.all(16),
              children: [
                if (_error != null) ...[
                  Text(_error!, style: const TextStyle(color: AppTheme.error)),
                  const SizedBox(height: 16),
                ],
                if (_setup != null)
                  ..._buildEnrollment()
                else if (_enabled)
                  ..._buildEnabled()
                else
                  ..._buildDisabled(),
              ],
            ),
    );
  }

  List<Widget> _buildDisabled() => [
        const Text(
          'Protect your account login with a time-based one-time code from an '
          'authenticator app (e.g. Aegis, Google Authenticator).',
        ),
        const SizedBox(height: 16),
        ElevatedButton.icon(
          onPressed: _beginSetup,
          icon: const Icon(Icons.shield_outlined),
          label: const Text('Enable 2FA'),
        ),
      ];

  List<Widget> _buildEnrollment() => [
        const Text('1. Add this secret to your authenticator app:'),
        const SizedBox(height: 8),
        Card(
          child: ListTile(
            title: SelectableText(_setup!.secret),
            trailing: IconButton(
              icon: const Icon(Icons.copy),
              onPressed: () {
                Clipboard.setData(ClipboardData(text: _setup!.secret));
                ScaffoldMessenger.of(context).showSnackBar(
                  const SnackBar(content: Text('Secret copied')),
                );
              },
            ),
          ),
        ),
        const SizedBox(height: 16),
        const Text('2. Enter the 6-digit code it shows:'),
        const SizedBox(height: 8),
        TextField(
          controller: _codeCtrl,
          keyboardType: TextInputType.number,
          decoration: const InputDecoration(labelText: '6-digit code'),
        ),
        const SizedBox(height: 16),
        Row(
          children: [
            TextButton(
              onPressed: () => setState(() => _setup = null),
              child: const Text('Cancel'),
            ),
            const Spacer(),
            ElevatedButton(onPressed: _confirm, child: const Text('Confirm')),
          ],
        ),
      ];

  List<Widget> _buildEnabled() => [
        const Row(
          children: [
            Icon(Icons.verified_user, color: AppTheme.green),
            SizedBox(width: 8),
            Text('Two-factor authentication is ON'),
          ],
        ),
        const SizedBox(height: 16),
        OutlinedButton.icon(
          onPressed: _disable,
          icon: const Icon(Icons.lock_open_outlined, color: AppTheme.error),
          label: const Text('Disable 2FA', style: TextStyle(color: AppTheme.error)),
        ),
      ];
}

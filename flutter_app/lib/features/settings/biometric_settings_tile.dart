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
import 'package:local_auth/local_auth.dart';

import '../../core/auth/auth_service.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_text_field.dart';

class BiometricSettingsTile extends ConsumerStatefulWidget {
  const BiometricSettingsTile({super.key});

  @override
  ConsumerState<BiometricSettingsTile> createState() =>
      _BiometricSettingsTileState();
}

class _BiometricSettingsTileState
    extends ConsumerState<BiometricSettingsTile> {
  final _localAuth = LocalAuthentication();
  bool _bioSupported = false;
  bool _bioEnabled = false;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    bool supported = false;
    try {
      supported = await _localAuth.canCheckBiometrics ||
          await _localAuth.isDeviceSupported();
    } catch (_) {}
    final enabled =
        await ref.read(authStateProvider.notifier).svc.hasBiometricMasterPassword();
    if (mounted) {
      setState(() {
        _bioSupported = supported;
        _bioEnabled = enabled;
        _loading = false;
      });
    }
  }

  Future<void> _toggle(bool value) async {
    if (value) {
      await _enableBiometric();
    } else {
      await ref.read(authStateProvider.notifier).svc.disableBiometric();
      if (mounted) setState(() => _bioEnabled = false);
    }
  }

  Future<void> _enableBiometric() async {
    final passCtrl = TextEditingController();
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: AppTheme.surface,
        title: const Text('Enable biometric unlock',
            style: TextStyle(color: AppTheme.green)),
        content: PbTextField(
          label: 'Master password',
          controller: passCtrl,
          obscureText: true,
          prefixIcon: Icons.key,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('Confirm', style: TextStyle(color: AppTheme.green)),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;

    final password = passCtrl.text;
    passCtrl.dispose();
    if (password.isEmpty) return;

    // Verify the password is correct before caching it.
    final ok = await ref.read(authStateProvider.notifier).svc.unlock(password);
    if (!mounted) return;
    if (!ok) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Wrong master password')),
      );
      return;
    }

    await ref
        .read(authStateProvider.notifier)
        .svc
        .saveBiometricMasterPassword(password);
    if (mounted) setState(() => _bioEnabled = true);
  }

  @override
  Widget build(BuildContext context) {
    if (_loading || !_bioSupported) return const SizedBox.shrink();
    return ListTile(
      leading: const Icon(Icons.fingerprint),
      title: const Text('Biometric unlock'),
      subtitle: const Text('Use biometrics instead of master password'),
      trailing: Switch(
        value: _bioEnabled,
        onChanged: _toggle,
        activeThumbColor: AppTheme.green,
      ),
    );
  }
}

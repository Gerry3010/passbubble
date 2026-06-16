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
import '../../shared/widgets/pb_button.dart';
import '../../shared/widgets/pb_text_field.dart';

/// Unlock screen: decrypt private keys with master password (or biometrics if cached).
class UnlockScreen extends ConsumerStatefulWidget {
  const UnlockScreen({super.key});

  @override
  ConsumerState<UnlockScreen> createState() => _UnlockScreenState();
}

class _UnlockScreenState extends ConsumerState<UnlockScreen> {
  final _passCtrl = TextEditingController();
  bool _loading = false;
  bool _obscure = true;
  String? _error;

  final _localAuth = LocalAuthentication();

  @override
  void initState() {
    super.initState();
    _tryBiometric();
  }

  Future<void> _tryBiometric() async {
    try {
      final canBio = await _localAuth.canCheckBiometrics;
      if (!canBio) return;
      final ok = await _localAuth.authenticate(
        localizedReason: 'Unlock your vault',
        options: const AuthenticationOptions(biometricOnly: true),
      );
      if (ok && mounted) {
        // Biometric unlocked — signal success (master password was cached via secure storage)
        // For now, prompt for password after biometric success
        // TODO: cache derived key in secure storage, protected by biometric
      }
    } catch (_) {}
  }

  Future<void> _unlock() async {
    if (_passCtrl.text.isEmpty) {
      setState(() => _error = 'Enter your master password');
      return;
    }
    setState(() { _loading = true; _error = null; });
    try {
      final ok = await ref
          .read(authStateProvider.notifier)
          .unlock(_passCtrl.text);
      if (!ok && mounted) {
        setState(() => _error = 'Wrong master password');
      }
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 420),
          child: Padding(
            padding: const EdgeInsets.all(32),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Icon(Icons.lock_outline, color: AppTheme.green, size: 48),
                const SizedBox(height: 24),
                Text(
                  '> UNLOCK VAULT',
                  style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                        color: AppTheme.green,
                        letterSpacing: 2,
                      ),
                ),
                const SizedBox(height: 32),
                PbTextField(
                  label: 'Master Password',
                  controller: _passCtrl,
                  obscureText: _obscure,
                  prefixIcon: Icons.key,
                  suffixIcon: IconButton(
                    icon: Icon(
                      _obscure ? Icons.visibility_off : Icons.visibility,
                      color: AppTheme.onBgDim,
                    ),
                    onPressed: () => setState(() => _obscure = !_obscure),
                  ),
                ),
                if (_error != null) ...[
                  const SizedBox(height: 8),
                  Text(_error!, style: const TextStyle(color: AppTheme.error)),
                ],
                const SizedBox(height: 24),
                SizedBox(
                  width: double.infinity,
                  child: PbButton(
                    label: 'Unlock',
                    onPressed: _loading ? null : _unlock,
                    loading: _loading,
                    icon: Icons.lock_open,
                  ),
                ),
                const SizedBox(height: 12),
                Center(
                  child: TextButton.icon(
                    onPressed: _tryBiometric,
                    icon: const Icon(Icons.fingerprint, color: AppTheme.green),
                    label: const Text('Use Biometrics'),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

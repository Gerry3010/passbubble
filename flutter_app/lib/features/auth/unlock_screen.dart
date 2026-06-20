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
import 'package:local_auth_android/local_auth_android.dart';

import '../../core/auth/auth_service.dart';
import '../../core/crypto/pin_crypto.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_button.dart';
import '../../shared/widgets/pb_text_field.dart';
import '../../widgets/app_logo.dart';

/// Unlock screen: decrypt private keys with master password, PIN, or biometrics.
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

  bool _pinEnabled = false;
  bool _pinMode = false;
  bool _bioAvailable = false;

  final _localAuth = LocalAuthentication();

  @override
  void initState() {
    super.initState();
    _checkCapabilities();
  }

  Future<void> _checkCapabilities() async {
    final status = await ref.read(authStateProvider.notifier).pinStatus();
    bool bioAvailable = false;
    try {
      bioAvailable = await _localAuth.canCheckBiometrics &&
          await _localAuth.isDeviceSupported();
    } catch (_) {}

    if (!mounted) return;
    setState(() {
      _pinEnabled = status.enabled && !status.expired;
      _pinMode = _pinEnabled;
      _bioAvailable = bioAvailable;
    });

    // Auto-trigger biometrics on open when available and master password is cached.
    if (bioAvailable) {
      final hasCached = await ref
          .read(authStateProvider.notifier)
          .svc
          .hasBiometricMasterPassword();
      if (hasCached && mounted) _tryBiometric();
    }
  }

  void _toggleMode() {
    setState(() {
      _pinMode = !_pinMode;
      _passCtrl.clear();
      _error = null;
    });
  }

  Future<void> _logout() async {
    setState(() => _loading = true);
    try {
      await ref.read(authStateProvider.notifier).logout();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _tryBiometric() async {
    setState(() { _loading = true; _error = null; });
    try {
      final ok = await _localAuth.authenticate(
        localizedReason: 'Unlock your Passbubble vault',
        authMessages: const [
          AndroidAuthMessages(cancelButton: 'Cancel'),
        ],
      );
      if (!ok || !mounted) {
        setState(() => _loading = false);
        return;
      }

      final svc = ref.read(authStateProvider.notifier).svc;
      final cached = await svc.loadBiometricMasterPassword();
      if (cached == null) {
        if (mounted) {
          setState(() {
            _loading = false;
            _error = 'Unlock once with your master password to enable biometrics';
          });
        }
        return;
      }

      final unlocked = await ref.read(authStateProvider.notifier).unlock(cached);
      if (!unlocked && mounted) {
        setState(() {
          _loading = false;
          _error = 'Biometric unlock failed — use your master password';
        });
      }
    } catch (e) {
      if (mounted) setState(() { _loading = false; _error = e.toString(); });
    }
  }

  Future<void> _unlock() async {
    if (_passCtrl.text.isEmpty) {
      setState(() => _error =
          _pinMode ? 'Enter your PIN' : 'Enter your master password');
      return;
    }
    setState(() { _loading = true; _error = null; });
    try {
      if (_pinMode) {
        await _unlockWithPin();
      } else {
        final password = _passCtrl.text;
        final ok = await ref.read(authStateProvider.notifier).unlock(password);
        if (!ok && mounted) {
          setState(() => _error = 'Wrong master password');
          return;
        }
        // Cache password so biometrics can use it on future unlocks.
        if (ok && _bioAvailable && mounted) {
          await ref
              .read(authStateProvider.notifier)
              .svc
              .saveBiometricMasterPassword(password);
        }
      }
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _unlockWithPin() async {
    final result =
        await ref.read(authStateProvider.notifier).unlockWithPin(_passCtrl.text);
    if (!mounted) return;
    switch (result.status) {
      case PinUnlockStatus.ok:
        break;
      case PinUnlockStatus.wrongPin:
        setState(() => _error =
            'Incorrect PIN — ${result.triesRemaining} attempt(s) left');
        _passCtrl.clear();
      case PinUnlockStatus.expired:
        setState(() {
          _pinEnabled = false;
          _pinMode = false;
          _passCtrl.clear();
          _error = 'PIN expired — enter your master password';
        });
      case PinUnlockStatus.lockedOut:
        setState(() {
          _pinEnabled = false;
          _pinMode = false;
          _passCtrl.clear();
          _error = 'Too many attempts — PIN removed. Enter your master password.';
        });
      case PinUnlockStatus.notEnabled:
        setState(() {
          _pinEnabled = false;
          _pinMode = false;
        });
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
                const AppLogo(size: 56),
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
                  label: _pinMode ? 'PIN' : 'Master Password',
                  controller: _passCtrl,
                  obscureText: _obscure,
                  keyboardType:
                      _pinMode ? TextInputType.number : TextInputType.text,
                  prefixIcon: _pinMode ? Icons.pin : Icons.key,
                  suffixIcon: IconButton(
                    icon: Icon(
                      _obscure ? Icons.visibility_off : Icons.visibility,
                      color: AppTheme.onBgDim,
                    ),
                    onPressed: () => setState(() => _obscure = !_obscure),
                  ),
                  onSubmitted: (_) => _unlock(),
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
                if (_pinEnabled) ...[
                  const SizedBox(height: 12),
                  Center(
                    child: TextButton.icon(
                      onPressed: _toggleMode,
                      icon: Icon(_pinMode ? Icons.key : Icons.pin,
                          color: AppTheme.green),
                      label: Text(_pinMode
                          ? 'Use master password'
                          : 'Use PIN'),
                    ),
                  ),
                ],
                if (_bioAvailable) ...[
                  const SizedBox(height: 12),
                  Center(
                    child: TextButton.icon(
                      onPressed: _loading ? null : _tryBiometric,
                      icon: const Icon(Icons.fingerprint, color: AppTheme.green),
                      label: const Text('Use Biometrics'),
                    ),
                  ),
                ],
                const SizedBox(height: 4),
                Center(
                  child: TextButton.icon(
                    onPressed: _loading ? null : _logout,
                    icon: const Icon(Icons.logout, color: AppTheme.onBgDim),
                    label: const Text('Log out',
                        style: TextStyle(color: AppTheme.onBgDim)),
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

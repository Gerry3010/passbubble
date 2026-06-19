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
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_button.dart';
import '../../shared/widgets/pb_text_field.dart';
import '../../widgets/app_logo.dart';

class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key});

  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final _form = GlobalKey<FormState>();
  final _emailCtrl = TextEditingController();
  final _passCtrl = TextEditingController();
  final _totpCtrl = TextEditingController();
  final _passFocus = FocusNode();
  bool _loading = false;
  bool _obscure = true;
  String? _error;

  /// Non-null once the password step returned "2fa_required"; holds the
  /// short-lived pending token used to complete the second step.
  String? _pendingToken;

  @override
  void dispose() {
    _emailCtrl.dispose();
    _passCtrl.dispose();
    _totpCtrl.dispose();
    _passFocus.dispose();
    super.dispose();
  }

  Future<void> _login() async {
    if (!_form.currentState!.validate()) return;
    setState(() { _loading = true; _error = null; });
    try {
      final pending = await ref.read(authStateProvider.notifier).login(
            _emailCtrl.text.trim(),
            _passCtrl.text,
          );
      if (pending != null && mounted) {
        setState(() => _pendingToken = pending);
      }
    } catch (e) {
      setState(() => _error = e is Exception
          ? e.toString().replaceFirst('Exception: ', '')
          : 'An unexpected error occurred.');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _verifyTotp() async {
    final code = _totpCtrl.text.trim();
    if (code.isEmpty) return;
    setState(() { _loading = true; _error = null; });
    try {
      await ref.read(authStateProvider.notifier).verifyTotp(
            _pendingToken!,
            code,
            _passCtrl.text,
          );
    } catch (e) {
      setState(() => _error = e is Exception
          ? e.toString().replaceFirst('Exception: ', '')
          : 'An unexpected error occurred.');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _recover() async {
    setState(() { _loading = true; _error = null; });
    try {
      await ref
          .read(authStateProvider.notifier)
          .requestTotpRecovery(_pendingToken!);
      if (mounted) {
        setState(() {
          _pendingToken = null;
          _error = null;
        });
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(
          content: Text(
              'If the account exists, a reset link has been emailed. Open it, then sign in again.'),
        ));
      }
    } catch (e) {
      setState(() => _error = e is Exception
          ? e.toString().replaceFirst('Exception: ', '')
          : 'An unexpected error occurred.');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final infoMessage = GoRouterState.of(context).extra as String?;
    return Scaffold(
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 420),
          child: Padding(
            padding: const EdgeInsets.all(32),
            child: Form(
              key: _form,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const AppLogo(size: 64),
                  const SizedBox(height: 24),
                  Text(
                    '> LOGIN',
                    style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                          color: AppTheme.green,
                          letterSpacing: 3,
                        ),
                  ),
                  if (infoMessage != null) ...[
                    const SizedBox(height: 20),
                    Container(
                      padding: const EdgeInsets.all(12),
                      decoration: BoxDecoration(
                        border: Border.all(color: AppTheme.green),
                        color: AppTheme.green.withAlpha(25),
                      ),
                      child: Row(
                        children: [
                          const Icon(Icons.mark_email_unread_outlined,
                              color: AppTheme.green, size: 18),
                          const SizedBox(width: 8),
                          Expanded(
                            child: Text(
                              infoMessage,
                              style: const TextStyle(
                                  color: AppTheme.green, fontSize: 13),
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                  const SizedBox(height: 32),
                  if (_pendingToken == null) ...[
                    PbTextField(
                      label: 'Email',
                      controller: _emailCtrl,
                      keyboardType: TextInputType.emailAddress,
                      prefixIcon: Icons.alternate_email,
                      validator: (v) => v!.isEmpty ? 'Required' : null,
                      textInputAction: TextInputAction.next,
                      onSubmitted: (_) => _passFocus.requestFocus(),
                    ),
                    const SizedBox(height: 16),
                    PbTextField(
                      label: 'Master Password',
                      controller: _passCtrl,
                      focusNode: _passFocus,
                      obscureText: _obscure,
                      prefixIcon: Icons.lock_outline,
                      suffixIcon: IconButton(
                        icon: Icon(
                          _obscure ? Icons.visibility_off : Icons.visibility,
                          color: AppTheme.onBgDim,
                        ),
                        onPressed: () => setState(() => _obscure = !_obscure),
                      ),
                      validator: (v) => v!.isEmpty ? 'Required' : null,
                      onSubmitted: (_) => _login(),
                    ),
                  ] else ...[
                    Text(
                      'Enter the 6-digit code from your authenticator app.',
                      style: const TextStyle(color: AppTheme.onBgDim, fontSize: 13),
                    ),
                    const SizedBox(height: 16),
                    PbTextField(
                      label: '2FA Code',
                      controller: _totpCtrl,
                      keyboardType: TextInputType.number,
                      prefixIcon: Icons.pin_outlined,
                      onSubmitted: (_) => _verifyTotp(),
                    ),
                  ],
                  if (_error != null) ...[
                    const SizedBox(height: 12),
                    Container(
                      padding: const EdgeInsets.all(12),
                      decoration: BoxDecoration(
                        border: Border.all(color: AppTheme.error),
                        color: AppTheme.error.withAlpha(25),
                      ),
                      child: Text(
                        _error!,
                        style: const TextStyle(color: AppTheme.error, fontSize: 13),
                      ),
                    ),
                  ],
                  const SizedBox(height: 24),
                  if (_pendingToken == null) ...[
                    SizedBox(
                      width: double.infinity,
                      child: PbButton(
                        label: 'Sign In',
                        onPressed: _loading ? null : _login,
                        loading: _loading,
                        icon: Icons.login,
                      ),
                    ),
                    const SizedBox(height: 16),
                    Row(
                      children: [
                        TextButton(
                          onPressed: () => context.go('/setup'),
                          child: const Text('Change server'),
                        ),
                        const Spacer(),
                        TextButton(
                          onPressed: () => context.go('/register'),
                          child: const Text('Register'),
                        ),
                      ],
                    ),
                  ] else ...[
                    SizedBox(
                      width: double.infinity,
                      child: PbButton(
                        label: 'Verify',
                        onPressed: _loading ? null : _verifyTotp,
                        loading: _loading,
                        icon: Icons.check,
                      ),
                    ),
                    const SizedBox(height: 16),
                    Row(
                      children: [
                        TextButton(
                          onPressed: _loading
                              ? null
                              : () => setState(() {
                                    _pendingToken = null;
                                    _error = null;
                                  }),
                          child: const Text('Back'),
                        ),
                        const Spacer(),
                        TextButton(
                          onPressed: _loading ? null : _recover,
                          child: const Text('Lost authenticator?'),
                        ),
                      ],
                    ),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

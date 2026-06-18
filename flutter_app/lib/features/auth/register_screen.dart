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

class RegisterScreen extends ConsumerStatefulWidget {
  const RegisterScreen({super.key});

  @override
  ConsumerState<RegisterScreen> createState() => _RegisterScreenState();
}

class _RegisterScreenState extends ConsumerState<RegisterScreen> {
  final _form = GlobalKey<FormState>();
  final _emailCtrl = TextEditingController();
  final _nameCtrl = TextEditingController();
  final _passCtrl = TextEditingController();
  final _pass2Ctrl = TextEditingController();
  final _tokenCtrl = TextEditingController();
  bool _loading = false;
  bool _obscure = true;
  String? _error;

  static final _specialCharsRe =
      RegExp(r"""[!@#$%^&*(),.?":{}|<>\-_=+\[\]\\;'`~/]""");

  @override
  void initState() {
    super.initState();
    _passCtrl.addListener(() => setState(() {}));
  }

  @override
  void dispose() {
    _emailCtrl.dispose();
    _nameCtrl.dispose();
    _passCtrl.dispose();
    _pass2Ctrl.dispose();
    _tokenCtrl.dispose();
    super.dispose();
  }

  Future<void> _register() async {
    if (!_form.currentState!.validate()) return;
    if (_passCtrl.text != _pass2Ctrl.text) {
      setState(() => _error = 'Passwords do not match');
      return;
    }
    setState(() { _loading = true; _error = null; });
    try {
      final pending = await ref.read(authStateProvider.notifier).register(
            _emailCtrl.text.trim(),
            _nameCtrl.text.trim(),
            _passCtrl.text,
            _tokenCtrl.text.trim(),
          );
      if (pending != null && mounted) {
        // Email verification required — go to login and show the message there.
        context.go('/login', extra: pending);
      }
      // If pending == null, router auto-navigates because isLoggedIn flipped.
    } catch (e) {
      setState(() => _error = e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  String? _validatePassword(String? v) {
    if (v == null || v.isEmpty) return 'Required';
    if (v.length < 12) return 'Min. 12 Zeichen erforderlich';
    if (!RegExp(r'\d').hasMatch(v)) return 'Min. 1 Zahl erforderlich';
    if (!_specialCharsRe.hasMatch(v)) return 'Min. 1 Sonderzeichen erforderlich';
    return null;
  }

  int get _passwordStrength {
    final v = _passCtrl.text;
    int s = 0;
    if (v.length >= 12) s++;
    if (RegExp(r'\d').hasMatch(v)) s++;
    if (_specialCharsRe.hasMatch(v)) s++;
    return s;
  }

  Widget _buildStrengthBar() {
    final pw = _passCtrl.text;
    if (pw.isEmpty) return const SizedBox.shrink();

    final s = _passwordStrength;
    const colors = [Colors.red, Colors.orange, Colors.yellow, Colors.green];

    return Padding(
      padding: const EdgeInsets.only(top: 6, bottom: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: List.generate(
              3,
              (i) => Expanded(
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 200),
                  height: 4,
                  margin: EdgeInsets.only(right: i < 2 ? 4 : 0),
                  decoration: BoxDecoration(
                    color: i < s ? colors[s] : Colors.white12,
                    borderRadius: BorderRadius.circular(2),
                  ),
                ),
              ),
            ),
          ),
          const SizedBox(height: 6),
          _rule('Min. 12 Zeichen', pw.length >= 12),
          _rule('Min. 1 Zahl', RegExp(r'\d').hasMatch(pw)),
          _rule('Min. 1 Sonderzeichen', _specialCharsRe.hasMatch(pw)),
        ],
      ),
    );
  }

  Widget _rule(String label, bool ok) => Padding(
        padding: const EdgeInsets.only(bottom: 2),
        child: Row(
          children: [
            Icon(
              ok ? Icons.check_circle : Icons.radio_button_unchecked,
              size: 14,
              color: ok ? Colors.green : AppTheme.onBgDim,
            ),
            const SizedBox(width: 6),
            Text(
              label,
              style: TextStyle(
                fontSize: 12,
                color: ok ? Colors.green : AppTheme.onBgDim,
              ),
            ),
          ],
        ),
      );

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('> REGISTER'),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => context.go('/login'),
        ),
      ),
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 420),
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(32),
            child: Form(
              key: _form,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const AppLogo(size: 56),
                  const SizedBox(height: 24),
                  PbTextField(
                    label: 'Invitation Token',
                    controller: _tokenCtrl,
                    prefixIcon: Icons.vpn_key_outlined,
                    validator: (v) => null, // optional for first user
                  ),
                  const SizedBox(height: 16),
                  PbTextField(
                    label: 'Email',
                    controller: _emailCtrl,
                    keyboardType: TextInputType.emailAddress,
                    prefixIcon: Icons.alternate_email,
                    validator: (v) => v!.isEmpty ? 'Required' : null,
                  ),
                  const SizedBox(height: 16),
                  PbTextField(
                    label: 'Display Name',
                    controller: _nameCtrl,
                    prefixIcon: Icons.person_outline,
                    validator: (v) => v!.isEmpty ? 'Required' : null,
                  ),
                  const SizedBox(height: 16),
                  PbTextField(
                    label: 'Master Password',
                    controller: _passCtrl,
                    obscureText: _obscure,
                    prefixIcon: Icons.lock_outline,
                    suffixIcon: IconButton(
                      icon: Icon(
                        _obscure ? Icons.visibility_off : Icons.visibility,
                        color: AppTheme.onBgDim,
                      ),
                      onPressed: () => setState(() => _obscure = !_obscure),
                    ),
                    validator: _validatePassword,
                  ),
                  _buildStrengthBar(),
                  const SizedBox(height: 16),
                  PbTextField(
                    label: 'Confirm Master Password',
                    controller: _pass2Ctrl,
                    obscureText: _obscure,
                    prefixIcon: Icons.lock_outline,
                    validator: (v) => v!.isEmpty ? 'Required' : null,
                    onSubmitted: (_) => _register(),
                  ),
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
                        style: const TextStyle(color: AppTheme.error),
                      ),
                    ),
                  ],
                  const SizedBox(height: 24),
                  SizedBox(
                    width: double.infinity,
                    child: PbButton(
                      label: 'Create Account',
                      onPressed: _loading ? null : _register,
                      loading: _loading,
                      icon: Icons.person_add_outlined,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

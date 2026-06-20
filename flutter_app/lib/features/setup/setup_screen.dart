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

import '../../core/api/api_client.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_button.dart';
import '../../shared/widgets/pb_text_field.dart';

/// First-launch screen: user enters server URL.
class SetupScreen extends ConsumerStatefulWidget {
  const SetupScreen({super.key});

  @override
  ConsumerState<SetupScreen> createState() => _SetupScreenState();
}

class _SetupScreenState extends ConsumerState<SetupScreen> {
  late final TextEditingController _urlCtrl;
  bool _loading = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    final existing = ref.read(apiClientProvider).baseUrl;
    _urlCtrl = TextEditingController(
      text: (existing != null && existing.isNotEmpty) ? existing : 'https://',
    );
  }

  @override
  void dispose() {
    _urlCtrl.dispose();
    super.dispose();
  }

  Future<void> _connect() async {
    final url = _urlCtrl.text.trim();
    if (url.isEmpty || !url.startsWith('http')) {
      setState(() => _error = 'Enter a valid server URL');
      return;
    }
    setState(() { _loading = true; _error = null; });
    try {
      await ref.read(apiClientProvider).setServerUrl(url);
      if (mounted) context.go('/login');
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      setState(() => _loading = false);
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
                Text(
                  'PASSBUBBLE',
                  style: Theme.of(context).textTheme.headlineMedium?.copyWith(
                        color: AppTheme.green,
                        letterSpacing: 4,
                        fontWeight: FontWeight.w700,
                      ),
                ),
                const SizedBox(height: 4),
                Text(
                  'self-hosted · e2e encrypted · open source',
                  style: Theme.of(context)
                      .textTheme
                      .bodySmall
                      ?.copyWith(color: AppTheme.onBgDim),
                ),
                const SizedBox(height: 48),
                PbTextField(
                  label: 'Server URL',
                  hint: 'https://pass.yourdomain.com',
                  controller: _urlCtrl,
                  keyboardType: TextInputType.url,
                  prefixIcon: Icons.dns_outlined,
                  onSubmitted: (_) => _connect(),
                ),
                if (_error != null) ...[
                  const SizedBox(height: 8),
                  Text(_error!, style: const TextStyle(color: AppTheme.error)),
                ],
                const SizedBox(height: 24),
                SizedBox(
                  width: double.infinity,
                  child: PbButton(
                    label: 'Connect',
                    onPressed: _loading ? null : _connect,
                    loading: _loading,
                    icon: Icons.arrow_forward,
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

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
import '../../core/generator/generator_defaults_provider.dart';
import '../../core/generator/password_generator.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/bottom_nav.dart';
import '../../shared/widgets/generator_options_form.dart';
import '../../shared/widgets/pb_button.dart';
import '../../shared/widgets/prompt_title.dart';
import '../../shared/widgets/strength_bar.dart';

/// The Generate tab. Generates on the device, starting from the generator
/// defaults; option changes here are not saved (Settings → Password
/// generator changes the defaults).
class GenerateScreen extends ConsumerStatefulWidget {
  const GenerateScreen({super.key});

  @override
  ConsumerState<GenerateScreen> createState() => _GenerateScreenState();
}

class _GenerateScreenState extends ConsumerState<GenerateScreen> {
  late GeneratorOptions _options;
  bool _edited = false;
  int _count = 1;
  List<String> _results = [];

  void _generate() {
    _results = validateGeneratorOptions(_options) == null
        ? [for (var i = 0; i < _count; i++) generatePassword(_options)]
        : [];
  }

  void _onChanged(GeneratorOptions o) => setState(() {
        _edited = true;
        _options = o;
        _generate();
      });

  @override
  void initState() {
    super.initState();
    final defaults = ref.read(generatorDefaultsProvider.notifier);
    _options = ref.read(generatorDefaultsProvider);
    _generate();
    // Right after start the stored defaults may still be loading.
    defaults.loaded.then((_) {
      if (!mounted || _edited) return;
      final loaded = ref.read(generatorDefaultsProvider);
      if (loaded == _options) return;
      setState(() {
        _options = loaded;
        _generate();
      });
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const PromptTitle('generate')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          GeneratorOptionsForm(options: _options, onChanged: _onChanged),
          const SizedBox(height: 8),

          // Count
          Row(
            children: [
              const Text('Count: '),
              ...List.generate(
                5,
                (i) => Padding(
                  padding: const EdgeInsets.only(left: 8),
                  child: ChoiceChip(
                    label: Text('${i + 1}'),
                    selected: _count == i + 1,
                    onSelected: (_) => setState(() {
                      _count = i + 1;
                      _generate();
                    }),
                    selectedColor: AppTheme.greenFaint,
                  ),
                ),
              ),
            ],
          ),

          const Divider(height: 32),

          // Results
          for (final pw in _results) _PasswordCard(password: pw),

          const SizedBox(height: 16),
          SizedBox(
            width: double.infinity,
            child: PbButton(
              label: 'Regenerate',
              onPressed: _results.isEmpty ? null : () => setState(_generate),
              icon: Icons.refresh,
              outlined: true,
            ),
          ),
        ],
      ),
      bottomNavigationBar: const PbBottomNav(currentIndex: 2),
    );
  }
}

class _PasswordCard extends StatelessWidget {
  final String password;
  const _PasswordCard({required this.password});

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        border: Border.all(color: AppTheme.border),
        color: AppTheme.surface,
      ),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  password,
                  key: const ValueKey('generated-password'),
                  style: AppTheme.mono(
                    fontSize: 14,
                    color: AppTheme.green,
                  ),
                ),
                const SizedBox(height: 4),
                StrengthBar(password: password),
              ],
            ),
          ),
          IconButton(
            icon: const Icon(Icons.copy, size: 18, color: AppTheme.onBgDim),
            onPressed: () {
              Clipboard.setData(ClipboardData(text: password));
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text('Password copied')),
              );
            },
          ),
        ],
      ),
    );
  }
}

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

import '../../core/generator/generator_defaults_provider.dart';
import '../../core/generator/password_generator.dart';
import '../../core/theme/app_theme.dart';
import 'generator_options_form.dart';
import 'strength_bar.dart';

/// Opens the password generator, starting from this device's defaults.
/// Returns the password on OK, null on Cancel. Option changes in the dialog
/// apply to this one password only.
Future<String?> showGeneratorDialog(BuildContext context, WidgetRef ref) async {
  await ref.read(generatorDefaultsProvider.notifier).loaded;
  if (!context.mounted) return null;
  return showDialog<String>(
    context: context,
    builder: (_) => const GeneratorDialog(),
  );
}

/// Generated password with strength and "Regenerate", the options below.
/// Pops the password on OK.
class GeneratorDialog extends ConsumerStatefulWidget {
  const GeneratorDialog({super.key});

  @override
  ConsumerState<GeneratorDialog> createState() => _GeneratorDialogState();
}

class _GeneratorDialogState extends ConsumerState<GeneratorDialog> {
  late GeneratorOptions _options;
  String? _password;

  @override
  void initState() {
    super.initState();
    _options = ref.read(generatorDefaultsProvider);
    _generate();
  }

  /// A new password for the current options; none while they are invalid
  /// (the form says why).
  void _generate() {
    _password = validateGeneratorOptions(_options) == null
        ? generatePassword(_options)
        : null;
  }

  void _onChanged(GeneratorOptions o) => setState(() {
        _options = o;
        _generate();
      });

  @override
  Widget build(BuildContext context) {
    final password = _password;
    return AlertDialog(
      title: const Text('Generate password'),
      content: SizedBox(
        width: 420,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                padding: const EdgeInsets.fromLTRB(12, 4, 4, 4),
                decoration: BoxDecoration(
                  border: Border.all(color: AppTheme.border),
                  color: AppTheme.surface,
                ),
                child: Row(
                  children: [
                    Expanded(
                      child: SelectableText(
                        password ?? '—',
                        key: const ValueKey('generated-password'),
                        style: AppTheme.mono(
                          fontSize: 14,
                          color: password == null ? AppTheme.onBgDim : AppTheme.green,
                        ),
                      ),
                    ),
                    IconButton(
                      icon: const Icon(Icons.refresh, size: 18, color: AppTheme.onBgDim),
                      tooltip: 'Regenerate',
                      onPressed: password == null ? null : () => setState(_generate),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 6),
              if (password != null) StrengthBar(password: password),
              const SizedBox(height: 12),
              GeneratorOptionsForm(options: _options, onChanged: _onChanged),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        TextButton(
          onPressed: password == null ? null : () => Navigator.of(context).pop(password),
          child: const Text('OK'),
        ),
      ],
    );
  }
}

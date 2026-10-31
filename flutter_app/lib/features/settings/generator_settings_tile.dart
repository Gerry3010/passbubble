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
import '../../shared/widgets/generator_options_form.dart';

/// Settings tile (under VAULT) for the password generator defaults of this
/// device. The only place they change; every generator starts from them.
class GeneratorSettingsTile extends ConsumerWidget {
  const GeneratorSettingsTile({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return ListTile(
      leading: const Icon(Icons.casino_outlined),
      title: const Text('Password generator'),
      subtitle: Text(describeGeneratorOptions(ref.watch(generatorDefaultsProvider))),
      trailing: const Icon(Icons.chevron_right),
      onTap: () async {
        await ref.read(generatorDefaultsProvider.notifier).loaded;
        if (!context.mounted) return;
        await showDialog<void>(
          context: context,
          builder: (_) => const _GeneratorDefaultsDialog(),
        );
      },
    );
  }
}

/// Edits the defaults. "Reset to defaults" only resets the form; nothing is
/// stored before Save.
class _GeneratorDefaultsDialog extends ConsumerStatefulWidget {
  const _GeneratorDefaultsDialog();

  @override
  ConsumerState<_GeneratorDefaultsDialog> createState() =>
      _GeneratorDefaultsDialogState();
}

class _GeneratorDefaultsDialogState extends ConsumerState<_GeneratorDefaultsDialog> {
  late GeneratorOptions _options = ref.read(generatorDefaultsProvider);
  bool _busy = false;

  Future<void> _save() async {
    setState(() => _busy = true);
    final notifier = ref.read(generatorDefaultsProvider.notifier);
    // The factory defaults are stored as "nothing stored" (see reset).
    if (_options == defaultGeneratorOptions) {
      await notifier.reset();
    } else {
      await notifier.save(_options);
    }
    if (mounted) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final valid = validateGeneratorOptions(_options) == null;
    return AlertDialog(
      title: const Text('Password generator'),
      content: SizedBox(
        width: 420,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text(
                'Defaults for new passwords on this device. Changes in the '
                'generator itself apply to that one password only.',
                style: TextStyle(fontSize: 12),
              ),
              const SizedBox(height: 12),
              GeneratorOptionsForm(
                options: _options,
                onChanged: (o) => setState(() => _options = o),
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _busy || _options == defaultGeneratorOptions
              ? null
              : () => setState(() => _options = defaultGeneratorOptions),
          child: const Text('Reset to defaults'),
        ),
        TextButton(
          onPressed: _busy ? null : () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        TextButton(
          onPressed: _busy || !valid ? null : _save,
          child: const Text('Save'),
        ),
      ],
    );
  }
}

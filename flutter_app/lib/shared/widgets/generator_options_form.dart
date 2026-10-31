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

import '../../core/generator/password_generator.dart';
import '../../core/theme/app_theme.dart';

/// The generator options: length, the four character classes, the symbol set
/// and "avoid ambiguous". Shared by the generator dialog, the Generate tab and
/// the defaults in Settings. Stateless about the options themselves: every
/// change goes to [onChanged] and comes back as [options].
class GeneratorOptionsForm extends StatefulWidget {
  final GeneratorOptions options;
  final ValueChanged<GeneratorOptions> onChanged;
  const GeneratorOptionsForm({
    super.key,
    required this.options,
    required this.onChanged,
  });

  @override
  State<GeneratorOptionsForm> createState() => _GeneratorOptionsFormState();
}

class _GeneratorOptionsFormState extends State<GeneratorOptionsForm> {
  late final _symbolCtrl = TextEditingController(text: widget.options.symbolSet);

  @override
  void didUpdateWidget(GeneratorOptionsForm old) {
    super.didUpdateWidget(old);
    // Options replaced from outside (e.g. "Reset to defaults").
    if (widget.options.symbolSet != _symbolCtrl.text) {
      _symbolCtrl.text = widget.options.symbolSet;
    }
  }

  @override
  void dispose() {
    _symbolCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final o = widget.options;
    final error = validateGeneratorOptions(o);

    Widget classChip(String key, String label, bool selected,
            GeneratorOptions Function(bool) toggle) =>
        FilterChip(
          key: ValueKey('generator-$key'),
          label: Text(label),
          selected: selected,
          onSelected: (v) => widget.onChanged(toggle(v)),
          selectedColor: AppTheme.greenFaint,
          side: BorderSide(color: selected ? AppTheme.green : AppTheme.border),
        );

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            const Text('Length: '),
            Text(
              '${o.length}',
              style: const TextStyle(color: AppTheme.green, fontWeight: FontWeight.bold),
            ),
          ],
        ),
        Slider(
          key: const ValueKey('generator-length'),
          value: o.length.clamp(minGeneratorLength, maxGeneratorLength).toDouble(),
          min: minGeneratorLength.toDouble(),
          max: maxGeneratorLength.toDouble(),
          divisions: maxGeneratorLength - minGeneratorLength,
          label: '${o.length}',
          activeColor: AppTheme.green,
          onChanged: (v) => widget.onChanged(o.copyWith(length: v.round())),
        ),
        Wrap(
          spacing: 8,
          runSpacing: 4,
          children: [
            classChip('lowercase', 'a-z', o.lowercase, (v) => o.copyWith(lowercase: v)),
            classChip('uppercase', 'A-Z', o.uppercase, (v) => o.copyWith(uppercase: v)),
            classChip('digits', '0-9', o.digits, (v) => o.copyWith(digits: v)),
            classChip('symbols', 'Symbols', o.symbols, (v) => o.copyWith(symbols: v)),
          ],
        ),
        const SizedBox(height: 12),
        TextField(
          key: const ValueKey('generator-symbol-set'),
          controller: _symbolCtrl,
          enabled: o.symbols,
          autocorrect: false,
          enableSuggestions: false,
          style: AppTheme.mono(fontSize: 14),
          decoration: const InputDecoration(
            labelText: 'Symbol set',
            prefixIcon: Icon(Icons.tag),
          ),
          onChanged: (v) => widget.onChanged(o.copyWith(symbolSet: v)),
        ),
        CheckboxListTile(
          key: const ValueKey('generator-avoid-ambiguous'),
          value: o.avoidAmbiguous,
          onChanged: (v) => widget.onChanged(o.copyWith(avoidAmbiguous: v ?? false)),
          title: const Text('Avoid ambiguous characters'),
          subtitle: Text(ambiguousChars.split('').join(' '),
              style: AppTheme.mono(fontSize: 12, color: AppTheme.onBgDim)),
          activeColor: AppTheme.green,
          controlAffinity: ListTileControlAffinity.leading,
          contentPadding: EdgeInsets.zero,
          dense: true,
        ),
        if (error != null)
          Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Text(
              error,
              key: const ValueKey('generator-error'),
              style: const TextStyle(color: AppTheme.error, fontSize: 12),
            ),
          ),
      ],
    );
  }
}

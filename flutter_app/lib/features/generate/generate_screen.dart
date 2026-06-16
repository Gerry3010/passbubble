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

import '../../core/api/api_client.dart';
import '../../core/api/models.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_button.dart';

class GenerateScreen extends ConsumerStatefulWidget {
  const GenerateScreen({super.key});

  @override
  ConsumerState<GenerateScreen> createState() => _GenerateScreenState();
}

class _GenerateScreenState extends ConsumerState<GenerateScreen> {
  int _length = 20;
  String _type = 'strong';
  bool _noAmbiguous = false;
  int _count = 1;
  List<GeneratedPassword> _results = [];
  bool _loading = false;

  Future<void> _generate() async {
    setState(() { _loading = true; _results = []; });
    try {
      final resp = await ref.read(apiClientProvider).generate(
            length: _length,
            type: _type,
            count: _count,
            noAmbiguous: _noAmbiguous,
          );
      setState(() => _results = resp.passwords);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(e.toString())),
        );
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  void initState() {
    super.initState();
    _generate();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('> GENERATE')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          // Length slider
          Row(
            children: [
              const Text('Length: '),
              Text(
                '$_length',
                style: const TextStyle(color: AppTheme.green, fontWeight: FontWeight.bold),
              ),
            ],
          ),
          Slider(
            value: _length.toDouble(),
            min: 8,
            max: 64,
            divisions: 56,
            activeColor: AppTheme.green,
            onChanged: (v) => setState(() => _length = v.round()),
            onChangeEnd: (_) => _generate(),
          ),

          // Type selector
          const SizedBox(height: 8),
          Wrap(
            spacing: 8,
            children: [
              for (final t in ['strong', 'alphanum', 'numbers', 'lower'])
                ChoiceChip(
                  label: Text(t),
                  selected: _type == t,
                  onSelected: (_) {
                    setState(() => _type = t);
                    _generate();
                  },
                  selectedColor: AppTheme.greenFaint,
                  side: BorderSide(
                    color: _type == t ? AppTheme.green : AppTheme.border,
                  ),
                ),
            ],
          ),
          const SizedBox(height: 12),

          // Options
          Row(
            children: [
              Checkbox(
                value: _noAmbiguous,
                onChanged: (v) {
                  setState(() => _noAmbiguous = v ?? false);
                  _generate();
                },
                activeColor: AppTheme.green,
              ),
              const Text('No ambiguous characters (0, O, l, 1, I)'),
            ],
          ),

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
                    onSelected: (_) {
                      setState(() => _count = i + 1);
                      _generate();
                    },
                    selectedColor: AppTheme.greenFaint,
                  ),
                ),
              ),
            ],
          ),

          const Divider(height: 32),

          // Results
          if (_loading)
            const Center(child: CircularProgressIndicator())
          else
            for (final pw in _results) _PasswordCard(pw: pw),

          const SizedBox(height: 16),
          SizedBox(
            width: double.infinity,
            child: PbButton(
              label: 'Regenerate',
              onPressed: _generate,
              icon: Icons.refresh,
              outlined: true,
            ),
          ),
        ],
      ),
      bottomNavigationBar: const _BottomNav(currentIndex: 2),
    );
  }
}

class _PasswordCard extends StatelessWidget {
  final GeneratedPassword pw;
  const _PasswordCard({required this.pw});

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
                  pw.password,
                  style: const TextStyle(
                    fontFamily: 'monospace',
                    fontSize: 14,
                    color: AppTheme.green,
                  ),
                ),
                const SizedBox(height: 4),
                _StrengthBar(strength: pw.strength),
              ],
            ),
          ),
          IconButton(
            icon: const Icon(Icons.copy, size: 18, color: AppTheme.onBgDim),
            onPressed: () {
              Clipboard.setData(ClipboardData(text: pw.password));
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

class _StrengthBar extends StatelessWidget {
  final int strength;
  const _StrengthBar({required this.strength});

  @override
  Widget build(BuildContext context) {
    final color = strength >= 80
        ? AppTheme.green
        : strength >= 50
            ? Colors.amber
            : AppTheme.error;

    return Row(
      children: [
        Expanded(
          child: LinearProgressIndicator(
            value: strength / 100,
            backgroundColor: AppTheme.border,
            valueColor: AlwaysStoppedAnimation(color),
            minHeight: 3,
          ),
        ),
        const SizedBox(width: 8),
        Text(
          '$strength',
          style: TextStyle(color: color, fontSize: 11),
        ),
      ],
    );
  }
}

// Re-export the bottom nav from entries for use here
class _BottomNav extends _BottomNavBase {
  const _BottomNav({required super.currentIndex});
}

class _BottomNavBase extends StatelessWidget {
  final int currentIndex;
  const _BottomNavBase({required this.currentIndex});

  @override
  Widget build(BuildContext context) {
    return NavigationBar(
      backgroundColor: AppTheme.bg,
      selectedIndex: currentIndex,
      indicatorColor: AppTheme.greenFaint,
      destinations: const [
        NavigationDestination(
          icon: Icon(Icons.lock_outline),
          selectedIcon: Icon(Icons.lock, color: AppTheme.green),
          label: 'Vault',
        ),
        NavigationDestination(
          icon: Icon(Icons.folder_outlined),
          selectedIcon: Icon(Icons.folder, color: AppTheme.green),
          label: 'Folders',
        ),
        NavigationDestination(
          icon: Icon(Icons.casino_outlined),
          selectedIcon: Icon(Icons.casino, color: AppTheme.green),
          label: 'Generate',
        ),
      ],
      onDestinationSelected: (i) {
        switch (i) {
          case 0:
            Navigator.of(context).pushReplacementNamed('/entries');
          case 1:
            Navigator.of(context).pushReplacementNamed('/folders');
          case 2:
            break; // already here
        }
      },
    );
  }
}

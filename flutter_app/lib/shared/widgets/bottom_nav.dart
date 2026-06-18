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
import 'package:go_router/go_router.dart';

import '../../core/theme/app_theme.dart';

class PbBottomNav extends StatelessWidget {
  final int currentIndex;
  const PbBottomNav({super.key, required this.currentIndex});

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
        NavigationDestination(
          icon: Icon(Icons.tune_outlined),
          selectedIcon: Icon(Icons.tune, color: AppTheme.green),
          label: 'Manage',
        ),
      ],
      onDestinationSelected: (i) {
        switch (i) {
          case 0:
            context.go('/entries');
          case 1:
            context.go('/folders');
          case 2:
            context.go('/generate');
          case 3:
            context.go('/manage');
        }
      },
    );
  }
}

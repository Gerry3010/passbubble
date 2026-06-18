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

import '../../core/theme/app_theme.dart';
import '../../shared/widgets/bottom_nav.dart';
import 'export_tab.dart';
import 'import_tab.dart';
import 'jobs_tab.dart';
import 'shares_tab.dart';

class ManageScreen extends StatelessWidget {
  const ManageScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 4,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('> MANAGE'),
          bottom: const TabBar(
            labelColor: AppTheme.green,
            unselectedLabelColor: AppTheme.onBgDim,
            indicatorColor: AppTheme.green,
            tabs: [
              Tab(text: 'Import'),
              Tab(text: 'Export'),
              Tab(text: 'Shares'),
              Tab(text: 'Jobs'),
            ],
          ),
        ),
        body: const TabBarView(
          children: [
            ImportTab(),
            ExportTab(),
            SharesTab(),
            JobsTab(),
          ],
        ),
        bottomNavigationBar: const PbBottomNav(currentIndex: 3),
      ),
    );
  }
}

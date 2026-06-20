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

import '../../core/jobs/job_runner.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/bottom_nav.dart';
import 'export_tab.dart';
import 'import_tab.dart';
import 'jobs_tab.dart';
import 'shares_tab.dart';

class ManageScreen extends ConsumerWidget {
  /// Tab to open initially (0=Import … 3=Jobs). Driven by the `?tab=` query
  /// param so the job notifications' VIEW action can deep-link to the Job View.
  final int? initialTab;
  const ManageScreen({super.key, this.initialTab});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final activeJobs = ref.watch(activeJobCountProvider);
    return DefaultTabController(
      length: 4,
      initialIndex: (initialTab != null && initialTab! >= 0 && initialTab! < 4)
          ? initialTab!
          : 0,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('> MANAGE'),
          bottom: TabBar(
            labelColor: AppTheme.green,
            unselectedLabelColor: AppTheme.onBgDim,
            indicatorColor: AppTheme.green,
            tabs: [
              const Tab(text: 'Import'),
              const Tab(text: 'Export'),
              const Tab(text: 'Shares'),
              Tab(
                child: Badge(
                  isLabelVisible: activeJobs > 0,
                  label: Text('$activeJobs'),
                  backgroundColor: AppTheme.green,
                  textColor: AppTheme.bg,
                  offset: const Offset(12, -4),
                  child: const Text('Jobs'),
                ),
              ),
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
        bottomNavigationBar: const PbBottomNav(currentIndex: 2),
      ),
    );
  }
}

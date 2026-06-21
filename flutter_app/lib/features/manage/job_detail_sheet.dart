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

import '../../core/jobs/job_messenger.dart';
import '../../core/jobs/job_runner.dart';
import '../../core/jobs/local_job.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_button.dart';
import 'jobs_tab.dart';

/// Opens the Job Detail View (progress + full log) as a bottom sheet. While a
/// job is running the sheet updates live (it re-reads the [JobRunner] by id).
void showJobDetailSheet(BuildContext context, LocalJob initial) {
  showModalBottomSheet<void>(
    context: context,
    backgroundColor: AppTheme.bg,
    isScrollControlled: true,
    showDragHandle: true,
    builder: (_) => _JobDetailSheet(initial: initial),
  );
}

class _JobDetailSheet extends ConsumerWidget {
  final LocalJob initial;
  const _JobDetailSheet({required this.initial});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Prefer the live job from the runner (keeps progress/log fresh); fall back
    // to the snapshot for server-history jobs not tracked this session.
    final live = ref.watch(jobRunnerProvider);
    final job = live.firstWhere((j) => j.id == initial.id, orElse: () => initial);

    return DraggableScrollableSheet(
      expand: false,
      initialChildSize: 0.6,
      minChildSize: 0.3,
      maxChildSize: 0.92,
      builder: (ctx, scrollCtrl) => ListView(
        controller: scrollCtrl,
        padding: const EdgeInsets.fromLTRB(16, 0, 16, 24),
        children: [
          Row(
            children: [
              Text(
                job.type.toUpperCase(),
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                  color: jobStatusColor(job),
                  letterSpacing: 1.2,
                ),
              ),
              const SizedBox(width: 8),
              Text('· ${job.format}',
                  style: const TextStyle(fontSize: 13, color: AppTheme.onBgDim)),
              const Spacer(),
              JobStatusChip(status: job.status),
            ],
          ),
          const SizedBox(height: 12),

          LinearProgressIndicator(
            value: job.progressFraction == 0 && job.isRunning
                ? null
                : job.progressFraction,
            backgroundColor: AppTheme.border,
            valueColor: AlwaysStoppedAnimation(
                job.isFailed ? AppTheme.error : AppTheme.green),
          ),
          const SizedBox(height: 6),
          Text('${job.processedItems}/${job.totalItems}  ·  ${job.statusText}',
              style: const TextStyle(fontSize: 12, color: AppTheme.onBgDim)),
          const SizedBox(height: 10),

          Wrap(
            spacing: 12,
            children: [
              if (job.createdItems > 0)
                Text('${job.createdItems} created',
                    style: const TextStyle(fontSize: 12, color: AppTheme.green)),
              if (job.updatedItems > 0)
                Text('${job.updatedItems} updated',
                    style: const TextStyle(fontSize: 12, color: Colors.blue)),
              if (job.skippedItems > 0)
                Text('${job.skippedItems} skipped',
                    style: const TextStyle(fontSize: 12, color: AppTheme.onBgDim)),
              if (job.failedItems > 0)
                Text('${job.failedItems} failed',
                    style: const TextStyle(fontSize: 12, color: AppTheme.error)),
            ],
          ),

          if (job.errorMessage != null) ...[
            const SizedBox(height: 10),
            Text(job.errorMessage!,
                style: const TextStyle(fontSize: 12, color: AppTheme.error)),
          ],

          if (job.resultBytes != null) ...[
            const SizedBox(height: 16),
            PbButton(
              label: 'Share file',
              icon: Icons.ios_share,
              outlined: true,
              onPressed: () => shareJobResult(job),
            ),
          ],

          const SizedBox(height: 16),
          const Text('Log',
              style: TextStyle(fontSize: 12, color: AppTheme.onBgDim)),
          const SizedBox(height: 6),
          if (job.log.isEmpty)
            const Text(
              'No log available for this job. (Logs are kept only for jobs run in '
              'the current app session.)',
              style: TextStyle(fontSize: 12, color: AppTheme.onBgDim),
            )
          else
            Container(
              padding: const EdgeInsets.all(10),
              decoration: BoxDecoration(
                color: Colors.black.withValues(alpha: 0.25),
                borderRadius: BorderRadius.circular(6),
                border: Border.all(color: AppTheme.border),
              ),
              child: SelectableText(
                job.log.join('\n'),
                style: AppTheme.mono(fontSize: 11, color: AppTheme.onBgDim),
              ),
            ),
        ],
      ),
    );
  }
}

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

import '../../core/api/api_client.dart';
import '../../core/api/models.dart';
import '../../core/jobs/job_runner.dart';
import '../../core/jobs/local_job.dart';
import '../../core/theme/app_theme.dart';
import 'job_detail_sheet.dart';

final _jobsProvider = FutureProvider<List<JobResponse>>((ref) {
  return ref.watch(apiClientProvider).listJobs();
});

/// The Job View: live in-session jobs (from the [JobRunner]) merged on top of
/// the server-side history. Tapping a tile opens the detail sheet with progress
/// and — for jobs run this session — the full log.
class JobsTab extends ConsumerWidget {
  const JobsTab({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final liveJobs = ref.watch(jobRunnerProvider);
    final jobsAsync = ref.watch(_jobsProvider);

    return jobsAsync.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(child: Text('Error: $e')),
      data: (serverJobs) {
        // Live jobs (rich: log + result) on top; server history deduped by id.
        final liveIds = {for (final j in liveJobs) j.id};
        final merged = <LocalJob>[
          ...liveJobs,
          for (final j in serverJobs)
            if (!liveIds.contains(j.id)) LocalJob.fromServer(j),
        ];

        if (merged.isEmpty) {
          return const Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.work_history_outlined, size: 48, color: AppTheme.onBgDim),
                SizedBox(height: 12),
                Text('No jobs yet', style: TextStyle(color: AppTheme.onBgDim)),
              ],
            ),
          );
        }

        return RefreshIndicator(
          color: AppTheme.green,
          onRefresh: () async => ref.invalidate(_jobsProvider),
          child: ListView.builder(
            itemCount: merged.length,
            itemBuilder: (ctx, i) => JobTile(
              job: merged[i],
              onTap: () => showJobDetailSheet(ctx, merged[i]),
            ),
          ),
        );
      },
    );
  }
}

/// A single job row. Shared with the detail sheet header.
class JobTile extends StatelessWidget {
  final LocalJob job;
  final VoidCallback? onTap;
  const JobTile({super.key, required this.job, this.onTap});

  @override
  Widget build(BuildContext context) {
    final statusColor = jobStatusColor(job);
    final fraction = job.progressFraction;

    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(8),
        side: BorderSide(color: AppTheme.border),
      ),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(8),
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Icon(_iconForType(job.type), size: 16, color: statusColor),
                  const SizedBox(width: 6),
                  Text(
                    job.type.toUpperCase(),
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w600,
                      color: statusColor,
                      letterSpacing: 1.2,
                    ),
                  ),
                  const SizedBox(width: 8),
                  Text('· ${job.format}',
                      style: const TextStyle(fontSize: 12, color: AppTheme.onBgDim)),
                  const Spacer(),
                  JobStatusChip(status: job.status),
                ],
              ),
              const SizedBox(height: 8),

              if (job.isRunning || job.processedItems > 0) ...[
                LinearProgressIndicator(
                  value: fraction == 0 && job.isRunning ? null : fraction,
                  backgroundColor: AppTheme.border,
                  valueColor: AlwaysStoppedAnimation(
                      job.isFailed ? AppTheme.error : AppTheme.green),
                ),
                const SizedBox(height: 6),
                Text('${job.processedItems}/${job.totalItems}',
                    style: const TextStyle(fontSize: 11, color: AppTheme.onBgDim)),
                const SizedBox(height: 6),
              ],

              Wrap(
                spacing: 8,
                children: [
                  if (job.createdItems > 0)
                    _CountBadge(label: 'created', count: job.createdItems, color: AppTheme.green),
                  if (job.updatedItems > 0)
                    _CountBadge(label: 'updated', count: job.updatedItems, color: Colors.blue),
                  if (job.skippedItems > 0)
                    _CountBadge(label: 'skipped', count: job.skippedItems, color: AppTheme.onBgDim),
                  if (job.failedItems > 0)
                    _CountBadge(label: 'failed', count: job.failedItems, color: AppTheme.error),
                ],
              ),

              if (job.errorMessage != null) ...[
                const SizedBox(height: 6),
                Text(
                  job.errorMessage!,
                  style: const TextStyle(fontSize: 11, color: AppTheme.error),
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
              ],

              const SizedBox(height: 4),
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text(_formatDate(job.createdAt),
                      style: const TextStyle(fontSize: 10, color: AppTheme.onBgDim)),
                  const Text('Details ›',
                      style: TextStyle(fontSize: 10, color: AppTheme.onBgDim)),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }

  IconData _iconForType(String type) => switch (type) {
        'import' => Icons.upload_file,
        'export' => Icons.download,
        _ => Icons.work_outline,
      };

  String _formatDate(DateTime dt) {
    final l = dt.toLocal();
    return '${l.year}-${l.month.toString().padLeft(2, '0')}-${l.day.toString().padLeft(2, '0')} '
        '${l.hour.toString().padLeft(2, '0')}:${l.minute.toString().padLeft(2, '0')}';
  }
}

Color jobStatusColor(LocalJob job) => job.isCompleted
    ? AppTheme.green
    : job.isFailed
        ? AppTheme.error
        : job.isRunning
            ? Colors.blue
            : AppTheme.onBgDim;

class JobStatusChip extends StatelessWidget {
  final String status;
  const JobStatusChip({super.key, required this.status});

  @override
  Widget build(BuildContext context) {
    final color = switch (status) {
      'completed' => AppTheme.green,
      'failed' => AppTheme.error,
      'running' => Colors.blue,
      _ => AppTheme.onBgDim,
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: color.withValues(alpha: 0.4)),
      ),
      child: Text(status.toUpperCase(),
          style: TextStyle(fontSize: 10, color: color, letterSpacing: 0.8)),
    );
  }
}

class _CountBadge extends StatelessWidget {
  final String label;
  final int count;
  final Color color;
  const _CountBadge({required this.label, required this.count, required this.color});

  @override
  Widget build(BuildContext context) {
    return Text('$count $label', style: TextStyle(fontSize: 11, color: color));
  }
}

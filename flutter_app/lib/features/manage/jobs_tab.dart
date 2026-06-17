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
import '../../core/jobs/job_polling_service.dart';
import '../../core/theme/app_theme.dart';

final _jobsProvider = FutureProvider<List<JobResponse>>((ref) {
  return ref.watch(apiClientProvider).listJobs();
});

class JobsTab extends ConsumerWidget {
  const JobsTab({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // also watch the live running-jobs list from the polling service
    final runningJobs = ref.watch(runningJobsProvider);
    final jobsAsync = ref.watch(_jobsProvider);

    return jobsAsync.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(child: Text('Error: $e')),
      data: (jobs) {
        if (jobs.isEmpty && runningJobs.isEmpty) {
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

        // Merge running jobs from the polling service on top — they have fresh state.
        final runningIds = {for (final j in runningJobs) j.id};
        final merged = [
          ...runningJobs,
          ...jobs.where((j) => !runningIds.contains(j.id)),
        ];

        return RefreshIndicator(
          color: AppTheme.green,
          onRefresh: () async => ref.invalidate(_jobsProvider),
          child: ListView.builder(
            itemCount: merged.length,
            itemBuilder: (ctx, i) => _JobTile(job: merged[i]),
          ),
        );
      },
    );
  }
}

class _JobTile extends StatelessWidget {
  final JobResponse job;
  const _JobTile({required this.job});

  @override
  Widget build(BuildContext context) {
    final isRunning = job.status == 'running';
    final isCompleted = job.status == 'completed';
    final isFailed = job.status == 'failed';

    final statusColor = isCompleted
        ? AppTheme.green
        : isFailed
            ? AppTheme.error
            : isRunning
                ? Colors.blue
                : AppTheme.onBgDim;

    final fraction = job.progressFraction;

    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(8),
        side: BorderSide(color: AppTheme.border),
      ),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  _iconForType(job.type),
                  size: 16,
                  color: statusColor,
                ),
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
                Text(
                  '· ${job.format}',
                  style: const TextStyle(fontSize: 12, color: AppTheme.onBgDim),
                ),
                const Spacer(),
                _StatusChip(status: job.status),
              ],
            ),
            const SizedBox(height: 8),

            if (isRunning || job.processedItems > 0) ...[
              LinearProgressIndicator(
                value: fraction,
                backgroundColor: AppTheme.border,
                valueColor: AlwaysStoppedAnimation(isFailed ? AppTheme.error : AppTheme.green),
              ),
              const SizedBox(height: 6),
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text(
                    '${job.processedItems}/${job.totalItems}',
                    style: const TextStyle(fontSize: 11, color: AppTheme.onBgDim),
                  ),
                  if (job.clientName != null)
                    Text(
                      'from ${job.clientName}',
                      style: const TextStyle(fontSize: 11, color: AppTheme.onBgDim),
                    ),
                ],
              ),
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
            Text(
              _formatDate(job.createdAt),
              style: const TextStyle(fontSize: 10, color: AppTheme.onBgDim),
            ),
          ],
        ),
      ),
    );
  }

  IconData _iconForType(String type) {
    return switch (type) {
      'import' => Icons.upload_file,
      'export' => Icons.download,
      _ => Icons.work_outline,
    };
  }

  String _formatDate(String iso) {
    try {
      final dt = DateTime.parse(iso).toLocal();
      return '${dt.year}-${dt.month.toString().padLeft(2, '0')}-${dt.day.toString().padLeft(2, '0')} '
          '${dt.hour.toString().padLeft(2, '0')}:${dt.minute.toString().padLeft(2, '0')}';
    } catch (_) {
      return iso;
    }
  }
}

class _StatusChip extends StatelessWidget {
  final String status;
  const _StatusChip({required this.status});

  @override
  Widget build(BuildContext context) {
    final color = switch (status) {
      'completed' => AppTheme.green,
      'failed' => AppTheme.error,
      'running' => Colors.blue,
      'cancelled' => AppTheme.onBgDim,
      _ => AppTheme.onBgDim,
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: color.withValues(alpha: 0.4)),
      ),
      child: Text(
        status.toUpperCase(),
        style: TextStyle(fontSize: 10, color: color, letterSpacing: 0.8),
      ),
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
    return Text(
      '$count $label',
      style: TextStyle(fontSize: 11, color: color),
    );
  }
}

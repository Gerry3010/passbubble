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
import 'package:share_plus/share_plus.dart';

import '../router/router.dart';
import '../theme/app_theme.dart';
import 'local_job.dart';

/// Attached to [MaterialApp.router] so the [JobRunner] can show banners/snackbars
/// from anywhere — including after the user has navigated away from Manage.
final scaffoldMessengerKey = GlobalKey<ScaffoldMessengerState>();

/// Location of the Manage ▸ Jobs tab. The `tab` index matches the order of the
/// TabBar in ManageScreen (Import, Export, Shares, Jobs).
const _jobsLocation = '/manage?tab=3';

/// Navigate to the Job View with a clean stack (go_router `go()` replaces the
/// current stack, giving the "clean state" the flow asks for).
void _goToJobs(Ref ref) => ref.read(routerProvider).go(_jobsLocation);

String _verb(String type) => type == 'export' ? 'Export' : 'Import';

/// Shown when a job starts: a persistent banner with a VIEW action.
void showJobStartedBanner(Ref ref, LocalJob job) {
  final messenger = scaffoldMessengerKey.currentState;
  if (messenger == null) return;
  messenger.clearMaterialBanners();
  messenger.showMaterialBanner(
    MaterialBanner(
      backgroundColor: AppTheme.greenFaint,
      content: Text(
        '${_verb(job.type)} started — ${job.totalItems} item(s)…',
        style: const TextStyle(color: AppTheme.green),
      ),
      leading: const Icon(Icons.sync, color: AppTheme.green, size: 20),
      actions: [
        TextButton(
          onPressed: () {
            messenger.hideCurrentMaterialBanner();
            _goToJobs(ref);
          },
          child: const Text('VIEW', style: TextStyle(color: AppTheme.green)),
        ),
        TextButton(
          onPressed: messenger.hideCurrentMaterialBanner,
          child:
              const Text('DISMISS', style: TextStyle(color: AppTheme.onBgDim)),
        ),
      ],
    ),
  );
}

/// Shown when a job finishes (success / completed-with-failures / failed): a
/// SnackBar coloured by outcome, with a VIEW action and — for exports that
/// produced a file — a SHARE action.
void showJobDoneSnack(Ref ref, LocalJob job) {
  final messenger = scaffoldMessengerKey.currentState;
  if (messenger == null) return;
  // A start banner may still be up; clear it so the result is visible.
  messenger.clearMaterialBanners();

  final hasFailures = job.failedItems > 0;
  final color = job.isFailed
      ? AppTheme.error
      : hasFailures
          ? Colors.amber
          : AppTheme.green;

  final canShare = job.resultBytes != null;

  messenger.hideCurrentSnackBar();
  messenger.showSnackBar(
    SnackBar(
      duration: const Duration(seconds: 8),
      backgroundColor: AppTheme.bg,
      content: Row(
        children: [
          Icon(
            job.isFailed
                ? Icons.error_outline
                : hasFailures
                    ? Icons.warning_amber_rounded
                    : Icons.check_circle_outline,
            color: color,
            size: 20,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              job.statusText.isNotEmpty
                  ? job.statusText
                  : '${_verb(job.type)} ${job.status}',
              style: TextStyle(color: color),
            ),
          ),
          TextButton(
            onPressed: () {
              messenger.hideCurrentSnackBar();
              _goToJobs(ref);
            },
            child: const Text('VIEW', style: TextStyle(color: AppTheme.green)),
          ),
          if (canShare)
            TextButton(
              onPressed: () => shareJobResult(job),
              child:
                  const Text('SHARE', style: TextStyle(color: AppTheme.green)),
            ),
        ],
      ),
    ),
  );
}

/// Re-share the file an export job produced.
Future<void> shareJobResult(LocalJob job) async {
  final bytes = job.resultBytes;
  if (bytes == null) return;
  await Share.shareXFiles(
    [
      XFile.fromData(
        bytes,
        name: job.resultFilename ?? 'passbubble-export',
        mimeType: job.resultMime,
      ),
    ],
    subject: 'Passbubble Export',
    // iOS needs a non-zero source rect; no widget context here, so use a safe
    // fallback (anchors the iPad popover to the top-left corner).
    sharePositionOrigin: const Rect.fromLTWH(0, 0, 1, 1),
  );
}

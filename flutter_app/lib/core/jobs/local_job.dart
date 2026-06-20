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

import 'dart:typed_data';

import '../api/models.dart' show JobResponse;

/// Supported import formats. Shared by [ImportTab] and the [JobRunner].
enum ImportFormat {
  csvGeneric,
  csvChrome,
  csvLastPass,
  csv1Password,
  bitwarden,
  psono,
  onepassword1pux,
}

/// Supported export formats. Shared by [ExportTab] and the [JobRunner].
enum ExportFormat { csv, bitwarden, onepasswordCsv, onepassword1pux }

/// Duplicate handling strategy for imports.
enum DupStrategy { skip, overwrite }

extension ImportFormatX on ImportFormat {
  String get label => switch (this) {
        ImportFormat.csvGeneric => 'CSV (Generic)',
        ImportFormat.csvChrome => 'CSV (Chrome/Edge)',
        ImportFormat.csvLastPass => 'CSV (LastPass)',
        ImportFormat.csv1Password => 'CSV (1Password)',
        ImportFormat.bitwarden => 'Bitwarden JSON',
        ImportFormat.psono => 'Psono JSON',
        ImportFormat.onepassword1pux => '1Password (1PUX)',
      };

  /// Format identifier sent to the server job ledger (POST /jobs).
  String get apiFormat => switch (this) {
        ImportFormat.csvGeneric => 'csv-generic',
        ImportFormat.csvChrome => 'csv-chrome',
        ImportFormat.csvLastPass => 'csv-lastpass',
        ImportFormat.csv1Password => 'csv-1password',
        ImportFormat.bitwarden => 'bitwarden',
        ImportFormat.psono => 'psono',
        ImportFormat.onepassword1pux => 'onepassword',
      };
}

extension ExportFormatX on ExportFormat {
  String get label => switch (this) {
        ExportFormat.csv => 'CSV (Generic)',
        ExportFormat.bitwarden => 'Bitwarden JSON',
        ExportFormat.onepasswordCsv => '1Password CSV',
        ExportFormat.onepassword1pux => '1Password (1PUX)',
      };

  String get filename => switch (this) {
        ExportFormat.csv => 'passbubble-export.csv',
        ExportFormat.bitwarden => 'passbubble-export.json',
        ExportFormat.onepasswordCsv => 'passbubble-export.csv',
        ExportFormat.onepassword1pux => 'passbubble-export.1pux',
      };

  String get mimeType => switch (this) {
        ExportFormat.csv => 'text/csv',
        ExportFormat.bitwarden => 'application/json',
        ExportFormat.onepasswordCsv => 'text/csv',
        ExportFormat.onepassword1pux => 'application/zip',
      };

  /// Format identifier sent to the server job ledger (POST /jobs).
  String get apiFormat => switch (this) {
        ExportFormat.csv => 'csv-generic',
        ExportFormat.bitwarden => 'bitwarden',
        ExportFormat.onepasswordCsv => 'onepassword',
        ExportFormat.onepassword1pux => 'onepassword',
      };

  bool get supportsFiles =>
      this == ExportFormat.bitwarden || this == ExportFormat.onepassword1pux;
}

/// Options for an export job (file-attachment handling).
class ExportOptions {
  final bool includeFiles;
  final bool filesAsBase64;
  const ExportOptions({this.includeFiles = false, this.filesAsBase64 = false});
}

/// Terminal/running lifecycle states for a [LocalJob].
enum JobState { running, completed, failed }

/// In-session state for an import/export job owned by the [JobRunner].
///
/// Lives in the app-root provider container (not a widget), so the work keeps
/// running — and the progress stays observable — when the user navigates away
/// from the Manage screen. The per-step [log] is client-only: the server `jobs`
/// table has no log column, so it exists only for jobs run this session.
class LocalJob {
  final String id;
  final String type; // "import" | "export"
  final String format; // apiFormat sent to the ledger
  final JobState state;

  final int totalItems;
  final int processedItems;
  final int createdItems;
  final int updatedItems;
  final int skippedItems;
  final int failedItems;

  final String statusText;
  final List<String> log;
  final List<String> warnings;
  final String? errorMessage;
  final DateTime createdAt;

  // Export result, kept so the completion SnackBar / detail sheet can re-share.
  final Uint8List? resultBytes;
  final String? resultFilename;
  final String? resultMime;

  const LocalJob({
    required this.id,
    required this.type,
    required this.format,
    required this.state,
    required this.createdAt,
    this.totalItems = 0,
    this.processedItems = 0,
    this.createdItems = 0,
    this.updatedItems = 0,
    this.skippedItems = 0,
    this.failedItems = 0,
    this.statusText = '',
    this.log = const [],
    this.warnings = const [],
    this.errorMessage,
    this.resultBytes,
    this.resultFilename,
    this.resultMime,
  });

  /// Adapts a server-history job (no client-side log/result) for uniform display
  /// in the Job View / detail sheet.
  factory LocalJob.fromServer(JobResponse r) {
    final state = switch (r.status) {
      'completed' => JobState.completed,
      'failed' => JobState.failed,
      _ => r.status == 'running' ? JobState.running : JobState.completed,
    };
    return LocalJob(
      id: r.id,
      type: r.type,
      format: r.format,
      state: state,
      createdAt: DateTime.tryParse(r.createdAt)?.toLocal() ?? DateTime.now(),
      totalItems: r.totalItems,
      processedItems: r.processedItems,
      createdItems: r.createdItems,
      updatedItems: r.updatedItems,
      skippedItems: r.skippedItems,
      failedItems: r.failedItems,
      errorMessage: r.errorMessage,
    );
  }

  bool get isRunning => state == JobState.running;
  bool get isFailed => state == JobState.failed;
  bool get isCompleted => state == JobState.completed;

  /// Server-style status string (matches [JobResponse.status]).
  String get status => switch (state) {
        JobState.running => 'running',
        JobState.completed => 'completed',
        JobState.failed => 'failed',
      };

  double get progressFraction =>
      totalItems > 0 ? processedItems / totalItems : 0.0;

  LocalJob copyWith({
    JobState? state,
    int? totalItems,
    int? processedItems,
    int? createdItems,
    int? updatedItems,
    int? skippedItems,
    int? failedItems,
    String? statusText,
    List<String>? log,
    List<String>? warnings,
    String? errorMessage,
    Uint8List? resultBytes,
    String? resultFilename,
    String? resultMime,
  }) {
    return LocalJob(
      id: id,
      type: type,
      format: format,
      createdAt: createdAt,
      state: state ?? this.state,
      totalItems: totalItems ?? this.totalItems,
      processedItems: processedItems ?? this.processedItems,
      createdItems: createdItems ?? this.createdItems,
      updatedItems: updatedItems ?? this.updatedItems,
      skippedItems: skippedItems ?? this.skippedItems,
      failedItems: failedItems ?? this.failedItems,
      statusText: statusText ?? this.statusText,
      log: log ?? this.log,
      warnings: warnings ?? this.warnings,
      errorMessage: errorMessage ?? this.errorMessage,
      resultBytes: resultBytes ?? this.resultBytes,
      resultFilename: resultFilename ?? this.resultFilename,
      resultMime: resultMime ?? this.resultMime,
    );
  }
}

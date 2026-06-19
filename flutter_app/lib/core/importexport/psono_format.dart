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

import 'dart:convert';

import 'entry_record.dart';

/// Result of parsing a Psono export, including any parse warnings.
class PsonoParseResult {
  final List<EntryRecord> records;
  final List<String> warnings;

  const PsonoParseResult({required this.records, required this.warnings});
}

/// Parses a Psono unencrypted JSON export.
/// Mirrors backend/pkg/importers/psono.go.
PsonoParseResult parsePsono(String content) {
  final Map<String, dynamic> json = jsonDecode(content) as Map<String, dynamic>;
  final records = <EntryRecord>[];
  final warnings = <String>[];

  // Root-level items
  final rootItems = json['items'] as List? ?? [];
  for (final item in rootItems) {
    _convertItem(item as Map<String, dynamic>, records, warnings, const []);
  }

  // Recursive folders
  final folders = json['folders'] as List? ?? [];
  for (final folder in folders) {
    _walkFolder(folder as Map<String, dynamic>, records, warnings, const []);
  }

  return PsonoParseResult(records: records, warnings: warnings);
}

void _walkFolder(
  Map<String, dynamic> folder,
  List<EntryRecord> records,
  List<String> warnings,
  List<String> parentPath,
) {
  final folderName = folder['name'] as String? ?? '';
  final path = folderName.isNotEmpty ? [...parentPath, folderName] : parentPath;

  final items = folder['items'] as List? ?? [];
  for (final item in items) {
    _convertItem(item as Map<String, dynamic>, records, warnings, path);
  }
  final subFolders = folder['folders'] as List? ?? [];
  for (final sub in subFolders) {
    _walkFolder(sub as Map<String, dynamic>, records, warnings, path);
  }
}

void _convertItem(
  Map<String, dynamic> item,
  List<EntryRecord> records,
  List<String> warnings,
  List<String> folderPath,
) {
  String s(String key) => item[key] as String? ?? '';

  final entryType = s('type');
  final name = s('name');
  final created = normalizeImportDate(s('create_date'));
  final updated = normalizeImportDate(s('write_date'));

  EntryRecord? rec;

  switch (entryType) {
    case 'website_password':
      final url = s('website_password_url');
      final rawTitle = s('website_password_title').isNotEmpty ? s('website_password_title') : name;
      final title = rawTitle.isNotEmpty ? rawTitle : url;
      final totp = s('website_password_totp_code');
      rec = EntryRecord(
        name: title,
        type: totp.isNotEmpty ? 'totp' : 'password',
        url: url,
        username: s('website_password_username'),
        password: s('website_password_password'),
        totpSecret: totp,
        notes: s('website_password_notes'),
        customFields: _parseCustomFields(item),
        folderPath: folderPath,
        createdAt: created,
        updatedAt: updated,
      );

    case 'application_password':
      final rawTitle =
          s('application_password_title').isNotEmpty ? s('application_password_title') : name;
      rec = EntryRecord(
        name: rawTitle,
        type: 'password',
        username: s('application_password_username'),
        password: s('application_password_password'),
        customFields: _parseCustomFields(item),
        folderPath: folderPath,
        createdAt: created,
        updatedAt: updated,
      );

    case 'note':
      final rawTitle = s('note_title').isNotEmpty ? s('note_title') : name;
      rec = EntryRecord(
        name: rawTitle,
        type: 'note',
        notes: s('note_notes'),
        customFields: _parseCustomFields(item),
        folderPath: folderPath,
        createdAt: created,
        updatedAt: updated,
      );

    case 'bookmark':
      final url = s('bookmark_url');
      final rawTitle = s('bookmark_title').isNotEmpty ? s('bookmark_title') : name;
      final title = rawTitle.isNotEmpty ? rawTitle : url;
      rec = EntryRecord(
        name: title,
        type: 'password',
        url: url,
        customFields: _parseCustomFields(item),
        folderPath: folderPath,
        createdAt: created,
        updatedAt: updated,
      );

    case 'totp':
      final rawTitle = s('totp_title').isNotEmpty ? s('totp_title') : name;
      rec = EntryRecord(
        name: rawTitle,
        type: 'totp',
        totpSecret: s('totp_code'),
        notes: s('totp_notes'),
        customFields: _parseCustomFields(item),
        folderPath: folderPath,
        createdAt: created,
        updatedAt: updated,
      );

    default:
      if (entryType.isNotEmpty) {
        warnings.add('Skipping unknown Psono type "$entryType" ($name)');
      }
      return;
  }

  if (rec.name.isEmpty) {
    warnings.add('Skipping unnamed Psono entry (type: $entryType)');
    return;
  }

  records.add(rec);
}

List<CustomFieldRecord> _parseCustomFields(Map<String, dynamic> item) {
  final raw = item['custom_fields'] as List? ?? [];
  return raw
      .whereType<Map<String, dynamic>>()
      .map((cf) => CustomFieldRecord(
            label: cf['name'] as String? ?? '',
            value: cf['value'] as String? ?? '',
          ))
      .where((cf) => cf.label.isNotEmpty)
      .toList();
}

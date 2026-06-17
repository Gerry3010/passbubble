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

import 'entry_record.dart';

enum CsvImportFormat { generic, chrome, lastpass, onePassword }

/// Parses a CSV file into [EntryRecord]s.
/// Mirrors backend/pkg/importers/csv.go.
List<EntryRecord> parseCsv(String content, CsvImportFormat format) {
  final lines = _splitCsv(content);
  if (lines.length < 2) return [];

  final records = <EntryRecord>[];
  for (final row in lines.skip(1)) {
    if (row.isEmpty) continue;
    final rec = _convertRow(row, format);
    if (rec != null) records.add(rec);
  }
  return records;
}

EntryRecord? _convertRow(List<String> row, CsvImportFormat format) {
  String col(int i) => i < row.length ? row[i].trim() : '';

  switch (format) {
    case CsvImportFormat.chrome:
      // name, url, username, password
      final name = col(0).isNotEmpty ? col(0) : col(1);
      if (name.isEmpty) return null;
      return EntryRecord(name: name, url: col(1), username: col(2), password: col(3));

    case CsvImportFormat.lastpass:
      // url, username, password, totp, extra, name, grouping, fav
      final name = col(5).isNotEmpty ? col(5) : col(0);
      if (name.isEmpty) return null;
      final totp = col(3);
      return EntryRecord(
        name: name,
        url: col(0),
        username: col(1),
        password: col(2),
        totpSecret: totp,
        notes: col(4),
        type: totp.isNotEmpty ? 'totp' : 'password',
      );

    case CsvImportFormat.onePassword:
      // Title, Username, Password, URL, Notes, Type
      final name = col(0);
      if (name.isEmpty) return null;
      return EntryRecord(
        name: name,
        username: col(1),
        password: col(2),
        url: col(3),
        notes: col(4),
        type: _map1PasswordType(col(5)),
      );

    case CsvImportFormat.generic:
    // name, url, username, password, notes
      final name = col(0).isNotEmpty ? col(0) : col(1);
      if (name.isEmpty) return null;
      return EntryRecord(
        name: name,
        url: col(1),
        username: col(2),
        password: col(3),
        notes: col(4),
      );
  }
}

String _map1PasswordType(String t) {
  switch (t.toLowerCase()) {
    case 'login':
      return 'password';
    case 'secure note':
    case 'note':
      return 'note';
    case 'credit card':
      return 'credit-card';
    case 'identity':
      return 'identity';
    default:
      return 'password';
  }
}

/// Exports entries to generic CSV (name, url, username, password, notes).
String exportCsv(List<EntryRecord> records) {
  final buf = StringBuffer();
  buf.writeln('name,url,username,password,notes');
  for (final r in records) {
    buf.writeln([r.name, r.url, r.username, r.password, r.notes]
        .map(_csvEscape)
        .join(','));
  }
  return buf.toString();
}

/// Exports entries to 1Password CSV format (Title, Username, Password, URL, Notes, Type).
/// File attachments are skipped; CSV cannot carry binary data.
String export1PasswordCsv(List<EntryRecord> records) {
  final buf = StringBuffer();
  buf.writeln('Title,Username,Password,URL,Notes,Type');
  for (final r in records) {
    buf.writeln([r.name, r.username, r.password, r.url, r.notes, _to1PType(r.type)]
        .map(_csvEscape)
        .join(','));
  }
  return buf.toString();
}

String _to1PType(String t) => switch (t) {
      'note' => 'Secure Note',
      'credit-card' => 'Credit Card',
      'identity' => 'Identity',
      _ => 'Login',
    };

String _csvEscape(String s) {
  if (s.contains(',') || s.contains('"') || s.contains('\n')) {
    return '"${s.replaceAll('"', '""')}"';
  }
  return s;
}

/// Minimal RFC 4180 CSV parser (handles quoted fields with embedded commas/newlines).
List<List<String>> _splitCsv(String content) {
  final rows = <List<String>>[];
  final fields = <String>[];
  final field = StringBuffer();
  var inQuotes = false;

  for (var i = 0; i < content.length; i++) {
    final ch = content[i];
    if (inQuotes) {
      if (ch == '"') {
        if (i + 1 < content.length && content[i + 1] == '"') {
          field.write('"');
          i++;
        } else {
          inQuotes = false;
        }
      } else {
        field.write(ch);
      }
    } else {
      if (ch == '"') {
        inQuotes = true;
      } else if (ch == ',') {
        fields.add(field.toString());
        field.clear();
      } else if (ch == '\n' || (ch == '\r' && i + 1 < content.length && content[i + 1] == '\n')) {
        if (ch == '\r') i++;
        fields.add(field.toString());
        field.clear();
        if (fields.isNotEmpty) rows.add(List<String>.from(fields));
        fields.clear();
      } else {
        field.write(ch);
      }
    }
  }
  if (field.isNotEmpty || fields.isNotEmpty) {
    fields.add(field.toString());
    rows.add(List<String>.from(fields));
  }
  return rows;
}

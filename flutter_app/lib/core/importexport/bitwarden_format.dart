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

/// Parses a Bitwarden unencrypted JSON export.
/// Mirrors backend/pkg/importers/bitwarden.go.
List<EntryRecord> parseBitwarden(String content) {
  final Map<String, dynamic> json = jsonDecode(content) as Map<String, dynamic>;
  final items = json['items'] as List? ?? [];
  final records = <EntryRecord>[];

  for (final item in items) {
    final m = item as Map<String, dynamic>;
    final rec = _convertItem(m);
    if (rec != null) records.add(rec);
  }
  return records;
}

EntryRecord? _convertItem(Map<String, dynamic> m) {
  final name = m['name'] as String? ?? '';
  final notes = m['notes'] as String? ?? '';
  final type = m['type'] as int? ?? 1;

  final fieldsList = (m['fields'] as List? ?? [])
      .whereType<Map<String, dynamic>>()
      .where((f) => (f['name'] as String? ?? '').isNotEmpty)
      .map((f) {
        final bwType = f['type'] as int? ?? 0;
        return CustomFieldRecord(
          label: f['name'] as String? ?? '',
          value: f['value'] as String? ?? '',
          type: bwType == 1 ? CustomFieldType.password : CustomFieldType.text,
        );
      })
      .toList();

  switch (type) {
    case 1: // login
      final login = m['login'] as Map<String, dynamic>?;
      final uris = login?['uris'] as List? ?? [];
      final url = uris.isNotEmpty ? (uris[0] as Map)['uri'] as String? ?? '' : '';
      final totp = login?['totp'] as String? ?? '';
      return EntryRecord(
        name: name.isNotEmpty ? name : (login?['username'] as String? ?? ''),
        url: url,
        type: totp.isNotEmpty ? 'totp' : 'password',
        username: login?['username'] as String? ?? '',
        password: login?['password'] as String? ?? '',
        totpSecret: totp,
        notes: notes,
        customFields: fieldsList,
      );

    case 2: // secure note
      return EntryRecord(name: name, type: 'note', notes: notes, customFields: fieldsList);

    case 3: // card
      final card = m['card'] as Map<String, dynamic>?;
      return EntryRecord(
        name: name,
        type: 'credit-card',
        notes: notes,
        cardNumber: card?['number'] as String? ?? '',
        holderName: card?['cardholderName'] as String? ?? '',
        expiryMonth: card?['expMonth'] as String? ?? '',
        expiryYear: card?['expYear'] as String? ?? '',
        cvv: card?['code'] as String? ?? '',
        customFields: fieldsList,
      );

    case 4: // identity
      final id = m['identity'] as Map<String, dynamic>?;
      return EntryRecord(
        name: name,
        type: 'identity',
        notes: notes,
        firstName: id?['firstName'] as String? ?? '',
        lastName: id?['lastName'] as String? ?? '',
        company: id?['company'] as String? ?? '',
        email: id?['email'] as String? ?? '',
        phone: id?['phone'] as String? ?? '',
        street: id?['address1'] as String? ?? '',
        city: id?['city'] as String? ?? '',
        state: id?['state'] as String? ?? '',
        postalCode: id?['postalCode'] as String? ?? '',
        country: id?['country'] as String? ?? '',
        customFields: fieldsList,
      );

    default:
      return null;
  }
}

class BitwardenExportOptions {
  final bool includeFiles;
  final bool filesAsBase64;
  const BitwardenExportOptions({this.includeFiles = false, this.filesAsBase64 = false});
}

/// Exports entries to Bitwarden JSON format.
String exportBitwarden(List<EntryRecord> records, [BitwardenExportOptions opts = const BitwardenExportOptions()]) {
  final items = records.map((r) {
    final item = <String, dynamic>{
      'name': r.name,
      'notes': r.notes.isNotEmpty ? r.notes : null,
    };

    switch (r.type) {
      case 'totp':
      case 'password':
        item['type'] = 1;
        item['login'] = {
          'username': r.username,
          'password': r.password,
          if (r.totpSecret.isNotEmpty) 'totp': r.totpSecret,
          if (r.url.isNotEmpty)
            'uris': [
              {'uri': r.url}
            ],
        };
      case 'note':
        item['type'] = 2;
      case 'credit-card':
        item['type'] = 3;
        item['card'] = {
          'cardholderName': r.holderName,
          'number': r.cardNumber,
          'expMonth': r.expiryMonth,
          'expYear': r.expiryYear,
          'code': r.cvv,
        };
      default:
        item['type'] = 1;
        item['login'] = {'username': r.username, 'password': r.password};
    }

    final exportedFields = <Map<String, dynamic>>[];
    for (final f in r.customFields) {
      if (f.type == CustomFieldType.file) {
        if (!opts.includeFiles) continue;
        if (opts.filesAsBase64) {
          final mime = f.mimeType ?? 'application/octet-stream';
          exportedFields.add({
            'name': f.filename ?? f.label,
            'value': 'data:$mime;base64,${f.value}',
            'type': 1,
          });
        }
        continue;
      }
      final bwType = (f.type == CustomFieldType.password ||
              f.type == CustomFieldType.ssh ||
              f.type == CustomFieldType.totp)
          ? 1
          : 0;
      exportedFields.add({'name': f.label, 'value': f.value, 'type': bwType});
    }
    if (exportedFields.isNotEmpty) {
      item['fields'] = exportedFields;
    }

    return item;
  }).toList();

  return const JsonEncoder.withIndent('  ').convert({
    'encrypted': false,
    'items': items,
  });
}

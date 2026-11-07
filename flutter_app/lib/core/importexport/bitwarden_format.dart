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

import '../util/entry_tags.dart';
import '../util/entry_totp.dart';
import 'entry_record.dart';

/// Bitwarden has no tags, so they travel in a text custom field of this name.
const bitwardenTagsField = 'passbubble_tags';

const _fieldText = 0;
const _fieldHidden = 1;

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
  final created = normalizeImportDate(m['creationDate'] as String? ?? '');
  final updated = normalizeImportDate(m['revisionDate'] as String? ?? '');

  final rawFields = (m['fields'] as List? ?? [])
      .whereType<Map<String, dynamic>>()
      .where((f) => (f['name'] as String? ?? '').isNotEmpty)
      .toList();
  final tags = normalizeTags(rawFields
      .where((f) => f['name'] == bitwardenTagsField)
      .expand((f) => splitTags(f['value'] as String? ?? '')));
  final fieldsList = rawFields
      .where((f) => f['name'] != bitwardenTagsField)
      .map(_customField)
      .toList();

  switch (type) {
    case 1: // login
      final login = m['login'] as Map<String, dynamic>?;
      final uris = login?['uris'] as List? ?? [];
      final url = uris.isNotEmpty ? (uris[0] as Map)['uri'] as String? ?? '' : '';
      final password = login?['password'] as String? ?? '';
      final totp = login?['totp'] as String? ?? '';
      return EntryRecord(
        name: name.isNotEmpty ? name : (login?['username'] as String? ?? ''),
        url: url,
        type: loginType(password, totp),
        username: login?['username'] as String? ?? '',
        password: password,
        totpSecret: totp,
        notes: notes,
        customFields: fieldsList,
        tags: tags,
        createdAt: created,
        updatedAt: updated,
      );

    case 2: // secure note
      return EntryRecord(
          name: name,
          type: 'note',
          notes: notes,
          customFields: fieldsList,
          tags: tags,
          createdAt: created,
          updatedAt: updated);

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
        tags: tags,
        createdAt: created,
        updatedAt: updated,
      );

    case 4: // identity
      final id = m['identity'] as Map<String, dynamic>?;
      return EntryRecord(
        name: name,
        type: 'identity',
        notes: notes,
        title: id?['title'] as String? ?? '',
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
        tags: tags,
        createdAt: created,
        updatedAt: updated,
      );

    default:
      return null;
  }
}

/// Maps a field to its kind, as far as the format tells it: a hidden field is a
/// password, or a file when it holds a base64 data: URI (how the exporter writes
/// files). Other field types are plain text.
/// Mirrors bitwardenCustomField in backend/pkg/importers/bitwarden.go.
CustomFieldRecord _customField(Map<String, dynamic> f) {
  final label = f['name'] as String? ?? '';
  final value = f['value'] as String? ?? '';
  if (f['type'] != _fieldHidden) return CustomFieldRecord(label: label, value: value);
  final file = _splitBase64DataUri(value);
  if (file == null) {
    return CustomFieldRecord(label: label, value: value, type: CustomFieldType.password);
  }
  return CustomFieldRecord(
    label: label,
    value: file.content,
    type: CustomFieldType.file,
    filename: label,
    mimeType: file.mime.isEmpty ? null : file.mime,
  );
}

/// Splits `data:<mime>;base64,<content>`, or returns null for anything else.
({String mime, String content})? _splitBase64DataUri(String value) {
  const marker = ';base64,';
  if (!value.startsWith('data:')) return null;
  final at = value.indexOf(marker);
  if (at < 0) return null;
  // Line breaks are allowed, as in Go, but the app's decoder would trip on them.
  final content = value.substring(at + marker.length).replaceAll(RegExp(r'[\r\n]'), '');
  // Dart's decoder also takes the URL-safe alphabet, which is not this format.
  if (!RegExp(r'^[A-Za-z0-9+/]*={0,2}$').hasMatch(content)) return null;
  try {
    base64.decode(content);
  } on FormatException {
    return null;
  }
  return (mime: value.substring('data:'.length, at), content: content);
}

/// JSON keys of the entry data that each item type has a place for. Everything
/// else an entry holds goes into custom fields, see [_knownFields] and
/// [_unknownFields].
const _loginKeys = {'username', 'password', totpSecretKey, 'url'};
const _cardKeys = {'card_number', 'holder_name', 'expiry_month', 'expiry_year', 'cvv'};
const _identityKeys = {
  'title',
  'first_name',
  'last_name',
  'company',
  'email',
  'phone',
  'street',
  'city',
  'state',
  'postal_code',
  'country',
};

/// JSON keys of the entry data this client knows, in the order the exports write
/// them, with whether the value is secret. Same list as `keyedFields` in
/// backend/pkg/exporters/fields.go.
const _knownKeys = <String, bool>{
  'url': false,
  'username': false,
  'password': true,
  totpSecretKey: true,
  'sign_in_with': false,
  'issuer': false,
  'period': false,
  'digits': false,
  'algorithm': false,
  'card_number': true,
  'holder_name': false,
  'expiry_month': false,
  'expiry_year': false,
  'cvv': true,
  'bank_name': false,
  'iban': true,
  'bic': false,
  'account_number': true,
  'account_type': false,
  'title': false,
  'first_name': false,
  'last_name': false,
  'company': false,
  'email': false,
  'phone': false,
  'street': false,
  'city': false,
  'state': false,
  'postal_code': false,
  'country': false,
  'product_name': false,
  'license_key': true,
  'purchase_email': false,
  'purchase_date': false,
  'expires_at': false,
};

Map<String, dynamic> _field(String name, Object? value, {required bool hidden}) => {
      'name': name,
      'value': value is String ? value : jsonEncode(value),
      'type': hidden ? _fieldHidden : _fieldText,
    };

/// Custom fields, named after their JSON key, for the [values] of known keys that
/// the item has no place for. The item itself holds the keys in [held].
List<Map<String, dynamic>> _knownFields(Map<String, Object?> values, Set<String> held) => [
      for (final MapEntry(key: name, value: secret) in _knownKeys.entries)
        if (!held.contains(name) && values.containsKey(name))
          _field(name, values[name], hidden: secret),
    ];

/// Custom fields for the [values] of keys nobody here can vouch for, so they count
/// as secret. The notes are no such key: every item has a place for them.
List<Map<String, dynamic>> _unknownFields(Map<String, Object?> values) => [
      for (final MapEntry(key: name, :value) in values.entries)
        if (name != 'notes' && !_knownKeys.containsKey(name)) _field(name, value, hidden: true),
    ];

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

    final Set<String> held;
    switch (r.type) {
      case 'note':
        item['type'] = 2;
        held = const {};
      case 'credit-card':
        item['type'] = 3;
        item['card'] = {
          'cardholderName': r.holderName,
          'number': r.cardNumber,
          'expMonth': r.expiryMonth,
          'expYear': r.expiryYear,
          'code': r.cvv,
        };
        held = _cardKeys;
      case 'identity':
        item['type'] = 4;
        item['identity'] = {
          'title': r.title,
          'firstName': r.firstName,
          'lastName': r.lastName,
          'company': r.company,
          'email': r.email,
          'phone': r.phone,
          'address1': r.street,
          'city': r.city,
          'state': r.state,
          'postalCode': r.postalCode,
          'country': r.country,
        };
        held = _identityKeys;
      default: // password, totp, api-key, ssh-key, license, bank-account, ...
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
        held = _loginKeys;
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
            'type': _fieldHidden,
          });
        }
        continue;
      }
      final bwType = (f.type == CustomFieldType.password ||
              f.type == CustomFieldType.ssh ||
              f.type == CustomFieldType.totp)
          ? _fieldHidden
          : _fieldText;
      exportedFields.add({'name': f.label, 'value': f.value, 'type': bwType});
    }
    final values = <String, Object?>{
      if (r.url.isNotEmpty) 'url': r.url,
      ...r.dataValues,
      ...r.extraData,
    };
    exportedFields.addAll(_knownFields(values, held));
    // A login is what the format makes of the types it does not know, so the file
    // says what the entry really was.
    if (item['type'] == 1 && !const {'', 'password', 'totp'}.contains(r.type)) {
      exportedFields.add(_field('type', r.type, hidden: false));
    }
    exportedFields.addAll(_unknownFields(values));
    final tags = joinTags(r.tags);
    if (tags.isNotEmpty) {
      exportedFields.add({'name': bitwardenTagsField, 'value': tags, 'type': _fieldText});
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

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
import 'dart:typed_data';

import 'package:archive/archive.dart';
import 'package:uuid/uuid.dart';

import 'entry_record.dart';

const _uuid = Uuid();

// ── Export ────────────────────────────────────────────────────────────────────

/// Serialises [records] to a 1PUX ZIP archive (Uint8List).
/// Save the result as a *.1pux file.
Uint8List exportOnePux(List<EntryRecord> records) {
  final now = DateTime.now().millisecondsSinceEpoch ~/ 1000;
  final fileBlobs = <String, Uint8List>{};
  final items = records.map((r) => _buildItem(r, now, fileBlobs)).toList();

  final exportData = {
    'accounts': [
      {
        'attrs': {
          'accountName': 'Passbubble',
          'name': 'Passbubble Export',
          'createdAt': now,
          'lastAuthAt': now,
        },
        'vaults': [
          {
            'attrs': {
              'uuid': _uuid.v4(),
              'desc': '',
              'avatar': '',
              'name': 'Personal',
              'type': 'P',
              'createdAt': now,
            },
            'items': items,
          }
        ],
      }
    ],
  };

  final exportJson = const JsonEncoder.withIndent('  ').convert(exportData);

  final archive = Archive();
  archive.addFile(ArchiveFile(
    'export.data',
    exportJson.length,
    Uint8List.fromList(utf8.encode(exportJson)),
  ));
  for (final entry in fileBlobs.entries) {
    archive.addFile(ArchiveFile('files/${entry.key}', entry.value.length, entry.value));
  }

  return Uint8List.fromList(ZipEncoder().encode(archive)!);
}

Map<String, dynamic> _buildItem(EntryRecord rec, int now, Map<String, Uint8List> fileBlobs) {
  final id = _uuid.v4();
  final overview = <String, dynamic>{
    'title': rec.name,
    'tags': <String>[],
    if (rec.url.isNotEmpty)
      'urls': [
        {'label': 'website', 'url': rec.url}
      ],
  };

  final details = <String, dynamic>{
    'notesPlain': rec.notes,
    'passwordHistory': <dynamic>[],
  };

  String categoryUuid;
  final sections = <Map<String, dynamic>>[];
  final fileRefs = <Map<String, dynamic>>[];

  switch (rec.type) {
    case 'note':
      categoryUuid = '003';

    case 'credit-card':
      categoryUuid = '002';
      sections.add({
        'title': 'Card Details',
        'fields': [
          _sfield('card_number', 'Card Number', {'creditCardNumber': rec.cardNumber}),
          _sfield('holder_name', 'Cardholder Name', {'string': rec.holderName}),
          _sfield('expiry_month', 'Expiry Month', {'string': rec.expiryMonth}),
          _sfield('expiry_year', 'Expiry Year', {'string': rec.expiryYear}),
          _sfield('cvv', 'CVV', {'concealed': rec.cvv}),
        ],
      });
      overview['subtitle'] = rec.holderName;

    case 'identity':
      categoryUuid = '004';
      sections.addAll([
        {
          'title': 'Name',
          'fields': [
            _sfield('firstname', 'First Name', {'string': rec.firstName}),
            _sfield('lastname', 'Last Name', {'string': rec.lastName}),
            _sfield('company', 'Company', {'string': rec.company}),
          ],
        },
        {
          'title': 'Address',
          'fields': [
            _sfield('street', 'Street', {'string': rec.street}),
            _sfield('city', 'City', {'string': rec.city}),
            _sfield('state', 'State', {'string': rec.state}),
            _sfield('postal_code', 'Postal Code', {'string': rec.postalCode}),
            _sfield('country', 'Country', {'string': rec.country}),
          ],
        },
        {
          'title': 'Contact',
          'fields': [
            _sfield('email', 'Email', {'email': rec.email}),
            _sfield('phone', 'Phone', {'phone': rec.phone}),
          ],
        },
      ]);
      overview['subtitle'] = '${rec.firstName} ${rec.lastName}'.trim();

    default: // Login
      categoryUuid = '001';
      details['loginFields'] = [
        {'value': rec.username, 'id': 'username', 'name': 'username', 'fieldType': 'T', 'designation': 'username'},
        {'value': rec.password, 'id': 'password', 'name': 'password', 'fieldType': 'P', 'designation': 'password'},
      ];
      overview['subtitle'] = rec.username;
      overview['ainfo'] = rec.username;
      if (rec.totpSecret.isNotEmpty) {
        sections.add({
          'title': 'One-Time Password',
          'fields': [_sfield('totp', 'One-Time Password', {'totp': rec.totpSecret})],
        });
      }
      if (rec.type == 'license') {
        sections.add({
          'title': 'License',
          'fields': [
            _sfield('product_name', 'Product Name', {'string': rec.productName}),
            _sfield('license_key', 'License Key', {'concealed': rec.licenseKey}),
          ],
        });
      }
  }

  // Custom fields
  final customSectionFields = <Map<String, dynamic>>[];
  for (final cf in rec.customFields) {
    if (cf.type == CustomFieldType.file) {
      final raw = base64Decode(cf.value);
      final docID = _uuid.v4();
      final fname = cf.filename ?? cf.label;
      fileBlobs[docID] = Uint8List.fromList(raw);
      fileRefs.add({
        'documentAttributes': {
          'fileName': fname,
          'documentId': docID,
          'decryptedSize': raw.length,
        },
        'overview': {'fileName': fname},
      });
      continue;
    }
    final valueMap = _cfTypeToValue(cf.type, cf.value);
    customSectionFields.add({
      ..._sfield(_uuid.v4(), cf.label, valueMap),
      'multiline': cf.type == CustomFieldType.note || cf.type == CustomFieldType.ssh,
    });
  }
  if (customSectionFields.isNotEmpty) {
    sections.add({'title': 'Custom Fields', 'fields': customSectionFields});
  }

  if (sections.isNotEmpty) {
    details['sections'] = sections;
  }

  return {
    'uuid': id,
    'favIndex': 0,
    'createdAt': now,
    'updatedAt': now,
    'trashed': 'N',
    'categoryUuid': categoryUuid,
    'details': details,
    'overview': overview,
    if (fileRefs.isNotEmpty) 'files': fileRefs,
  };
}

Map<String, dynamic> _sfield(String id, String title, Map<String, dynamic> value) => {
      'title': title,
      'id': id,
      'value': value,
      'indexAtSource': 0,
      'guarded': false,
      'multiline': false,
      'dontGenerate': false,
      'inputTraits': {'keyboard': 'default', 'correction': 'default', 'capitalization': 'default'},
    };

Map<String, dynamic> _cfTypeToValue(CustomFieldType type, String value) => switch (type) {
      CustomFieldType.password || CustomFieldType.ssh => {'concealed': value},
      CustomFieldType.totp => {'totp': value},
      CustomFieldType.url => {'url': value},
      CustomFieldType.email => {'email': value},
      CustomFieldType.phone => {'phone': value},
      _ => {'string': value},
    };

// ── Import ────────────────────────────────────────────────────────────────────

class OnePuxParseResult {
  final List<EntryRecord> records;
  final List<String> warnings;
  const OnePuxParseResult({required this.records, required this.warnings});
}

/// Parses a 1PUX ZIP archive from [zipBytes] into [EntryRecord]s.
OnePuxParseResult parseOnePux(Uint8List zipBytes) {
  final archive = ZipDecoder().decodeBytes(zipBytes);
  final records = <EntryRecord>[];
  final warnings = <String>[];

  // Collect file blobs
  final fileBlobs = <String, Uint8List>{};
  Uint8List? exportDataBytes;

  for (final file in archive) {
    if (!file.isFile) continue;
    final content = Uint8List.fromList(file.content as List<int>);
    if (file.name == 'export.data') {
      exportDataBytes = content;
    } else if (file.name.startsWith('files/')) {
      final docID = file.name.substring('files/'.length);
      if (docID.isNotEmpty) fileBlobs[docID] = content;
    }
  }

  if (exportDataBytes == null) {
    return OnePuxParseResult(records: [], warnings: ['export.data not found in archive']);
  }

  final Map<String, dynamic> root = jsonDecode(utf8.decode(exportDataBytes)) as Map<String, dynamic>;
  for (final acc in (root['accounts'] as List? ?? [])) {
    final accMap = acc as Map<String, dynamic>;
    for (final vault in (accMap['vaults'] as List? ?? [])) {
      final vaultMap = vault as Map<String, dynamic>;
      for (final rawItem in (vaultMap['items'] as List? ?? [])) {
        final result = _convertItem(rawItem as Map<String, dynamic>, fileBlobs);
        if (result == null) continue;
        if (result is String) {
          if (result.isNotEmpty) warnings.add(result);
          continue;
        }
        records.add(result as EntryRecord);
      }
    }
  }

  return OnePuxParseResult(records: records, warnings: warnings);
}

/// Returns an [EntryRecord], a warning [String], or null to skip silently.
/// Converts a 1PUX Unix-seconds timestamp to RFC3339 UTC ('' if absent).
String _unixToRfc3339(dynamic v) {
  final sec = (v as num?)?.toInt() ?? 0;
  if (sec <= 0) return '';
  return DateTime.fromMillisecondsSinceEpoch(sec * 1000, isUtc: true).toIso8601String();
}

dynamic _convertItem(Map<String, dynamic> item, Map<String, Uint8List> fileBlobs) {
  if (item['trashed'] == 'Y') return null;

  final overview = item['overview'] as Map<String, dynamic>? ?? {};
  final details = item['details'] as Map<String, dynamic>? ?? {};
  final categoryUuid = item['categoryUuid'] as String? ?? '001';

  final name = overview['title'] as String? ?? '';
  final notes = details['notesPlain'] as String? ?? '';
  final urls = (overview['urls'] as List? ?? []);
  final url = urls.isNotEmpty ? (urls[0] as Map)['url'] as String? ?? '' : '';

  String type;
  final rec = _RecordBuilder(name: name, url: url, notes: notes);
  rec.createdAt = _unixToRfc3339(item['createdAt']);
  rec.updatedAt = _unixToRfc3339(item['updatedAt']);

  switch (categoryUuid) {
    case '003':
      type = 'note';
    case '002':
      type = 'credit-card';
    case '004':
      type = 'identity';
    default:
      type = 'password';
  }
  rec.type = type;

  // Login fields
  for (final lf in (details['loginFields'] as List? ?? [])) {
    final m = lf as Map<String, dynamic>;
    switch (m['designation'] as String? ?? '') {
      case 'username':
        rec.username = m['value'] as String? ?? '';
      case 'password':
        rec.password = m['value'] as String? ?? '';
    }
  }

  // Sections
  for (final sec in (details['sections'] as List? ?? [])) {
    final secMap = sec as Map<String, dynamic>;
    for (final sf in (secMap['fields'] as List? ?? [])) {
      final sfMap = sf as Map<String, dynamic>;
      final sfId = sfMap['id'] as String? ?? '';
      final sfTitle = sfMap['title'] as String? ?? '';
      final multiline = sfMap['multiline'] as bool? ?? false;
      final valRaw = sfMap['value'] as Map<String, dynamic>? ?? {};

      if (!_mapStructuredField(rec, sfId, valRaw)) {
        final (cfType, cfVal) = _valueToCustomField(valRaw, multiline);
        if (cfVal.isNotEmpty || cfType == CustomFieldType.file) {
          rec.customFields.add(CustomFieldRecord(label: sfTitle, value: cfVal, type: cfType));
        }
      }
    }
  }

  // File attachments
  for (final fileRef in (item['files'] as List? ?? [])) {
    final attrs = (fileRef as Map<String, dynamic>)['documentAttributes'] as Map<String, dynamic>? ?? {};
    final docID = attrs['documentId'] as String? ?? '';
    final fname = attrs['fileName'] as String? ?? '';
    final content = fileBlobs[docID];
    if (content == null) continue;
    rec.customFields.add(CustomFieldRecord(
      label: fname,
      value: base64Encode(content),
      type: CustomFieldType.file,
      filename: fname,
    ));
  }

  // Detect TOTP from custom fields
  for (final cf in rec.customFields) {
    if (cf.type == CustomFieldType.totp && cf.value.isNotEmpty) {
      rec.type = 'totp';
      rec.totpSecret = cf.value;
      break;
    }
  }

  if (rec.name.isEmpty) return 'item has no title';
  return rec.build();
}

bool _mapStructuredField(_RecordBuilder rec, String id, Map<String, dynamic> v) {
  String str(String key) => v[key] as String? ?? '';
  switch (id) {
    case 'username':
      rec.username = str('string');
    case 'password':
      rec.password = str('concealed');
    case 'totp':
      final t = str('totp');
      if (t.isNotEmpty) { rec.totpSecret = t; rec.type = 'totp'; }
    case 'card_number' || 'ccnum':
      rec.cardNumber = str('creditCardNumber').isNotEmpty ? str('creditCardNumber') : str('string');
    case 'holder_name' || 'cardholder':
      rec.holderName = str('string');
    case 'expiry_month':
      rec.expiryMonth = str('string');
    case 'expiry_year':
      rec.expiryYear = str('string');
    case 'cvv' || 'cvvnum':
      rec.cvv = str('concealed');
    case 'firstname':
      rec.firstName = str('string');
    case 'lastname':
      rec.lastName = str('string');
    case 'company':
      rec.company = str('string');
    case 'email':
      rec.email = str('email').isNotEmpty ? str('email') : str('string');
    case 'phone':
      rec.phone = str('phone').isNotEmpty ? str('phone') : str('string');
    case 'street':
      rec.street = str('string');
    case 'city':
      rec.city = str('string');
    case 'state':
      rec.state = str('string');
    case 'postal_code':
      rec.postalCode = str('string');
    case 'country':
      rec.country = str('string');
    default:
      return false;
  }
  return true;
}

(CustomFieldType, String) _valueToCustomField(Map<String, dynamic> v, bool multiline) {
  String str(String key) => v[key] as String? ?? '';
  if (str('concealed').isNotEmpty) return (CustomFieldType.password, str('concealed'));
  if (str('totp').isNotEmpty) return (CustomFieldType.totp, str('totp'));
  if (str('url').isNotEmpty) return (CustomFieldType.url, str('url'));
  if (str('email').isNotEmpty) return (CustomFieldType.email, str('email'));
  if (str('phone').isNotEmpty) return (CustomFieldType.phone, str('phone'));
  if (str('sshKey').isNotEmpty) return (CustomFieldType.ssh, str('sshKey'));
  final s = str('string');
  if (s.isNotEmpty) return (multiline ? CustomFieldType.note : CustomFieldType.text, s);
  return (CustomFieldType.text, '');
}

/// Mutable builder used during parsing before we construct the immutable [EntryRecord].
class _RecordBuilder {
  String name;
  String url;
  String notes;
  String type = 'password';
  String username = '';
  String password = '';
  String totpSecret = '';
  String cardNumber = '';
  String holderName = '';
  String expiryMonth = '';
  String expiryYear = '';
  String cvv = '';
  String firstName = '';
  String lastName = '';
  String company = '';
  String email = '';
  String phone = '';
  String street = '';
  String city = '';
  String state = '';
  String postalCode = '';
  String country = '';
  String licenseKey = '';
  String productName = '';
  String createdAt = '';
  String updatedAt = '';
  List<CustomFieldRecord> customFields = [];

  _RecordBuilder({required this.name, required this.url, required this.notes});

  EntryRecord build() => EntryRecord(
        name: name,
        url: url,
        type: type,
        notes: notes,
        createdAt: createdAt,
        updatedAt: updatedAt,
        username: username,
        password: password,
        totpSecret: totpSecret,
        cardNumber: cardNumber,
        holderName: holderName,
        expiryMonth: expiryMonth,
        expiryYear: expiryYear,
        cvv: cvv,
        firstName: firstName,
        lastName: lastName,
        company: company,
        email: email,
        phone: phone,
        street: street,
        city: city,
        state: state,
        postalCode: postalCode,
        country: country,
        licenseKey: licenseKey,
        productName: productName,
        customFields: customFields,
      );
}

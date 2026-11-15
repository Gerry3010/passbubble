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

// Import of the FIDO Credential Exchange Format (CXF) as JSON, which Apple
// Passwords hands over on iOS and macOS 26 (see docs/passkeys.md). One item
// becomes one entry. The reader is tolerant on purpose: keys in camelCase or
// snake_case, fields as EditableField objects or plain strings, timestamps in
// seconds, milliseconds or RFC 3339. Credential types it does not know are
// counted and reported, never fatal.

import 'dart:convert';

import '../passkeys/passkey.dart';
import '../util/entry_tags.dart';
import 'entry_record.dart';

/// Parses a CXF export: `{version, exporterRpId, exporterDisplayName, timestamp,
/// accounts: [{items: [...]}]}`. A bare account (`{items}`) or a list of
/// accounts or items is accepted too.
({List<EntryRecord> records, List<String> warnings}) parseCxf(String content) {
  final report = _Report();
  final records = <EntryRecord>[];
  for (final item in _itemsOf(jsonDecode(content))) {
    final rec = _convertItem(item, report);
    if (rec != null) records.add(rec);
  }
  return (records: records, warnings: report.warnings);
}

/// The number of items in a CXF export and who exported it, for the import
/// screen. Throws [FormatException] when [content] is not JSON.
({int items, String exporter}) describeCxf(String content) {
  final json = jsonDecode(content);
  final exporter = json is Map
      ? (_str(_pick(json, 'exporterDisplayName')).isNotEmpty
          ? _str(_pick(json, 'exporterDisplayName'))
          : _str(_pick(json, 'exporterRpId')))
      : '';
  return (items: _itemsOf(json).length, exporter: exporter);
}

Iterable<Map<String, dynamic>> _itemsOf(Object? json) sync* {
  Iterable<Object?> accounts;
  if (json is Map && _pick(json, 'accounts') is List) {
    accounts = _pick(json, 'accounts') as List;
  } else if (json is Map && _pick(json, 'items') is List) {
    accounts = [json];
  } else if (json is List) {
    // A list of accounts, or of items.
    accounts = json.any((e) => e is Map && _pick(e, 'items') is List) ? json : [
      {'items': json},
    ];
  } else {
    throw const FormatException('Not a Credential Exchange (CXF) export');
  }
  for (final account in accounts.whereType<Map>()) {
    final items = _pick(account, 'items');
    if (items is! List) continue;
    for (final item in items.whereType<Map>()) {
      yield item.cast<String, dynamic>();
    }
  }
}

// ── Reading values ────────────────────────────────────────────────────────────

/// `m[key]`, or its snake_case spelling.
Object? _pick(Map m, String key) {
  if (m.containsKey(key)) return m[key];
  final snake = key.replaceAllMapped(RegExp(r'[A-Z]'), (x) => '_${x[0]!.toLowerCase()}');
  return m[snake];
}

/// A CXF field's text: an EditableField's `value`, or the value itself.
String _str(Object? v) => switch (v) {
      final String s => s,
      final Map m => _str(m['value']),
      final num n => '$n',
      final bool b => '$b',
      _ => '',
    };

/// `m[key]` as text, see [_str].
String _field(Map m, String key) => _str(_pick(m, key)).trim();

int? _int(Object? v) => switch (v) {
      final num n => n.toInt(),
      final String s => int.tryParse(s.trim()),
      final Map m => _int(m['value']),
      _ => null,
    };

/// A CXF timestamp (seconds since the epoch, or milliseconds, or RFC 3339) as
/// RFC 3339 UTC, or '' when it is absent or unreadable.
String _timestamp(Object? v) {
  if (v is num) {
    final ms = v > 100000000000 ? v.toInt() : (v * 1000).toInt();
    return rfc3339Utc(DateTime.fromMillisecondsSinceEpoch(ms, isUtc: true));
  }
  final s = _str(v).trim();
  if (s.isEmpty) return '';
  final n = num.tryParse(s);
  if (n != null) return _timestamp(n);
  final t = DateTime.tryParse(s);
  return t == null ? '' : rfc3339Utc(t);
}

/// A credential type in the spec's kebab-case: `basicAuth` and `basic_auth`
/// read as `basic-auth`.
String _kind(Map cred) {
  final raw = _str(cred['type'] ?? cred['credentialType'] ?? cred['credential_type']).trim();
  return raw
      .replaceAllMapped(RegExp(r'(?<=[a-z0-9])[A-Z]'), (x) => '-${x[0]}')
      .replaceAll('_', '-')
      .toLowerCase();
}

// ── Items ─────────────────────────────────────────────────────────────────────

/// What the import leaves out, for the job's warnings.
class _Report {
  final unknownTypes = <String, int>{};
  int passkeysUnsupported = 0;
  int passkeysUnreadable = 0;
  int emptyItems = 0;
  int cardsNextToLogin = 0;

  List<String> get warnings => [
        if (unknownTypes.isNotEmpty)
          'Not imported (no matching entry type yet): '
              '${unknownTypes.entries.map((e) => '${e.value}× ${e.key}').join(', ')}.',
        if (passkeysUnsupported > 0)
          '$passkeysUnsupported ${passkeysUnsupported == 1 ? 'passkey was' : 'passkeys were'} '
              'skipped: only ES256 (P-256) passkeys are supported.',
        if (passkeysUnreadable > 0)
          '$passkeysUnreadable ${passkeysUnreadable == 1 ? 'passkey was' : 'passkeys were'} '
              'skipped: its data could not be read.',
        if (cardsNextToLogin > 0)
          '$cardsNextToLogin ${cardsNextToLogin == 1 ? 'card' : 'cards'} stored next to a '
              'login were not imported.',
        if (emptyItems > 0)
          '$emptyItems ${emptyItems == 1 ? 'item had' : 'items had'} nothing to import.',
      ];
}

/// The credentials of one item, collected by the handlers in [_handlers].
class _Collected {
  String username = '';
  String password = '';
  String totpSecret = '';
  final totpOptions = <String, dynamic>{};
  final notes = <String>[];
  final passkeys = <Map<String, dynamic>>[];
  final customFields = <CustomFieldRecord>[];
  String rpId = '';
  bool hasLogin = false;
  Map<String, dynamic>? card;
}

typedef _Handler = void Function(Map cred, _Collected c, _Report report, String createdAt);

/// Credential type → what it adds to the entry. A type missing here is counted
/// in the warnings.
final Map<String, _Handler> _handlers = {
  'basic-auth': (cred, c, _, _) {
    c.hasLogin = true;
    final username = _field(cred, 'username');
    final password = _str(_pick(cred, 'password'));
    if (c.username.isEmpty) {
      c.username = username;
    } else if (username.isNotEmpty && username != c.username) {
      c.customFields.add(CustomFieldRecord(label: 'Username', value: username));
    }
    if (c.password.isEmpty) {
      c.password = password;
    } else if (password.isNotEmpty && password != c.password) {
      c.customFields.add(CustomFieldRecord(
          label: 'Password', value: password, type: CustomFieldType.password));
    }
  },
  'passkey': (cred, c, report, createdAt) {
    c.hasLogin = true;
    final record = _passkeyOf(cred, report, createdAt);
    if (record == null) return;
    c.passkeys.add(record);
    if (c.rpId.isEmpty) c.rpId = record['rp_id'] as String;
    if (c.username.isEmpty) c.username = record['user_name'] as String;
  },
  'totp': (cred, c, _, _) {
    c.hasLogin = true;
    final secret = _field(cred, 'secret').replaceAll(' ', '');
    if (secret.isEmpty) return;
    if (c.username.isEmpty) c.username = _field(cred, 'username');
    if (c.totpSecret.isNotEmpty) {
      c.customFields.add(CustomFieldRecord(
          label: _field(cred, 'issuer').isNotEmpty ? _field(cred, 'issuer') : 'TOTP',
          value: secret,
          type: CustomFieldType.totp));
      return;
    }
    c.totpSecret = secret;
    final period = _int(_pick(cred, 'period'));
    final digits = _int(_pick(cred, 'digits'));
    final algorithm = _field(cred, 'algorithm').toUpperCase().replaceAll('-', '');
    if (period != null && period > 0 && period != 30) c.totpOptions['period'] = period;
    if (digits != null && digits != 6) c.totpOptions['digits'] = digits;
    if (algorithm.isNotEmpty && algorithm != 'SHA1') c.totpOptions['algorithm'] = algorithm;
    final issuer = _field(cred, 'issuer');
    if (issuer.isNotEmpty) c.totpOptions['issuer'] = issuer;
  },
  'note': (cred, c, _, _) {
    final content = _str(_pick(cred, 'content')).trim();
    if (content.isNotEmpty) c.notes.add(content);
  },
  'credit-card': (cred, c, _, _) {
    c.card ??= {
      'number': _field(cred, 'number').replaceAll(' ', ''),
      'fullName': _field(cred, 'fullName'),
      'verificationNumber': _field(cred, 'verificationNumber'),
      'expiryDate': _field(cred, 'expiryDate'),
      'cardType': _field(cred, 'cardType'),
      'pin': _field(cred, 'pin'),
    };
  },
};

EntryRecord? _convertItem(Map<String, dynamic> item, _Report report) {
  final createdAt = _timestamp(_pick(item, 'creationAt'));
  final updatedAt = _timestamp(_pick(item, 'modifiedAt'));
  final c = _Collected();
  var unknown = 0;
  for (final cred in (_pick(item, 'credentials') as List? ?? const []).whereType<Map>()) {
    final kind = _kind(cred);
    final handler = _handlers[kind];
    if (handler == null) {
      final label = kind.isEmpty ? 'untyped' : kind;
      report.unknownTypes[label] = (report.unknownTypes[label] ?? 0) + 1;
      unknown++;
      continue;
    }
    handler(cred, c, report, createdAt);
  }

  final urls = <String>[
    for (final u in (_pick(_pick(item, 'scope') as Map? ?? const {}, 'urls') as List? ?? const []))
      if (_str(u).trim().isNotEmpty) _str(u).trim(),
  ];
  final tags = normalizeTags([
    for (final t in (_pick(item, 'tags') as List? ?? const [])) _str(t),
  ]);
  final title = _field(item, 'title');
  final notes = c.notes.join('\n\n');

  if (c.hasLogin) {
    if (c.card != null) report.cardsNextToLogin++;
    if (c.username.isEmpty &&
        c.password.isEmpty &&
        c.totpSecret.isEmpty &&
        c.passkeys.isEmpty &&
        notes.isEmpty) {
      // Nothing of the login could be read; a skipped passkey is counted.
      return null;
    }
    final url = urls.isNotEmpty ? urls.first : (c.rpId.isNotEmpty ? 'https://${c.rpId}' : '');
    return EntryRecord(
      name: _nameOf(title, c.rpId, url, c.username),
      url: url,
      type: c.passkeys.isNotEmpty ? 'password' : loginType(c.password, c.totpSecret),
      username: c.username,
      password: c.password,
      totpSecret: c.totpSecret,
      notes: notes,
      customFields: [
        ...c.customFields,
        for (final (i, u) in urls.skip(1).indexed)
          CustomFieldRecord(label: 'URL ${i + 2}', value: u, type: CustomFieldType.url),
      ],
      extraData: c.totpSecret.isEmpty ? const {} : c.totpOptions,
      passkeys: c.passkeys,
      tags: tags,
      createdAt: createdAt,
      updatedAt: updatedAt,
    );
  }

  final card = c.card;
  if (card != null) {
    final (month, year) = _expiry(card['expiryDate'] as String);
    return EntryRecord(
      name: _nameOf(title, '', '', card['fullName'] as String),
      type: 'credit-card',
      cardNumber: card['number'] as String,
      holderName: card['fullName'] as String,
      cvv: card['verificationNumber'] as String,
      expiryMonth: month,
      expiryYear: year,
      notes: notes,
      customFields: [
        ...c.customFields,
        if ((card['cardType'] as String).isNotEmpty)
          CustomFieldRecord(label: 'Card type', value: card['cardType'] as String),
        if ((card['pin'] as String).isNotEmpty)
          CustomFieldRecord(label: 'PIN', value: card['pin'] as String, type: CustomFieldType.password),
      ],
      tags: tags,
      createdAt: createdAt,
      updatedAt: updatedAt,
    );
  }

  if (notes.isNotEmpty) {
    return EntryRecord(
      name: _nameOf(title, '', '', ''),
      type: 'note',
      notes: notes,
      customFields: c.customFields,
      tags: tags,
      createdAt: createdAt,
      updatedAt: updatedAt,
    );
  }

  // Credentials of types this import does not know are counted already.
  if (unknown == 0) report.emptyItems++;
  return null;
}

String _nameOf(String title, String rpId, String url, String fallback) {
  if (title.isNotEmpty) return title;
  if (rpId.isNotEmpty) return rpId;
  final host = Uri.tryParse(url)?.host ?? '';
  if (host.isNotEmpty) return host;
  return fallback.isNotEmpty ? fallback : 'Imported item';
}

/// Month and year of a card's expiry: `YYYY-MM` (CXF), `YYYY-MM-DD`, `MM/YYYY`
/// or `MM/YY`. Unknown forms give ('', '').
(String, String) _expiry(String v) {
  final iso = RegExp(r'^(\d{4})-(\d{1,2})').firstMatch(v);
  if (iso != null) return (iso[2]!.padLeft(2, '0'), iso[1]!);
  final slash = RegExp(r'^(\d{1,2})\s*/\s*(\d{2}|\d{4})$').firstMatch(v);
  if (slash != null) {
    final y = slash[2]!;
    return (slash[1]!.padLeft(2, '0'), y.length == 2 ? '20$y' : y);
  }
  return ('', '');
}

/// A CXF passkey credential as a passkey record, or null (counted in [report])
/// when it cannot be used.
Map<String, dynamic>? _passkeyOf(Map cred, _Report report, String createdAt) {
  final rpId = _field(cred, 'rpId').toLowerCase();
  String? credentialId;
  String? userHandle;
  try {
    final id = _field(cred, 'credentialId');
    credentialId = id.isEmpty ? null : normalizeBase64Url(id);
    final handle = _field(cred, 'userHandle');
    userHandle = handle.isEmpty ? null : normalizeBase64Url(handle);
  } on FormatException {
    credentialId = null;
  }
  if (rpId.isEmpty || credentialId == null || userHandle == null) {
    report.passkeysUnreadable++;
    return null;
  }
  final String privateKey;
  try {
    privateKey = normalizePrivateKey(_field(cred, 'key'));
  } on PasskeyKeyException catch (e) {
    if (e.message.contains('Passkeys need a P-256 key')) {
      report.passkeysUnsupported++;
    } else {
      report.passkeysUnreadable++;
    }
    return null;
  }
  return {
    'credential_id': credentialId,
    'rp_id': rpId,
    'user_handle': userHandle,
    'user_name': _field(cred, 'username'),
    if (_field(cred, 'userDisplayName').isNotEmpty)
      'user_display_name': _field(cred, 'userDisplayName'),
    'key_algorithm': es256,
    'private_key': privateKey,
    'sign_count': 0,
    'discoverable': true,
    'created_at': createdAt.isNotEmpty ? createdAt : rfc3339Utc(DateTime.now()),
  };
}

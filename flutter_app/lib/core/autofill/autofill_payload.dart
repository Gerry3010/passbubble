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

// What the system autofill caches hold for each entry. The app decrypts the
// vault; the Android autofill service and the iOS and macOS AutoFill
// extensions only read these records (see docs/passkeys.md, "iOS and macOS
// AutoFill extensions").

import '../api/models.dart';
import '../importexport/entry_record.dart';
import '../util/entry_totp.dart';

/// The record every autofill consumer gets for [entry], or null when the entry
/// has neither a password nor a TOTP secret: a password alone is enough (some
/// logins have no username), and an entry holding only a passkey is not
/// offered as a password. `totp` is the bare base32 key, also when the entry
/// stores an otpauth:// URL.
Map<String, Object>? autofillCredential(
  EntryResponse entry,
  Map<String, dynamic> data,
) {
  final username = (data['username'] ?? data['account'] ?? '').toString();
  final password = (data['password'] ?? '').toString();
  final totp = _totpOf(data)?.key ?? '';
  if (password.isEmpty && totp.isEmpty) return null;
  return {
    'id': entry.id,
    'url': entry.url,
    'name': entry.name,
    // Entry type so the native side can rank/filter (e.g. prefer logins over
    // api-keys for browser fields).
    'type': entry.type,
    'username': username,
    'password': password,
    'totp': totp,
  };
}

/// What the iOS and macOS AutoFill extensions show in an entry's detail view on
/// top of [autofillCredential]: the notes, the text custom fields, the options
/// of the TOTP secret and the favorite flag. Android does not get these.
///
/// `fields` leaves out file fields and fields without a label. A `totp` field
/// carries its bare key and its own `totp_*` options; one whose value holds no
/// key goes out as a `password` field, so it stays masked.
Map<String, Object> appleCredentialDetails(
  EntryResponse entry,
  Map<String, dynamic> data,
) {
  final details = <String, Object>{'favorite': entry.favorite};
  final notes = data['notes'];
  if (notes is String && notes.trim().isNotEmpty) details['notes'] = notes;

  final totp = _totpOf(data);
  if (totp != null) details.addAll(_optionsJson(totp.options));

  final fields = <Map<String, Object>>[];
  final raw = data['custom_fields'];
  if (raw is List) {
    final stored = TotpOptions.stored(data);
    for (final cf in raw) {
      if (cf is! Map) continue;
      final label = cf['label'];
      final value = cf['value'];
      if (label is! String || label.isEmpty) continue;
      if (value is! String && value is! num) continue;
      final type = CustomFieldTypeX.fromApi(cf['type'] as String?);
      if (type == CustomFieldType.file) continue;
      if (type == CustomFieldType.totp) {
        final resolved = resolveTotp('$value', stored);
        fields.add(resolved == null
            ? {'label': label, 'type': 'password', 'value': '$value'}
            : {
                'label': label,
                'type': 'totp',
                'value': resolved.key,
                ..._optionsJson(resolved.options),
              });
        continue;
      }
      fields.add({'label': label, 'type': type.apiValue, 'value': '$value'});
    }
  }
  if (fields.isNotEmpty) details['fields'] = fields;
  return details;
}

({String key, TotpOptions options})? _totpOf(Map<String, dynamic> data) {
  final secret = totpSecretOf(data);
  return secret == null ? null : resolveTotp(secret, TotpOptions.stored(data));
}

Map<String, Object> _optionsJson(TotpOptions o) => {
      'totp_algorithm': o.algorithm.name, // SHA1, SHA256 or SHA512
      'totp_digits': o.digits,
      'totp_period': o.period,
    };

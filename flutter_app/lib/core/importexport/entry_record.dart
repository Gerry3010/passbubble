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

import '../util/entry_tags.dart' show tagsKey;
import '../util/entry_totp.dart';

/// Normalises a source timestamp into an RFC3339 UTC string for import.
/// Returns '' when empty or unparseable (server falls back to NOW()).
String normalizeImportDate(String s) {
  if (s.isEmpty) return '';
  try {
    return DateTime.parse(s).toUtc().toIso8601String();
  } catch (_) {
    return '';
  }
}

/// Field types for custom fields, mirroring the backend CustomField.Type values.
enum CustomFieldType {
  text,
  password,
  totp,
  url,
  email,
  phone,
  note,
  ssh,
  file,
}

extension CustomFieldTypeX on CustomFieldType {
  String get apiValue => name; // "text", "password", ...

  static CustomFieldType fromApi(String? v) => switch (v) {
        'password' => CustomFieldType.password,
        'totp' => CustomFieldType.totp,
        'url' => CustomFieldType.url,
        'email' => CustomFieldType.email,
        'phone' => CustomFieldType.phone,
        'note' => CustomFieldType.note,
        'ssh' => CustomFieldType.ssh,
        'file' => CustomFieldType.file,
        _ => CustomFieldType.text,
      };
}

/// A typed custom field. [value] holds Base64 content for [CustomFieldType.file].
class CustomFieldRecord {
  final String label;
  final String value;
  final CustomFieldType type;
  final String? filename; // only for file type
  final String? mimeType; // only for file type

  const CustomFieldRecord({
    required this.label,
    required this.value,
    this.type = CustomFieldType.text,
    this.filename,
    this.mimeType,
  });

  /// Build from the encrypted-data JSON map stored in the vault. A kind from a
  /// newer client counts as secret, so an export masks it.
  factory CustomFieldRecord.fromJson(Map<String, dynamic> m) {
    final kind = m['type'] as String?;
    final known = kind == null ||
        kind.isEmpty ||
        CustomFieldType.values.any((t) => t.apiValue == kind);
    return CustomFieldRecord(
      label: m['label'] as String? ?? '',
      value: m['value'] as String? ?? '',
      type: known ? CustomFieldTypeX.fromApi(kind) : CustomFieldType.password,
      filename: m['filename'] as String?,
      mimeType: m['mime_type'] as String?,
    );
  }

  Map<String, dynamic> toJson() => {
        'label': label,
        'value': value,
        'type': type.apiValue,
        if (filename != null) 'filename': filename,
        if (mimeType != null) 'mime_type': mimeType,
      };
}

/// Plaintext intermediate format for import/export operations.
class EntryRecord {
  final String name;
  final String url;
  final String type;
  final String username;
  final String password;
  final String totpSecret;
  final String notes;
  final String cardNumber;
  final String holderName;
  final String expiryMonth;
  final String expiryYear;
  final String cvv;
  final String title;
  final String firstName;
  final String lastName;
  final String company;
  final String email;
  final String phone;
  final String street;
  final String city;
  final String state;
  final String postalCode;
  final String country;
  final String licenseKey;
  final String productName;
  final List<CustomFieldRecord> customFields;

  /// What the entry's decrypted data holds beyond the fields of this class, by
  /// JSON key: bank-account fields, other clients' keys. Only an export fills
  /// this (see [extraEntryData]), for the formats that have room for them.
  final Map<String, dynamic> extraData;
  final List<String> tags; // normalised, see core/util/entry_tags.dart; empty = none
  final List<String> folderPath; // root→leaf, e.g. ['Work', 'Dev']; empty = root
  final String createdAt; // RFC3339 UTC, '' = unknown (server uses NOW())
  final String updatedAt;

  const EntryRecord({
    required this.name,
    this.url = '',
    this.type = 'password',
    this.username = '',
    this.password = '',
    this.totpSecret = '',
    this.notes = '',
    this.cardNumber = '',
    this.holderName = '',
    this.expiryMonth = '',
    this.expiryYear = '',
    this.cvv = '',
    this.title = '',
    this.firstName = '',
    this.lastName = '',
    this.company = '',
    this.email = '',
    this.phone = '',
    this.street = '',
    this.city = '',
    this.state = '',
    this.postalCode = '',
    this.country = '',
    this.licenseKey = '',
    this.productName = '',
    this.customFields = const [],
    this.extraData = const {},
    this.tags = const [],
    this.folderPath = const [],
    this.createdAt = '',
    this.updatedAt = '',
  });

  /// The values this record holds for the decrypted entry data, by JSON key,
  /// empty ones left out. The URL is not among them: it lives in the metadata.
  Map<String, String> get dataValues => {
        'username': username,
        'password': password,
        totpSecretKey: totpSecret,
        'notes': notes,
        'card_number': cardNumber,
        'holder_name': holderName,
        'expiry_month': expiryMonth,
        'expiry_year': expiryYear,
        'cvv': cvv,
        'title': title,
        'first_name': firstName,
        'last_name': lastName,
        'company': company,
        'email': email,
        'phone': phone,
        'street': street,
        'city': city,
        'state': state,
        'postal_code': postalCode,
        'country': country,
        'license_key': licenseKey,
        'product_name': productName,
      }..removeWhere((_, value) => value.isEmpty);

  EntryRecord copyWith({String? type, String? totpSecret}) => EntryRecord(
        name: name,
        url: url,
        type: type ?? this.type,
        username: username,
        password: password,
        totpSecret: totpSecret ?? this.totpSecret,
        notes: notes,
        cardNumber: cardNumber,
        holderName: holderName,
        expiryMonth: expiryMonth,
        expiryYear: expiryYear,
        cvv: cvv,
        title: title,
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
        extraData: extraData,
        tags: tags,
        folderPath: folderPath,
        createdAt: createdAt,
        updatedAt: updatedAt,
      );

  /// Duplicate detection: same name AND username (case-insensitive, trimmed).
  bool isDuplicateOf(String otherName, String otherUsername) {
    String norm(String s) => s.trim().toLowerCase();
    return norm(name) == norm(otherName) && norm(username) == norm(otherUsername);
  }
}

/// The type of a login holding [password] and [totpSecret], once all its fields
/// are read: with a password it stays a password entry with a secret next to it,
/// only a secret without a password makes a TOTP entry.
String loginType(String password, String totpSecret) =>
    totpSecret.isNotEmpty && password.isEmpty ? 'totp' : 'password';

/// [records] with each TOTP secret that is just the password (see
/// [isStrayTotpSecret]) treated as absent, as `dropStrayTOTPSecrets` does in the
/// CLI: it would overwrite a real secret in a merge, and turn a new entry into
/// type `totp`.
List<EntryRecord> dropStrayTotpSecrets(List<EntryRecord> records) => [
      for (final r in records)
        isStrayTotpSecret({totpSecretKey: r.totpSecret, 'password': r.password})
            ? r.copyWith(totpSecret: '', type: r.type == 'totp' ? 'password' : r.type)
            : r,
    ];

/// Keys of the decrypted entry data that [EntryRecord] has a place for. The
/// metadata is the truth for `name` and `url`, so their copies in the data count
/// as covered.
const _recordKeys = {
  'name',
  'url',
  'username',
  'password',
  totpSecretKey,
  'notes',
  'card_number',
  'holder_name',
  'expiry_month',
  'expiry_year',
  'cvv',
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
  'license_key',
  'product_name',
  'custom_fields',
  tagsKey,
};

/// Everything in the decrypted entry [data] that [EntryRecord] has no place for,
/// empty values left out: this becomes [EntryRecord.extraData]. That includes
/// custom-field items that are not field maps.
Map<String, dynamic> extraEntryData(Map<String, dynamic> data) {
  final extra = <String, dynamic>{
    for (final e in data.entries)
      if (!_recordKeys.contains(e.key) && e.value != null && e.value != '') e.key: e.value,
  };
  final fields = data['custom_fields'];
  if (fields is List) {
    final odd = fields.where((f) => f is! Map<String, dynamic>).toList();
    if (odd.isNotEmpty) extra['custom_fields'] = odd;
  } else if (fields != null && fields != '') {
    extra['custom_fields'] = fields;
  }
  return extra;
}

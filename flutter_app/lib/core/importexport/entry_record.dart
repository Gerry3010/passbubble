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

  /// Build from the encrypted-data JSON map stored in the vault.
  factory CustomFieldRecord.fromJson(Map<String, dynamic> m) {
    return CustomFieldRecord(
      label: m['label'] as String? ?? '',
      value: m['value'] as String? ?? '',
      type: CustomFieldTypeX.fromApi(m['type'] as String?),
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
  });

  /// Duplicate detection: same name AND username (case-insensitive, trimmed).
  bool isDuplicateOf(String otherName, String otherUsername) {
    String norm(String s) => s.trim().toLowerCase();
    return norm(name) == norm(otherName) && norm(username) == norm(otherUsername);
  }
}

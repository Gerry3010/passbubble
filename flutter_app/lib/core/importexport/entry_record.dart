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
  final List<({String label, String value})> customFields;

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
    final norm = (String s) => s.trim().toLowerCase();
    return norm(name) == norm(otherName) && norm(username) == norm(otherUsername);
  }
}

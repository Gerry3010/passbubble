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

import 'package:cryptography/cryptography.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/crypto/vault_crypto.dart';

void main() {
  // Mirrors the share-link contract: the entry-detail screen encrypts the entry
  // payload with a random key (returned as the URL-fragment secret), and the
  // public viewer decrypts it with that fragment secret.
  test('share-link payload survives create → fragment-key → viewer decrypt', () async {
    final payload = {
      'name': 'GitHub',
      'type': 'password',
      'url': 'https://github.com',
      'data': {'username': 'octocat', 'password': 's3cr3t!'},
    };

    // Create side (entry_detail_screen._createShareLink):
    final enc = await VaultCrypto.encryptEntryData(payload);
    final fragmentSecret = base64Url.encode(enc.dataKey); // goes after '#'

    // Viewer side (share_viewer_screen._load):
    final key = SecretKey(base64Url.decode(fragmentSecret));
    final plain = await VaultCrypto.decrypt(key, base64.decode(enc.encryptedData));
    final decoded = jsonDecode(utf8.decode(plain)) as Map<String, dynamic>;

    expect(decoded['name'], 'GitHub');
    expect((decoded['data'] as Map)['password'], 's3cr3t!');
  });

  test('a wrong fragment key fails to decrypt', () async {
    final enc = await VaultCrypto.encryptEntryData({'name': 'x', 'data': {}});
    final wrong = SecretKey(VaultCrypto.randomKey());
    expect(
      () => VaultCrypto.decrypt(wrong, base64.decode(enc.encryptedData)),
      throwsA(anything),
    );
  });
}

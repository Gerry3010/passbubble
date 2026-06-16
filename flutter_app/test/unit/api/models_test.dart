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

import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/api/models.dart';

void main() {
  group('LoginResponse', () {
    test('fromJson parses all fields', () {
      final json = {
        'access_token': 'acc_tok',
        'refresh_token': 'ref_tok',
        'user_id': 'uid-1',
        'email': 'alice@example.com',
        'name': 'Alice',
        'role': 'admin',
        'kdf_salt': 'c2FsdA==',
        'kdf_time': 3,
        'kdf_memory': 65536,
        'enc_priv_x25519': 'encX',
        'enc_priv_mlkem768': 'encM',
        'pub_x25519': 'pubX',
        'pub_mlkem768': 'pubM',
      };
      final resp = LoginResponse.fromJson(json);
      expect(resp.accessToken, 'acc_tok');
      expect(resp.refreshToken, 'ref_tok');
      expect(resp.userId, 'uid-1');
      expect(resp.email, 'alice@example.com');
      expect(resp.name, 'Alice');
      expect(resp.role, 'admin');
      expect(resp.kdfSalt, 'c2FsdA==');
      expect(resp.kdfTime, 3);
      expect(resp.kdfMemory, 65536);
      expect(resp.encPrivX25519, 'encX');
      expect(resp.pubX25519, 'pubX');
    });
  });

  group('RegisterRequest', () {
    test('toJson produces correct keys', () {
      final req = RegisterRequest(
        email: 'bob@example.com',
        name: 'Bob',
        password: 'secret',
        invitationToken: 'tok',
        pubX25519: 'pubX',
        pubMlkem768: 'pubM',
        encPrivX25519: 'encX',
        encPrivMlkem768: 'encM',
        kdfSalt: 'salt',
      );
      final json = req.toJson();
      expect(json['email'], 'bob@example.com');
      expect(json['name'], 'Bob');
      expect(json['password'], 'secret');
      expect(json['invitation_token'], 'tok');
      expect(json['pub_x25519'], 'pubX');
      expect(json['pub_mlkem768'], 'pubM');
      expect(json['enc_priv_x25519'], 'encX');
      expect(json['enc_priv_mlkem768'], 'encM');
      expect(json['kdf_salt'], 'salt');
    });
  });

  group('EntryResponse', () {
    test('fromJson handles null optional fields', () {
      final json = {
        'id': 'entry-1',
        'name': 'GitHub',
        'url': 'https://github.com',
        'type': 'password',
        'encrypted_data': 'YWJj',
        'data_nonce': 'bm9uY2U=',
        'permission': 'owner',
        'created_at': '2025-01-01T00:00:00Z',
        'updated_at': '2025-01-01T00:00:00Z',
        'folder_id': null,
        'entry_key': null,
      };
      final entry = EntryResponse.fromJson(json);
      expect(entry.id, 'entry-1');
      expect(entry.name, 'GitHub');
      expect(entry.folderId, isNull);
      expect(entry.entryKey, isNull);
    });

    test('fromJson parses entry_key when present', () {
      final json = {
        'id': 'entry-2',
        'name': 'Gmail',
        'url': 'https://gmail.com',
        'type': 'password',
        'encrypted_data': 'Y2lwaGVy',
        'data_nonce': 'bm9uY2U=',
        'permission': 'owner',
        'created_at': '2025-01-01T00:00:00Z',
        'updated_at': '2025-01-01T00:00:00Z',
        'folder_id': null,
        'entry_key': {
          'user_id': 'user-1',
          'encrypted_key': 'ZW5jS2V5',
        },
      };
      final entry = EntryResponse.fromJson(json);
      expect(entry.entryKey, isNotNull);
      expect(entry.entryKey!.userId, 'user-1');
      expect(entry.entryKey!.encryptedKey, 'ZW5jS2V5');
    });
  });

  group('CreateEntryRequest', () {
    test('toJson produces correct structure with entry keys', () {
      final req = CreateEntryRequest(
        type: 'password',
        name: 'Test Entry',
        url: 'https://example.com',
        encryptedData: 'enc',
        dataNonce: 'nonce',
        entryKeys: [
          EntryKey(userId: 'user-1', encryptedKey: 'encKey'),
        ],
      );
      final json = req.toJson();
      expect(json['type'], 'password');
      expect(json['name'], 'Test Entry');
      expect(json['url'], 'https://example.com');
      expect(json['encrypted_data'], 'enc');
      expect(json['data_nonce'], 'nonce');
      final keys = json['entry_keys'] as List;
      expect(keys.length, 1);
      expect(keys[0]['user_id'], 'user-1');
      expect(keys[0]['encrypted_key'], 'encKey');
    });
  });

  group('GenerateResponse', () {
    test('fromJson parses passwords list', () {
      final json = {
        'passwords': [
          {'password': 'Abc123!', 'strength': 72},
          {'password': 'Xyz789@', 'strength': 85},
        ],
      };
      final resp = GenerateResponse.fromJson(json);
      expect(resp.passwords.length, 2);
      expect(resp.passwords[0].password, 'Abc123!');
      expect(resp.passwords[0].strength, 72);
      expect(resp.passwords[1].strength, 85);
    });
  });

  group('FolderResponse', () {
    test('fromJson parses nested children', () {
      final json = {
        'id': 'folder-1',
        'name': 'Work',
        'parent_id': null,
        'created_at': '2025-01-01T00:00:00Z',
        'updated_at': '2025-01-01T00:00:00Z',
        'children': [
          {
            'id': 'folder-2',
            'name': 'Dev',
            'parent_id': 'folder-1',
            'created_at': '2025-01-01T00:00:00Z',
            'updated_at': '2025-01-01T00:00:00Z',
            'children': [],
          }
        ],
      };
      final folder = FolderResponse.fromJson(json);
      expect(folder.name, 'Work');
      expect(folder.children.length, 1);
      expect(folder.children[0].name, 'Dev');
      expect(folder.children[0].parentId, 'folder-1');
    });
  });
}

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
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/importexport/csv_format.dart';
import 'package:passbubble/core/importexport/entry_record.dart';
import 'package:passbubble/core/importexport/onepassword_format.dart';
import 'package:passbubble/core/importexport/psono_format.dart';

const _secret = 'JBSWY3DPEHPK3PXP';

/// A 1PUX archive holding one item of [category] with a TOTP section.
Uint8List _onePux({required String category, String password = ''}) {
  final json = utf8.encode(jsonEncode({
    'accounts': [
      {
        'vaults': [
          {
            'items': [
              {
                'categoryUuid': category,
                'overview': {'title': 'Item'},
                'details': {
                  'loginFields': [
                    {'designation': 'password', 'value': password},
                  ],
                  'sections': [
                    {
                      'fields': [
                        {
                          'id': 'totp',
                          'title': 'One-Time Password',
                          'value': {'totp': _secret},
                        },
                      ],
                    },
                  ],
                },
              },
            ],
          },
        ],
      },
    ],
  }));
  final archive = Archive()..addFile(ArchiveFile('export.data', json.length, json));
  return Uint8List.fromList(ZipEncoder().encode(archive)!);
}

void main() {
  group('login types in every format', () {
    String lastPass(String password) => parseCsv(
          'url,username,password,totp,extra,name,grouping,fav\n,alice,$password,$_secret,,Mail,,\n',
          CsvImportFormat.lastpass,
        ).single.type;
    String psono(String password) => parsePsono(jsonEncode({
          'items': [
            {
              'type': 'website_password',
              'website_password_title': 'Mail',
              'website_password_password': password,
              'website_password_totp_code': _secret,
            },
          ],
        })).records.single.type;
    String onePux(String password) =>
        parseOnePux(_onePux(category: '001', password: password)).records.single.type;

    for (final (name, type) in [
      ('LastPass', lastPass),
      ('Psono', psono),
      ('1PUX', onePux),
    ]) {
      test('$name: a password and a secret stay a login, a bare secret is a TOTP entry', () {
        expect(type('pw'), 'password');
        expect(type(''), 'totp');
      });
    }

    test('1PUX: a secret on another category leaves its type alone', () {
      final r = parseOnePux(_onePux(category: '002')).records.single;
      expect(r.type, 'credit-card');
      expect(r.totpSecret, _secret);
    });
  });

  group('CustomFieldRecord.fromJson', () {
    test('a kind from a newer client counts as secret', () {
      expect(
          CustomFieldRecord.fromJson(const {'label': 'L', 'value': 'v', 'type': 'passkey'}).type,
          CustomFieldType.password);
    });

    test('a missing kind is text, a known one stays', () {
      expect(CustomFieldRecord.fromJson(const {'label': 'L', 'value': 'v'}).type,
          CustomFieldType.text);
      expect(CustomFieldRecord.fromJson(const {'label': 'L', 'value': 'v', 'type': ''}).type,
          CustomFieldType.text);
      for (final kind in CustomFieldType.values) {
        expect(
            CustomFieldRecord.fromJson({'label': 'L', 'value': 'v', 'type': kind.apiValue}).type,
            kind);
      }
    });
  });

  group('dropStrayTotpSecrets', () {
    test('a secret that is the password goes, and a totp type becomes password', () {
      final r = dropStrayTotpSecrets(const [
        EntryRecord(
          name: 'Mail',
          type: 'totp',
          username: 'alice',
          password: 'same',
          totpSecret: 'same',
          notes: 'n',
          tags: ['t'],
          folderPath: ['Work'],
          customFields: [CustomFieldRecord(label: 'L', value: 'v')],
        ),
      ]).single;
      expect(r.totpSecret, isEmpty);
      expect(r.type, 'password');
      expect(r.password, 'same');
      expect(r.username, 'alice');
      expect(r.notes, 'n');
      expect(r.tags, ['t']);
      expect(r.folderPath, ['Work']);
      expect(r.customFields.single.label, 'L');
    });

    test('another type stays', () {
      final r = dropStrayTotpSecrets(
          const [EntryRecord(name: 'K', type: 'api-key', password: 's', totpSecret: 's')]).single;
      expect(r.totpSecret, isEmpty);
      expect(r.type, 'api-key');
    });

    test('a real secret and a bare secret stay', () {
      final out = dropStrayTotpSecrets(const [
        EntryRecord(name: 'A', type: 'totp', password: 'pw', totpSecret: 'JBSWY3DPEHPK3PXP'),
        EntryRecord(name: 'B', type: 'totp', totpSecret: 'JBSWY3DPEHPK3PXP'),
        EntryRecord(name: 'C', password: 'pw'),
      ]);
      expect(out.map((r) => r.totpSecret),
          ['JBSWY3DPEHPK3PXP', 'JBSWY3DPEHPK3PXP', '']);
      expect(out.map((r) => r.type), ['totp', 'totp', 'password']);
    });
  });

  group('extraEntryData', () {
    test('holds what the record has no place for, and not the empty', () {
      expect(
        extraEntryData({
          'name': 'copy of the metadata',
          'url': 'copy of the metadata',
          'username': 'alice',
          'password': 'pw',
          'totp_secret': 'x',
          'title': 'Dr',
          'tags': ['t'],
          'custom_fields': [
            {'label': 'L', 'value': 'v', 'type': 'text'},
          ],
          'iban': 'AT00',
          'period': 60,
          'other_client': {'nested': true},
          'blank': '',
          'nothing': null,
        }),
        {
          'iban': 'AT00',
          'period': 60,
          'other_client': {'nested': true},
        },
      );
    });

    test('custom-field items of unknown shape are in it too', () {
      expect(
        extraEntryData({
          'custom_fields': [
            'odd',
            {'label': 'L', 'value': 'v', 'type': 'text'},
            7,
          ],
        }),
        {
          'custom_fields': ['odd', 7],
        },
      );
      expect(extraEntryData({'custom_fields': 'odd'}), {'custom_fields': 'odd'});
      expect(extraEntryData({'custom_fields': <dynamic>[]}), isEmpty);
    });
  });

  group('csvLeftOutFields', () {
    test('a record that fits the columns leaves nothing out', () {
      expect(
        csvLeftOutFields(const EntryRecord(
          name: 'Mail',
          url: 'https://x.example',
          username: 'alice',
          password: 'pw',
          notes: 'n',
          tags: ['t'],
        )),
        0,
      );
    });

    test('counts every value without a column', () {
      expect(
        csvLeftOutFields(const EntryRecord(
          name: 'Mail',
          username: 'alice',
          password: 'pw',
          totpSecret: 'JBSWY3DPEHPK3PXP',
          cardNumber: '4111',
          customFields: [
            CustomFieldRecord(label: 'A', value: 'a'),
            CustomFieldRecord(label: 'B', value: 'QUJD', type: CustomFieldType.file),
          ],
          extraData: {'iban': 'AT00'},
        )),
        5,
      );
    });
  });
}

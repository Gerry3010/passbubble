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

import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/importexport/bitwarden_format.dart';
import 'package:passbubble/core/importexport/entry_record.dart';
import 'package:passbubble/core/jobs/job_runner.dart';

const _secret = 'JBSWY3DPEHPK3PXP';

/// The custom field a Bitwarden file with this one field parses to.
CustomFieldRecord _parsedField(Map<String, dynamic> field) => parseBitwarden(jsonEncode({
      'items': [
        {
          'type': 2,
          'name': 'Note',
          'fields': [field],
        },
      ],
    })).single.customFields.single;

Map<String, dynamic> _item(EntryRecord r) =>
    ((jsonDecode(exportBitwarden([r])) as Map)['items'] as List).single as Map<String, dynamic>;

/// The item's custom fields as name → (value, type).
Map<String, (String, int)> _fields(Map<String, dynamic> item) => {
      for (final f in (item['fields'] as List? ?? const []))
        f['name'] as String: (f['value'] as String, f['type'] as int),
    };

void main() {
  group('import: file fields', () {
    test('a hidden field with a base64 data: URI is a file', () {
      final f = _parsedField(
          {'name': 'a.pdf', 'value': 'data:application/pdf;base64,QUJD', 'type': 1});
      expect(f.type, CustomFieldType.file);
      expect(f.label, 'a.pdf');
      expect(f.value, 'QUJD');
      expect(f.filename, 'a.pdf');
      expect(f.mimeType, 'application/pdf');
    });

    test('a data: URI without a mime type has none, and an empty one is still a file', () {
      final f = _parsedField({'name': 'x', 'value': 'data:;base64,QUJD', 'type': 1});
      expect(f.type, CustomFieldType.file);
      expect(f.mimeType, isNull);
      expect(f.toJson().containsKey('mime_type'), isFalse);

      expect(_parsedField({'name': 'x', 'value': 'data:a/b;base64,', 'type': 1}).type,
          CustomFieldType.file);
    });

    test('line breaks in the content are gone, so the app can decode it', () {
      final f = _parsedField({'name': 'x', 'value': 'data:a/b;base64,QUJD\r\nQUJD\n', 'type': 1});
      expect(f.type, CustomFieldType.file);
      expect(f.value, 'QUJDQUJD');
    });

    test('anything else hidden stays a password, untouched', () {
      for (final value in [
        'hunter2',
        'data:text/plain,hello', // not base64
        'data:a/b;base64,not base64!',
        'data:a/b;base64,QUJ', // bad length
        'data:a/b;base64,-_-_', // URL-safe alphabet
        'xdata:a/b;base64,QUJD',
      ]) {
        final f = _parsedField({'name': 'x', 'value': value, 'type': 1});
        expect(f.type, CustomFieldType.password, reason: value);
        expect(f.value, value);
        expect(f.filename, isNull, reason: value);
      }
    });

    test('a visible field is text, whatever it holds', () {
      final f = _parsedField({'name': 'x', 'value': 'data:a/b;base64,QUJD', 'type': 0});
      expect(f.type, CustomFieldType.text);
      expect(f.value, 'data:a/b;base64,QUJD');
    });

    test('the exporter\'s own file fields come back as files', () {
      final json = exportBitwarden(
        const [
          EntryRecord(name: 'Mail', customFields: [
            CustomFieldRecord(
              label: 'a.pdf',
              value: 'QUJD',
              type: CustomFieldType.file,
              filename: 'a.pdf',
              mimeType: 'application/pdf',
            ),
          ]),
        ],
        const BitwardenExportOptions(includeFiles: true, filesAsBase64: true),
      );
      final f = parseBitwarden(json).single.customFields.single;
      expect(f.toJson(), {
        'label': 'a.pdf',
        'value': 'QUJD',
        'type': 'file',
        'filename': 'a.pdf',
        'mime_type': 'application/pdf',
      });
    });

    test('a merge over a file field with the same label gives a working file', () {
      final parsed = _parsedField(
          {'name': 'Doc', 'value': 'data:image/png;base64,REVG', 'type': 1});
      final merged = mergeImportedData(
        {
          'custom_fields': [
            {'label': 'Doc', 'value': 'QUJD', 'type': 'file', 'filename': 'a.pdf', 'mime_type': 'application/pdf'},
          ],
        },
        {
          'custom_fields': [parsed.toJson()],
        },
      );
      expect(merged['custom_fields'], [
        {'label': 'Doc', 'value': 'REVG', 'type': 'file', 'filename': 'Doc', 'mime_type': 'image/png'},
      ]);
    });
  });

  group('import: login types', () {
    EntryRecord login(Map<String, dynamic> login) => parseBitwarden(jsonEncode({
          'items': [
            {'type': 1, 'name': 'Mail', 'login': login},
          ],
        })).single;

    test('a login with a password and a secret stays a login and keeps both', () {
      final r = login({'username': 'alice', 'password': 'pw', 'totp': _secret});
      expect(r.type, 'password');
      expect(r.password, 'pw');
      expect(r.totpSecret, _secret);
    });

    test('only a bare secret becomes a TOTP entry', () {
      expect(login({'username': 'alice', 'totp': _secret}).type, 'totp');
      expect(login({'username': 'alice', 'password': '', 'totp': _secret}).type, 'totp');
    });

    test('no secret means a login', () {
      expect(login({'username': 'alice', 'password': 'pw'}).type, 'password');
      expect(login({'username': 'alice'}).type, 'password');
    });
  });

  group('export', () {
    test('an identity is an identity item, and comes back complete', () {
      const identity = EntryRecord(
        name: 'Me',
        type: 'identity',
        notes: 'n',
        title: 'Dr',
        firstName: 'Ada',
        lastName: 'Lovelace',
        company: 'Analytical',
        email: 'ada@example.com',
        phone: '+43 1',
        street: 'Main St 1',
        city: 'Vienna',
        state: 'W',
        postalCode: '1010',
        country: 'AT',
      );
      final item = _item(identity);
      expect(item['type'], 4);
      expect(item.containsKey('login'), isFalse);
      expect(item['identity'], {
        'title': 'Dr',
        'firstName': 'Ada',
        'lastName': 'Lovelace',
        'company': 'Analytical',
        'email': 'ada@example.com',
        'phone': '+43 1',
        'address1': 'Main St 1',
        'city': 'Vienna',
        'state': 'W',
        'postalCode': '1010',
        'country': 'AT',
      });
      expect(item.containsKey('fields'), isFalse, reason: 'the item holds it all');

      final back = parseBitwarden(exportBitwarden(const [identity])).single;
      expect(back.type, 'identity');
      expect(back.dataValues, identity.dataValues);
    });

    test('every login-shaped type keeps URL and 2FA in the login', () {
      for (final type in ['password', 'totp', 'api-key', 'ssh-key', 'license', 'bank-account', 'future-type', '']) {
        final item = _item(EntryRecord(
          name: 'X',
          type: type,
          url: 'https://x.example',
          username: 'alice',
          password: 'pw',
          totpSecret: _secret,
        ));
        expect(item['type'], 1, reason: type);
        expect(item['login'], {
          'username': 'alice',
          'password': 'pw',
          'totp': _secret,
          'uris': [
            {'uri': 'https://x.example'},
          ],
        }, reason: type);
        expect(
          _fields(item),
          const {'password', 'totp', ''}.contains(type) ? isEmpty : {'type': (type, 0)},
          reason: 'the file says what the entry really was',
        );
      }
    });

    test('item types without a login keep URL and 2FA as custom fields', () {
      const record = EntryRecord(
        name: 'X',
        url: 'https://x.example',
        totpSecret: _secret,
        username: 'alice',
        password: 'pw',
      );
      EntryRecord typed(String type) => EntryRecord(
            name: record.name,
            type: type,
            url: record.url,
            totpSecret: record.totpSecret,
            username: record.username,
            password: record.password,
          );

      for (final (type, bwType) in [('note', 2), ('credit-card', 3), ('identity', 4)]) {
        final item = _item(typed(type));
        expect(item['type'], bwType, reason: type);
        expect(item.containsKey('login'), isFalse, reason: type);
        expect(_fields(item), {
          'url': ('https://x.example', 0),
          'totp_secret': (_secret, 1),
          'username': ('alice', 0),
          'password': ('pw', 1),
        }, reason: type);
      }
    });

    test('a record\'s values the item has no place for become custom fields', () {
      final item = _item(const EntryRecord(
        name: 'Key',
        type: 'license',
        productName: 'Tool',
        licenseKey: 'AAAA-BBBB',
        cardNumber: '4111',
        cvv: '123',
        holderName: 'Ada',
        title: 'Dr',
      ));
      expect(_fields(item), {
        'type': ('license', 0),
        'title': ('Dr', 0),
        'card_number': ('4111', 1),
        'holder_name': ('Ada', 0),
        'cvv': ('123', 1),
        'license_key': ('AAAA-BBBB', 1),
        'product_name': ('Tool', 0),
      });
    });

    test('extra data: known plain keys are text, secrets and unknown keys hidden', () {
      final item = _item(const EntryRecord(
        name: 'Bank',
        type: 'bank-account',
        extraData: {
          'bank_name': 'Erste',
          'bic': 'GIBAATWW',
          'account_type': 'savings',
          'iban': 'AT00 1234',
          'account_number': '1234',
          'issuer': 'ACME',
          'period': 60,
          'digits': 8,
          'algorithm': 'SHA256',
          'sign_in_with': 'google',
          'purchase_date': '2026-01-01',
          'somebody_elses': 'who knows',
          'other_client': {'nested': true},
        },
      ));
      expect(_fields(item), {
        'type': ('bank-account', 0),
        'bank_name': ('Erste', 0),
        'bic': ('GIBAATWW', 0),
        'account_type': ('savings', 0),
        'iban': ('AT00 1234', 1),
        'account_number': ('1234', 1),
        'issuer': ('ACME', 0),
        'period': ('60', 0),
        'digits': ('8', 0),
        'algorithm': ('SHA256', 0),
        'sign_in_with': ('google', 0),
        'purchase_date': ('2026-01-01', 0),
        'somebody_elses': ('who knows', 1),
        'other_client': ('{"nested":true}', 1),
      });
    });

    test('the order follows the Go exporter: known keys, the type, unknown keys', () {
      final item = _item(const EntryRecord(
        name: 'X',
        type: 'license',
        cardNumber: '4111',
        productName: 'Tool',
        extraData: {
          'somebody_elses': 'who knows',
          'expires_at': '2030-01-01',
          'issuer': 'ACME',
        },
      ));
      expect((item['fields'] as List).map((f) => f['name']),
          ['issuer', 'card_number', 'product_name', 'expires_at', 'type', 'somebody_elses']);
    });

    test('they sit after the user\'s fields and before the tags, and survive an import', () {
      final json = exportBitwarden(const [
        EntryRecord(
          name: 'Bank',
          type: 'bank-account',
          tags: ['t'],
          customFields: [CustomFieldRecord(label: 'Mine', value: 'm')],
          extraData: {'iban': 'AT00'},
        ),
      ]);
      final fields = (((jsonDecode(json) as Map)['items'] as List).single as Map)['fields'] as List;
      expect(fields.map((f) => f['name']), ['Mine', 'iban', 'type', 'passbubble_tags']);

      final back = parseBitwarden(json).single;
      expect(back.tags, ['t']);
      expect(back.customFields.map((f) => f.toJson()), [
        {'label': 'Mine', 'value': 'm', 'type': 'text'},
        {'label': 'iban', 'value': 'AT00', 'type': 'password'},
        {'label': 'type', 'value': 'bank-account', 'type': 'text'},
      ]);
    });

    test('notes stay on the item, whatever the type', () {
      for (final type in ['password', 'note', 'credit-card', 'identity']) {
        final item = _item(EntryRecord(name: 'X', type: type, notes: 'n'));
        expect(item['notes'], 'n', reason: type);
        expect(item.containsKey('fields'), isFalse, reason: type);
      }
    });
  });
}

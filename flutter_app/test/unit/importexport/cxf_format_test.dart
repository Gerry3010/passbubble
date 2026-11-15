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
import 'package:passbubble/core/importexport/cxf_format.dart';
import 'package:passbubble/core/importexport/entry_record.dart';

import '../../fixtures/passkey_fixtures.dart';

/// A CXF export with [items] in one account.
String _export(List<Map<String, dynamic>> items) => jsonEncode({
      'version': {'major': 1, 'minor': 0},
      'exporterRpId': 'apple.com',
      'exporterDisplayName': 'Apple Passwords',
      'timestamp': 1763089200,
      'accounts': [
        {'id': 'YWNj', 'username': 'me', 'email': 'me@example.com', 'items': items},
      ],
    });

Map<String, dynamic> _field(String value, [String type = 'string']) =>
    {'fieldType': type, 'value': value};

void main() {
  test('a login with password, passkey and TOTP becomes one password entry', () {
    final r = parseCxf(_export([
      {
        'id': 'aXRlbQ',
        'title': 'Example',
        'creationAt': 1763089200,
        'modifiedAt': 1763092800,
        'scope': {
          'urls': ['https://example.com/login', 'https://m.example.com'],
        },
        'tags': ['Work', 'work'],
        'credentials': [
          {
            'type': 'basic-auth',
            'username': _field('alice@example.com'),
            'password': _field(' s3cret ', 'concealed-string'),
          },
          {
            'type': 'passkey',
            'credentialId': 'q2D0bQ2u3E7XoWlS1yZbVw',
            'rpId': 'example.com',
            'username': 'alice@example.com',
            'userDisplayName': 'Alice',
            'userHandle': 'dXNlci0xMjM0',
            'key': p256Long,
          },
          {
            'type': 'totp',
            'secret': 'JBSWY3DPEHPK3PXP',
            'period': 60,
            'digits': 8,
            'algorithm': 'sha256',
            'issuer': 'Example',
          },
        ],
      },
    ]));
    expect(r.warnings, isEmpty);
    final rec = r.records.single;
    expect(rec.type, 'password');
    expect(rec.name, 'Example');
    expect(rec.url, 'https://example.com/login');
    expect(rec.username, 'alice@example.com');
    expect(rec.password, ' s3cret ');
    expect(rec.totpSecret, 'JBSWY3DPEHPK3PXP');
    expect(rec.extraData, {'period': 60, 'digits': 8, 'algorithm': 'SHA256', 'issuer': 'Example'});
    expect(rec.tags, ['Work']);
    expect(rec.createdAt, '2025-11-14T03:00:00Z');
    expect(rec.updatedAt, '2025-11-14T04:00:00Z');
    expect(rec.customFields.map((f) => f.toJson()), [
      {'label': 'URL 2', 'value': 'https://m.example.com', 'type': 'url'},
    ]);
    expect(rec.passkeys, [
      {
        'credential_id': 'q2D0bQ2u3E7XoWlS1yZbVw',
        'rp_id': 'example.com',
        'user_handle': 'dXNlci0xMjM0',
        'user_name': 'alice@example.com',
        'user_display_name': 'Alice',
        'key_algorithm': -7,
        'private_key': p256Long,
        'sign_count': 0,
        'discoverable': true,
        'created_at': '2025-11-14T03:00:00Z',
      },
    ]);
  });

  test('a passkey alone gives a login named after the site, its key normalised', () {
    final rec = parseCxf(_export([
      {
        'id': 'aQ',
        'credentials': [
          {
            'type': 'passkey',
            'credentialId': 'q2D0bQ2u3E7XoWlS1yZbVw==',
            'rpId': 'Example.com',
            'username': 'bob',
            'userHandle': 'Ym9i',
            'key': p256Short,
          },
        ],
      },
    ])).records.single;
    expect(rec.type, 'password');
    expect(rec.name, 'example.com');
    expect(rec.url, 'https://example.com');
    expect(rec.username, 'bob');
    expect(rec.password, '');
    expect(rec.passkeys.single['private_key'], p256Long);
    expect(rec.passkeys.single['credential_id'], 'q2D0bQ2u3E7XoWlS1yZbVw');
  });

  test('a note and a card, with plain strings instead of fields', () {
    final r = parseCxf(_export([
      {
        'id': 'bg',
        'title': 'Wifi at home',
        'credentials': [
          {'type': 'note', 'content': 'pw: hunter2'},
        ],
      },
      {
        'id': 'Yw',
        'title': 'Visa',
        'credentials': [
          {
            'type': 'credit-card',
            'number': _field('4111 1111 1111 1111'),
            'fullName': 'Alice Example',
            'verificationNumber': _field('123', 'concealed-string'),
            'expiryDate': _field('2029-07', 'year-month'),
            'cardType': 'Visa',
          },
          {'type': 'note', 'content': _field('billing card')},
        ],
      },
    ]));
    expect(r.warnings, isEmpty);
    final [note, card] = r.records;
    expect(note.type, 'note');
    expect(note.name, 'Wifi at home');
    expect(note.notes, 'pw: hunter2');
    expect(card.type, 'credit-card');
    expect(card.cardNumber, '4111111111111111');
    expect(card.holderName, 'Alice Example');
    expect(card.cvv, '123');
    expect(card.expiryMonth, '07');
    expect(card.expiryYear, '2029');
    expect(card.notes, 'billing card');
    expect(card.customFields.map((f) => f.toJson()), [
      {'label': 'Card type', 'value': 'Visa', 'type': 'text'},
    ]);
  });

  test('unknown credential types are counted, never fatal', () {
    final r = parseCxf(_export([
      {
        'id': 'MQ',
        'title': 'Router',
        'credentials': [
          {'type': 'wifi', 'ssid': 'home'},
          {'type': 'ssh-key', 'privateKey': 'x'},
        ],
      },
      {
        'id': 'Mg',
        'title': 'Mixed',
        'credentials': [
          {'type': 'basicAuth', 'username': 'u', 'password': 'p'},
          {'type': 'ssh-key', 'privateKey': 'y'},
        ],
      },
      {'id': 'Mw', 'title': 'Empty', 'credentials': []},
    ]));
    expect(r.records.map((e) => (e.name, e.username, e.password)), [('Mixed', 'u', 'p')]);
    expect(r.warnings, [
      'Not imported (no matching entry type yet): 1× wifi, 2× ssh-key.',
      '1 item had nothing to import.',
    ]);
  });

  test('passkeys that are not P-256 or cannot be read are counted', () {
    final r = parseCxf(_export([
      {
        'id': 'NA',
        'title': 'Keys',
        'credentials': [
          {'type': 'basic-auth', 'username': 'u', 'password': 'p'},
          {'type': 'passkey', 'credentialId': 'AQ', 'rpId': 'a.com', 'userHandle': 'AQ', 'key': p384Pem},
          {'type': 'passkey', 'credentialId': 'AQ', 'rpId': 'a.com', 'userHandle': 'AQ', 'key': 'xx'},
          {'type': 'passkey', 'rpId': 'a.com', 'userHandle': 'AQ', 'key': p256Long},
        ],
      },
    ]));
    expect(r.records.single.passkeys, isEmpty);
    expect(r.records.single.type, 'password');
    expect(r.warnings, [
      '1 passkey was skipped: only ES256 (P-256) passkeys are supported.',
      '2 passkeys were skipped: its data could not be read.',
    ]);
  });

  test('a bare account, a list of items and RFC 3339 dates are read too', () {
    final item = {
      'id': 'NQ',
      'title': 'Mail',
      'creationAt': '2026-01-02T03:04:05Z',
      'credentials': [
        {'type': 'basic-auth', 'username': 'u', 'password': 'p'},
      ],
    };
    for (final json in [
      jsonEncode({'items': [item]}),
      jsonEncode([item]),
      jsonEncode([
        {'items': [item]},
      ]),
    ]) {
      final rec = parseCxf(json).records.single;
      expect(rec.name, 'Mail', reason: json);
      expect(rec.createdAt, '2026-01-02T03:04:05Z', reason: json);
    }
    expect(() => parseCxf('{"hello": 1}'), throwsFormatException);
  });

  test('a TOTP alone is a totp entry; describeCxf counts items and names the exporter', () {
    final json = _export([
      {
        'id': 'Ng',
        'title': 'GitHub',
        'credentials': [
          {'type': 'totp', 'secret': 'JBSW Y3DP EHPK 3PXP', 'username': 'octo'},
        ],
      },
    ]);
    final rec = parseCxf(json).records.single;
    expect(rec.type, 'totp');
    expect(rec.totpSecret, 'JBSWY3DPEHPK3PXP');
    expect(rec.username, 'octo');
    expect(rec.extraData, isEmpty);
    expect(describeCxf(json), (items: 1, exporter: 'Apple Passwords'));
    expect(rec, isA<EntryRecord>());
  });
}

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
import 'package:passbubble/core/passkeys/passkey.dart';
import 'package:passbubble/core/passkeys/passkey_vault.dart';

import '../../fixtures/passkey_fixtures.dart';

Matcher _refused(String part) => throwsA(isA<PasskeyKeyException>()
    .having((e) => e.message, 'message', contains(part)));

void main() {
  group('normalizePrivateKey', () {
    test('the long PKCS#8 form stays the same, and is 138 bytes', () {
      expect(normalizePrivateKey(p256Long), p256Long);
      expect(decodeBase64Flexible(p256Long), hasLength(138));
    });

    test('every form of the same key gives the long PKCS#8 form', () {
      final forms = {
        'PKCS#8 PEM': p256Pkcs8Pem,
        'SEC1 PEM': p256Sec1Pem,
        'short PKCS#8, standard base64': p256Short,
        'PKCS#8 with the curve but no public key': p256WithParams,
        'long form in standard base64 with padding':
            base64.encode(decodeBase64Flexible(p256Long)),
        'base64 wrapped over lines': base64
            .encode(decodeBase64Flexible(p256Long))
            .replaceAllMapped(RegExp('.{40}'), (m) => '${m[0]}\n'),
        'PEM with surrounding blanks': '\n  $p256Pkcs8Pem  \n',
      };
      for (final MapEntry(key: label, value: input) in forms.entries) {
        expect(normalizePrivateKey(input), p256Long, reason: label);
      }
    });

    test('an EC PARAMETERS block before the key is skipped', () {
      final out = decodeBase64Flexible(normalizePrivateKey(p256OtherWithParamsPem));
      expect(out, hasLength(138));
      expect(normalizePrivateKey(p256OtherWithParamsPem), isNot(p256Long));
    });

    test('other curves and algorithms are refused by name', () {
      expect(() => normalizePrivateKey(p384Pem), _refused('P-384'));
      expect(() => normalizePrivateKey(secp256k1Pem), _refused('secp256k1'));
      expect(() => normalizePrivateKey(ed25519Pem), _refused('Ed25519'));
      for (final key in [p384Pem, secp256k1Pem, ed25519Pem]) {
        expect(() => normalizePrivateKey(key), _refused('Passkeys need a P-256 key'));
      }
    });

    test('garbage gets a message, not a crash', () {
      expect(() => normalizePrivateKey(''), _refused('Enter the private key'));
      expect(() => normalizePrivateKey('not a key!'), _refused('Not a key'));
      expect(() => normalizePrivateKey(base64.encode(utf8.encode('hello world'))),
          _refused('damaged'));
      // Cut short.
      final der = decodeBase64Flexible(p256Long);
      expect(() => normalizePrivateKey(base64.encode(der.sublist(0, 60))), _refused('damaged'));
      expect(
          () => normalizePrivateKey('-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----'),
          _refused('CERTIFICATE'));
      expect(
          () => normalizePrivateKey(
              '-----BEGIN ENCRYPTED PRIVATE KEY-----\nAAAA\n-----END ENCRYPTED PRIVATE KEY-----'),
          _refused('encrypted'));
    });

    test('a public key that does not belong to the scalar is refused', () {
      final der = decodeBase64Flexible(p256Long);
      der[der.length - 1] ^= 0x01;
      expect(() => normalizePrivateKey(base64.encode(der)), _refused('does not belong'));
    });
  });

  group('base64url', () {
    test('decoding takes both alphabets, padding and whitespace', () {
      final bytes = [0xfb, 0xff, 0xbf, 0x01];
      for (final s in ['-_-_AQ', '+/+/AQ==', '+/+/AQ', '-_-_ AQ==\n']) {
        expect(decodeBase64Flexible(s), bytes, reason: s);
        expect(normalizeBase64Url(s), '-_-_AQ', reason: s);
      }
      expect(() => decodeBase64Flexible('a*b'), throwsFormatException);
      expect(() => decodeBase64Flexible('abcde'), throwsFormatException);
    });
  });

  group('Passkey record', () {
    test('keeps unknown keys and normalises the ids', () {
      final p = Passkey.fromJson({
        'credential_id': 'q2D0bQ2u3E7XoWlS1yZbVw==',
        'rp_id': 'example.com',
        'user_handle': 'dXNlci0xMjM0',
        'user_name': 'alice',
        'key_algorithm': -7,
        'private_key': p256Long,
        'sign_count': 0,
        'discoverable': true,
        'prf_secret': 'abc',
        'created_at': '2026-11-14T03:00:00Z',
        'transports': ['internal'],
      });
      expect(p.credentialId, 'q2D0bQ2u3E7XoWlS1yZbVw');
      expect(p.created, DateTime.utc(2026, 11, 14, 3).toLocal());
      final json = p.toJson();
      expect(json['transports'], ['internal']);
      expect(json['prf_secret'], 'abc');
      expect(json.containsKey('rp_name'), isFalse);
    });

    test('created_at is RFC 3339 UTC without fractions', () {
      expect(rfc3339Utc(DateTime.utc(2026, 1, 2, 3, 4, 5, 678)), '2026-01-02T03:04:05Z');
    });
  });

  group('entry rules', () {
    test('an rp id matches its host and subdomains on a label boundary', () {
      expect(hostMatchesRpId(entryHost('https://example.com/login'), 'example.com'), isTrue);
      expect(hostMatchesRpId(entryHost('https://www.example.com'), 'example.com'), isTrue);
      expect(hostMatchesRpId(entryHost('example.com'), 'example.com'), isTrue);
      expect(hostMatchesRpId(entryHost('https://badexample.com'), 'example.com'), isFalse);
      expect(hostMatchesRpId(entryHost('https://example.com'), 'www.example.com'), isFalse);
      expect(hostMatchesRpId(entryHost(''), 'example.com'), isFalse);
    });

    test('a record replaces the same id, or the same account when asked', () {
      final a = {'credential_id': 'AAAA', 'rp_id': 'x.com', 'user_handle': 'dQ', 'n': 1};
      final b = {'credential_id': 'BBBB', 'rp_id': 'x.com', 'user_handle': 'dQ', 'n': 2};
      final c = {'credential_id': 'CCCC', 'rp_id': 'y.com', 'user_handle': 'dQ', 'n': 3};
      final data = {
        'passkeys': [a, c]
      };
      expect(upsertPasskey(data, b), [b, c]);
      expect(upsertPasskey(data, b, sameAccountReplaces: false), [a, c, b]);
      expect(upsertPasskey(data, {...a, 'n': 9}), [
        {...a, 'n': 9},
        c,
      ]);
    });

    test('a record from the extension is normalised, its key too', () {
      final out = normalizePasskeyRecord({
        'credential_id': 'q2D0bQ2u3E7XoWlS1yZbVw==',
        'rp_id': 'Example.COM.',
        'user_handle': 'dXNlci0xMjM0',
        'private_key': base64.encode(decodeBase64Flexible(p256Short)),
        'sign_count': 7,
        'extra': 'kept',
      });
      expect(out['credential_id'], 'q2D0bQ2u3E7XoWlS1yZbVw');
      expect(out['rp_id'], 'example.com');
      expect(out['private_key'], p256Long);
      expect(out['sign_count'], 0);
      expect(out['key_algorithm'], -7);
      expect(out['discoverable'], true);
      expect(out['extra'], 'kept');
      expect(out['created_at'], isA<String>());
    });
  });

  test('share links leave passkeys out and the entry keeps them', () {
    final data = <String, dynamic>{
      'username': 'alice',
      'passkeys': [
        {'credential_id': 'AAAA', 'rp_id': 'example.com', 'private_key': 'k'},
      ],
    };
    final shared = withoutPasskeys(data);
    expect(shared.containsKey('passkeys'), isFalse);
    expect(shared['username'], 'alice');
    expect(data['passkeys'], hasLength(1));
  });
}

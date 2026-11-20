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

// The records the system autofill caches get: the shared credential and the
// detail fields only the iOS and macOS AutoFill extensions receive.

import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/api/models.dart';
import 'package:passbubble/core/autofill/autofill_payload.dart';

EntryResponse _entry({bool favorite = false, String type = 'password'}) => EntryResponse(
      id: 'e1',
      type: type,
      name: 'Mail',
      url: 'https://mail.example.com',
      encryptedData: '',
      dataNonce: '',
      permission: 'owner',
      favorite: favorite,
      createdAt: '',
      updatedAt: '',
    );

void main() {
  group('autofillCredential', () {
    test('a login keeps the fields every platform had so far', () {
      expect(autofillCredential(_entry(), {'username': 'alice', 'password': 'pw'}), {
        'id': 'e1',
        'url': 'https://mail.example.com',
        'name': 'Mail',
        'type': 'password',
        'username': 'alice',
        'password': 'pw',
        'totp': '',
      });
    });

    test('totp is the bare key, also from an otpauth URL', () {
      final cred = autofillCredential(_entry(), {
        'password': '',
        'totp_secret': 'otpauth://totp/x?secret=jbsw%20y3dp&digits=8',
      });
      expect(cred!['totp'], 'JBSWY3DP');
    });

    test('no password and no usable TOTP key: not offered', () {
      expect(autofillCredential(_entry(), {'username': 'alice', 'passkeys': []}), isNull);
      expect(autofillCredential(_entry(), {'totp_secret': 'otpauth://totp/x'}), isNull);
    });

    test('an account name stands in for the username', () {
      expect(
          autofillCredential(_entry(type: 'api-key'), {'account': 'svc', 'password': 'k'})![
              'username'],
          'svc');
    });
  });

  group('appleCredentialDetails', () {
    test('notes, TOTP options, favorite and the text fields', () {
      final details = appleCredentialDetails(_entry(favorite: true), {
        'password': 'pw',
        'notes': 'Door code 1234',
        'totp_secret': 'JBSWY3DPEHPK3PXP',
        'algorithm': 'sha256',
        'digits': '8',
        'period': 60,
        'custom_fields': [
          {'label': 'PIN', 'type': 'password', 'value': '4711'},
          {'label': 'Recovery', 'type': 'note', 'value': 'line 1\nline 2'},
          {'label': 'Contract', 'type': 'file', 'value': 'QUJD', 'filename': 'c.pdf'},
          {'label': '', 'type': 'text', 'value': 'no label'},
          {'label': 'Customer no.', 'value': 12345},
          {'label': 'Odd', 'type': 'something-new', 'value': 'x'},
          {'label': 'Backup 2FA', 'type': 'totp', 'value': 'gezd gnbv'},
          {'label': 'Broken 2FA', 'type': 'totp', 'value': 'otpauth://totp/x'},
          'not a map',
        ],
      });
      expect(details, {
        'favorite': true,
        'notes': 'Door code 1234',
        'totp_algorithm': 'SHA256',
        'totp_digits': 8,
        'totp_period': 60,
        'fields': [
          {'label': 'PIN', 'type': 'password', 'value': '4711'},
          {'label': 'Recovery', 'type': 'note', 'value': 'line 1\nline 2'},
          {'label': 'Customer no.', 'type': 'text', 'value': '12345'},
          {'label': 'Odd', 'type': 'text', 'value': 'x'},
          {
            'label': 'Backup 2FA',
            'type': 'totp',
            'value': 'GEZDGNBV',
            'totp_algorithm': 'SHA256',
            'totp_digits': 8,
            'totp_period': 60,
          },
          {'label': 'Broken 2FA', 'type': 'password', 'value': 'otpauth://totp/x'},
        ],
      });
    });

    test('a plain login gets only the favorite flag', () {
      expect(appleCredentialDetails(_entry(), {'password': 'pw', 'notes': '  '}),
          {'favorite': false});
    });

    test('the options of an otpauth URL win over the stored ones', () {
      final details = appleCredentialDetails(_entry(), {
        'totp_secret': 'otpauth://totp/x?secret=JBSWY3DP&algorithm=SHA512&period=15',
        'digits': 8,
      });
      expect(details['totp_algorithm'], 'SHA512');
      expect(details['totp_digits'], 6);
      expect(details['totp_period'], 15);
    });
  });
}

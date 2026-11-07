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
import 'package:passbubble/core/jobs/job_runner.dart';

void main() {
  group('mergeImportedData', () {
    test('values in the file replace existing ones, everything else stays', () {
      final merged = mergeImportedData(
        {
          'username': 'old-user',
          'password': 'old-pw',
          'notes': 'old notes', // known, absent from the import → stays
          'totp_secret': 'JBSWY3DPEHPK3PXP',
          'tags': ['work', 'client'],
          'sign_in_with': 'google',
          'iban': 'AT00 1234',
          'other_client_key': {'nested': true},
        },
        {
          'username': 'new-user',
          'password': 'new-pw',
        },
      );

      expect(merged['username'], 'new-user');
      expect(merged['password'], 'new-pw');
      expect(merged['notes'], 'old notes');
      expect(merged['totp_secret'], 'JBSWY3DPEHPK3PXP');
      expect(merged['tags'], ['work', 'client']);
      expect(merged['sign_in_with'], 'google');
      expect(merged['iban'], 'AT00 1234');
      expect(merged['other_client_key'], {'nested': true});
    });

    test('takes the imported tags and secret when the record has them', () {
      final merged = mergeImportedData(
        {
          'totp_secret': 'OLD',
          'tags': ['work', 'client'],
        },
        {
          'totp_secret': 'NEW',
          'tags': ['imported'],
        },
      );

      expect(merged['tags'], ['imported']);
      expect(merged['totp_secret'], 'NEW');
    });

    group('custom fields', () {
      List<dynamic> mergeFields(List<dynamic>? existing, List<dynamic> imported) =>
          mergeImportedData(
            existing == null ? const {} : {'custom_fields': existing},
            {'custom_fields': imported},
          )['custom_fields'] as List<dynamic>;

      const file = {
        'label': 'Doc',
        'value': 'QUJD',
        'type': 'file',
        'filename': 'a.pdf',
        'mime_type': 'application/pdf',
      };

      test('same label replaces value and type in place, others stay, new ones append', () {
        expect(
          mergeFields(
            [
              {'label': 'A', 'value': 'a', 'type': 'text'},
              {'label': 'B', 'value': 'b', 'type': 'password', 'id': 'cf-2', 'color': 'red'},
              file,
              {'label': 'C', 'value': 'c', 'type': 'text'},
            ],
            [
              {'label': 'New', 'value': 'n', 'type': 'text'},
              {'label': 'B', 'value': 'b2', 'type': 'text'},
            ],
          ),
          [
            {'label': 'A', 'value': 'a', 'type': 'text'},
            {'label': 'B', 'value': 'b2', 'type': 'text', 'id': 'cf-2', 'color': 'red'},
            file,
            {'label': 'C', 'value': 'c', 'type': 'text'},
            {'label': 'New', 'value': 'n', 'type': 'text'},
          ],
        );
      });

      test('no custom fields in the file leaves them all alone', () {
        final existing = {
          'custom_fields': [file],
        };
        expect(mergeImportedData(existing, const {})['custom_fields'], [file]);
      });

      test('an empty value in the file changes nothing', () {
        expect(
          mergeFields(
            [
              {'label': 'A', 'value': 'a', 'type': 'password'},
            ],
            [
              {'label': 'A', 'value': '', 'type': 'text'},
            ],
          ),
          [
            {'label': 'A', 'value': 'a', 'type': 'password'},
          ],
        );
      });

      test('a text field over a file field drops the file details, and the reverse brings them', () {
        expect(
          mergeFields([file], [
            {'label': 'Doc', 'value': 'plain', 'type': 'text'},
          ]),
          [
            {'label': 'Doc', 'value': 'plain', 'type': 'text'},
          ],
        );
        expect(
          mergeFields([
            {'label': 'Doc', 'value': 'plain', 'type': 'text', 'id': 'cf-1'},
          ], [file]),
          [
            {...file, 'id': 'cf-1'},
          ],
        );
      });

      test('a label twice matches one field each, so nothing is lost', () {
        expect(
          mergeFields(
            [
              {'label': 'A', 'value': '1', 'type': 'text'},
              {'label': 'A', 'value': '2', 'type': 'text'},
              {'label': 'B', 'value': 'b', 'type': 'text'},
            ],
            [
              {'label': 'A', 'value': 'x', 'type': 'text'},
              {'label': 'B', 'value': 'y', 'type': 'text'},
              {'label': 'B', 'value': 'z', 'type': 'text'},
            ],
          ),
          [
            {'label': 'A', 'value': 'x', 'type': 'text'},
            {'label': 'A', 'value': '2', 'type': 'text'},
            {'label': 'B', 'value': 'y', 'type': 'text'},
            {'label': 'B', 'value': 'z', 'type': 'text'},
          ],
        );
      });

      test('items of unknown shape stay, and a missing list is created', () {
        expect(
          mergeFields(
            ['odd', 7],
            [
              {'label': 'A', 'value': 'a', 'type': 'text'},
            ],
          ),
          [
            'odd',
            7,
            {'label': 'A', 'value': 'a', 'type': 'text'},
          ],
        );
        expect(mergeFields(null, [file]), [file]);
      });
    });

    group('the stored TOTP options', () {
      const options = {'period': 60, 'digits': 8, 'algorithm': 'SHA256'};
      const old = {'totp_secret': 'JBSWY3DPEHPK3PXP', ...options};

      void expectKept(Map<String, dynamic> merged) => expect(merged, containsPair('period', 60));
      void expectGone(Map<String, dynamic> merged) {
        expect(merged.containsKey('period'), isFalse);
        expect(merged.containsKey('digits'), isFalse);
        expect(merged.containsKey('algorithm'), isFalse);
      }

      test('go when a different secret comes in without any', () {
        final merged = mergeImportedData(old, {'totp_secret': 'GEZDGNBVGY3TQOJQ'});
        expect(merged['totp_secret'], 'GEZDGNBVGY3TQOJQ');
        expectGone(merged);
      });

      test('go as well when no secret was stored next to them', () {
        expectGone(mergeImportedData(options, {'totp_secret': 'GEZDGNBVGY3TQOJQ'}));
      });

      test('stay when the file brings no secret', () {
        final merged = mergeImportedData(old, {'password': 'new-pw'});
        expect(merged['totp_secret'], old['totp_secret']);
        expect(merged['digits'], 8);
        expect(merged['algorithm'], 'SHA256');
        expectKept(merged);
      });

      test('stay when the file brings the same key, however it is spelled', () {
        expectKept(mergeImportedData(old, {'totp_secret': 'JBSWY3DPEHPK3PXP'}));
        expectKept(mergeImportedData(old, {'totp_secret': 'jbsw y3dp ehpk 3pxp'}));
      });

      test('options in the file win, and the ones it lacks still go', () {
        final merged = mergeImportedData(old, {'totp_secret': 'GEZDGNBVGY3TQOJQ', 'period': 45});
        expect(merged['period'], 45);
        expect(merged.containsKey('digits'), isFalse);
        expect(merged.containsKey('algorithm'), isFalse);
      });

      group('next to a custom totp field', () {
        Map<String, dynamic> withField(String value) => {
              ...old,
              'custom_fields': [
                {'label': 'Bank', 'value': value, 'type': 'totp'},
              ],
            };

        test('stay while it holds another key: it reads the same options', () {
          expectKept(mergeImportedData(
              withField('MFRGGZDFMZTWQ2LK'), {'totp_secret': 'GEZDGNBVGY3TQOJQ'}));
        });

        test('go when it holds the new key', () {
          expectGone(mergeImportedData(
              withField('GEZDGNBVGY3TQOJQ'), {'totp_secret': 'GEZDGNBVGY3TQOJQ'}));
        });
      });
    });

    group('a stray totp_secret', () {
      test('goes, so a new password cannot turn it into a real secret', () {
        final merged = mergeImportedData(
          {'password': 'old-pw', 'totp_secret': 'old-pw'},
          {'password': 'new-pw'},
        );
        expect(merged['password'], 'new-pw');
        expect(merged.containsKey('totp_secret'), isFalse);
      });

      test('goes as well when the file leaves the password alone', () {
        final merged = mergeImportedData(
          {'password': 'pw', 'totp_secret': 'pw'},
          {'notes': 'n'},
        );
        expect(merged['password'], 'pw');
        expect(merged.containsKey('totp_secret'), isFalse);
      });

      test('a secret in the file still counts', () {
        final merged = mergeImportedData(
          {'password': 'pw', 'totp_secret': 'pw'},
          {'totp_secret': 'JBSWY3DPEHPK3PXP'},
        );
        expect(merged['totp_secret'], 'JBSWY3DPEHPK3PXP');
      });
    });
  });
}

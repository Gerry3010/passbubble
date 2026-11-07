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
import 'package:otp/otp.dart';
import 'package:passbubble/core/util/entry_totp.dart';

// RFC 6238 appendix B: T = 59 s, 8 digits, one key per algorithm.
const _sha1Key = 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ';
const _sha256Key = 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZA====';
const _sha512Key = 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ'
    'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNA=';
const _t59 = 59 * 1000;

void main() {
  group('totpSecretOf', () {
    test('a value equal to the password counts as absent', () {
      const data = {'password': 'same', 'totp_secret': 'same'};
      expect(isStrayTotpSecret(data), isTrue);
      expect(totpSecretOf(data), isNull);
    });

    test('a real secret stays untouched', () {
      const data = {'password': 'pw', 'totp_secret': 'JBSWY3DPEHPK3PXP'};
      expect(isStrayTotpSecret(data), isFalse);
      expect(totpSecretOf(data), 'JBSWY3DPEHPK3PXP');
    });

    test('an entry with no password keeps its secret', () {
      const data = {'totp_secret': 'JBSWY3DPEHPK3PXP'};
      expect(isStrayTotpSecret(data), isFalse);
      expect(totpSecretOf(data), 'JBSWY3DPEHPK3PXP');
    });

    test('nothing there, empty or not a string gives null and is not stray', () {
      for (final data in <Map<String, dynamic>>[
        const {},
        const {'password': 'pw'},
        const {'password': '', 'totp_secret': ''},
        const {'totp_secret': 7},
      ]) {
        expect(totpSecretOf(data), isNull, reason: '$data');
        expect(isStrayTotpSecret(data), isFalse, reason: '$data');
      }
    });
  });

  group('TotpOptions.stored', () {
    test('reads the keys other clients store', () {
      final o = TotpOptions.stored(const {'period': 60, 'digits': 8, 'algorithm': 'SHA256'});
      expect(o.period, 60);
      expect(o.digits, 8);
      expect(o.algorithm, Algorithm.SHA256);
      expect(TotpOptions.stored(const {'algorithm': 'sha512'}).algorithm, Algorithm.SHA512);
      expect(TotpOptions.stored(const {'period': '45', 'digits': '8'}).period, 45);
      expect(TotpOptions.stored(const {'period': '45', 'digits': '8'}).digits, 8);
    });

    test('unset or unknown values mean the defaults', () {
      for (final data in <Map<String, dynamic>>[
        const {},
        const {'period': 0, 'digits': 7, 'algorithm': 'MD5'},
        const {'period': -30, 'digits': 'x', 'algorithm': 5},
      ]) {
        final o = TotpOptions.stored(data);
        expect(o.period, 30, reason: '$data');
        expect(o.digits, 6, reason: '$data');
        expect(o.algorithm, Algorithm.SHA1, reason: '$data');
      }
    });
  });

  group('totpCodeAt', () {
    test('matches the RFC 6238 vectors for each algorithm', () {
      const eight = TotpOptions(digits: 8);
      expect(totpCodeAt(_sha1Key, eight, _t59)!.code, '94287082');
      expect(
          totpCodeAt(
                  _sha256Key, const TotpOptions(digits: 8, algorithm: Algorithm.SHA256), _t59)!
              .code,
          '46119246');
      expect(
          totpCodeAt(
                  _sha512Key, const TotpOptions(digits: 8, algorithm: Algorithm.SHA512), _t59)!
              .code,
          '90693936');
    });

    test('defaults are SHA1, six digits, 30 seconds', () {
      final code = totpCodeAt(_sha1Key, const TotpOptions(), _t59)!;
      expect(code.code, '287082');
      expect(code.remaining, 1);
    });

    test('the period sets the time step and the countdown', () {
      // Counter 1 lasts from 60 s to 119 s at a period of 60.
      final code = totpCodeAt(_sha1Key, const TotpOptions(period: 60), 119 * 1000)!;
      expect(code.code, '287082');
      expect(code.remaining, 1);
      expect(totpCodeAt(_sha1Key, const TotpOptions(period: 60), 60 * 1000)!.remaining, 60);
    });

    test('accepts grouped and lowercase secrets', () {
      final spaced = _sha1Key.toLowerCase().replaceAllMapped(
          RegExp(r'.{4}'), (m) => '${m[0]} ');
      expect(totpCodeAt(spaced, const TotpOptions(), _t59)!.code, '287082');
      expect(totpCodeAt('GEZD-GNBV_GY3T QOJQ GEZD GNBV GY3T QOJQ', const TotpOptions(), _t59)!.code,
          '287082');
    });

    test('an otpauth URL brings its own secret and options', () {
      const stored = TotpOptions(period: 300, digits: 6, algorithm: Algorithm.SHA512);
      final code = totpCodeAt(
        'otpauth://totp/ACME:alice?secret=$_sha1Key&digits=8&period=30&algorithm=SHA1&issuer=ACME',
        stored,
        _t59,
      )!;
      expect(code.code, '94287082');
      expect(code.remaining, 1);
      // Parameters it leaves out mean the defaults, not the stored ones.
      expect(totpCodeAt('otpauth://totp/x?secret=$_sha1Key', stored, _t59)!.code, '287082');
    });

    test('a value that cannot give a code returns null', () {
      for (final bad in [
        '',
        '   ',
        '-_-',
        'not base32 !',
        'otpauth://totp/x',
        'otpauth://totp/x?secret=',
        'otpauth://totp/x?secret=%zz',
      ]) {
        expect(totpCodeAt(bad, const TotpOptions(), _t59), isNull, reason: bad);
      }
    });
  });

  group('isSameTotpKey', () {
    test('ignores grouping and case, and nothing else', () {
      expect(isSameTotpKey('JBSWY3DPEHPK3PXP', 'jbsw y3dp-ehpk_3pxp'), isTrue);
      expect(isSameTotpKey('JBSWY3DPEHPK3PXP====', 'JBSWY3DPEHPK3PXP'), isTrue);
      expect(isSameTotpKey('JBSWY3DPEHPK3PXP', 'GEZDGNBVGY3TQOJQ'), isFalse);
      expect(isSameTotpKey('otpauth://totp/x?secret=JBSWY3DPEHPK3PXP', 'JBSWY3DPEHPK3PXP'), isFalse);
      expect(isSameTotpKey('', 'GEZDGNBVGY3TQOJQ'), isFalse);
    });
  });
}

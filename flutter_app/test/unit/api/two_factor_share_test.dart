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
  group('LoginResponse 2FA', () {
    test('parses a 2fa_required response', () {
      final r = LoginResponse.fromJson({
        'status': '2fa_required',
        'pending_token': 'pending-abc',
        'expires_in': 300,
      });
      expect(r.requiresTotp, isTrue);
      expect(r.pendingToken, 'pending-abc');
      expect(r.accessToken, isEmpty);
    });

    test('a normal login does not require TOTP', () {
      final r = LoginResponse.fromJson({
        'access_token': 'a',
        'refresh_token': 'b',
        'user_id': 'u1',
      });
      expect(r.requiresTotp, isFalse);
      expect(r.accessToken, 'a');
    });
  });

  test('SetupTotpResponse parses secret + url', () {
    final s = SetupTotpResponse.fromJson({
      'secret': 'JBSWY3DPEHPK3PXP',
      'otpauth_url': 'otpauth://totp/Passbubble:a@b.c?secret=JBSWY3DPEHPK3PXP',
    });
    expect(s.secret, 'JBSWY3DPEHPK3PXP');
    expect(s.otpauthUrl, contains('otpauth://'));
  });

  test('CreateShareLinkRequest omits null optionals', () {
    final json = const CreateShareLinkRequest(
      encryptedPayload: 'Y2lwaGVy',
      payloadNonce: 'bm9uY2U=',
      expiresAt: '2026-07-01T00:00:00Z',
    ).toJson();
    expect(json.containsKey('max_views'), isFalse);
    expect(json.containsKey('password'), isFalse);
    expect(json['encrypted_payload'], 'Y2lwaGVy');
  });

  test('CreateShareLinkRequest includes optionals when set', () {
    final json = const CreateShareLinkRequest(
      encryptedPayload: 'x',
      payloadNonce: 'y',
      expiresAt: '2026-07-01T00:00:00Z',
      maxViews: 5,
      password: 'hunter2',
    ).toJson();
    expect(json['max_views'], 5);
    expect(json['password'], 'hunter2');
  });

  test('ShareLinkResponse parses the create response (token present)', () {
    final r = ShareLinkResponse.fromJson({
      'id': 'l1',
      'token': 'tok123',
      'expires_at': '2026-07-01T00:00:00Z',
      'has_password': true,
    });
    expect(r.token, 'tok123');
    expect(r.hasPassword, isTrue);
  });

  test('PublicShareLinkResponse signals password requirement', () {
    final r = PublicShareLinkResponse.fromJson({'requires_password': true});
    expect(r.requiresPassword, isTrue);
    expect(r.encryptedPayload, isEmpty);
  });
}

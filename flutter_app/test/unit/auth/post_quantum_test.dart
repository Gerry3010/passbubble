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

import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/auth/auth_service.dart';

void main() {
  group('AuthService.isPlaceholderMlkemKey', () {
    test('null / empty → placeholder (needs upgrade)', () {
      expect(AuthService.isPlaceholderMlkemKey(null), isTrue);
      expect(AuthService.isPlaceholderMlkemKey(''), isTrue);
    });

    test('32-byte X25519 placeholder → needs upgrade', () {
      expect(AuthService.isPlaceholderMlkemKey(base64.encode(Uint8List(32))), isTrue);
    });

    test('real 1184-byte ML-KEM key → already upgraded', () {
      expect(AuthService.isPlaceholderMlkemKey(base64.encode(Uint8List(1184))), isFalse);
    });

    test('invalid base64 → placeholder (needs upgrade)', () {
      expect(AuthService.isPlaceholderMlkemKey('!!!not base64!!!'), isTrue);
    });
  });
}

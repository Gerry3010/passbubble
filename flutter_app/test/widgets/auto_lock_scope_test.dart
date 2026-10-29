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

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/api/api_client.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/core/auth/auto_lock_provider.dart';
import 'package:passbubble/core/auth/auto_lock_scope.dart';

class _UnlockedAuth extends AuthNotifier {
  _UnlockedAuth(super.svc) {
    state = const AuthState(isLoggedIn: true, isUnlocked: true, userId: 'u');
  }
}

/// A 1-minute auto-lock; returns the container to inspect the auth state.
Future<ProviderContainer> _pumpScope(WidgetTester tester) async {
  FlutterSecureStorage.setMockInitialValues({'auto_lock_minutes': '1'});
  final container = ProviderContainer(overrides: [
    authStateProvider
        .overrideWith((ref) => _UnlockedAuth(AuthService(ApiClient()))),
  ]);
  addTearDown(container.dispose);
  await tester.pumpWidget(UncontrolledProviderScope(
    container: container,
    child: const MaterialApp(
      home: AutoLockScope(child: Scaffold(body: TextField(autofocus: true))),
    ),
  ));
  await tester.pump();
  expect(container.read(autoLockProvider), 1);
  return container;
}

bool _unlocked(ProviderContainer c) => c.read(authStateProvider).isUnlocked;

void main() {
  testWidgets('locks after the idle interval', (tester) async {
    final c = await _pumpScope(tester);
    await tester.pump(const Duration(seconds: 61));
    expect(_unlocked(c), isFalse);
  });

  testWidgets('typing counts as activity', (tester) async {
    final c = await _pumpScope(tester);
    await tester.pump(const Duration(seconds: 50));
    await tester.sendKeyEvent(LogicalKeyboardKey.keyA);
    await tester.pump(const Duration(seconds: 50));
    expect(_unlocked(c), isTrue, reason: '100 s total, but typed at 50 s');

    await tester.pump(const Duration(seconds: 11));
    expect(_unlocked(c), isFalse);
  });

  testWidgets('mouse hover counts as activity', (tester) async {
    final c = await _pumpScope(tester);
    final mouse = await tester.createGesture(kind: PointerDeviceKind.mouse);
    await mouse.addPointer(location: const Offset(10, 10));
    addTearDown(mouse.removePointer);

    await tester.pump(const Duration(seconds: 50));
    await mouse.moveTo(const Offset(40, 40));
    await tester.pump(const Duration(seconds: 50));
    expect(_unlocked(c), isTrue, reason: 'hovered at 50 s');

    await tester.pump(const Duration(seconds: 11));
    expect(_unlocked(c), isFalse);
  });
}

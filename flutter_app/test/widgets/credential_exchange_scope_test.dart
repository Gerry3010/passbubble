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

// CXF data handed over by the native side waits while the vault is locked and
// opens the import screen once it is open.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:passbubble/core/api/api_client.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/core/autofill/autofill_service.dart';
import 'package:passbubble/core/autofill/credential_exchange_scope.dart';
import 'package:passbubble/core/importexport/credential_exchange.dart';
import 'package:passbubble/core/router/router.dart';

class _CapturingBridge extends AutofillBridge {
  void Function(String json)? handler;
  int registrations = 0;

  @override
  void setCredentialExchangeHandler(void Function(String json)? onReceived) {
    handler = onReceived;
    if (onReceived != null) registrations++;
  }
}

class _Auth extends AuthNotifier {
  _Auth(super.svc, AuthState initial) {
    state = initial;
  }

  void set(AuthState s) => state = s;
}

void main() {
  const locked = AuthState(isLoggedIn: true, isUnlocked: false, userId: 'u1');
  const open = AuthState(isLoggedIn: true, isUnlocked: true, userId: 'u1');

  Future<({_CapturingBridge bridge, _Auth auth, ProviderContainer container})> pump(
      WidgetTester tester, AuthState initial) async {
    final bridge = _CapturingBridge();
    final auth = _Auth(AuthService(ApiClient()), initial);
    final router = GoRouter(
      initialLocation: '/entries',
      routes: [
        GoRoute(path: '/entries', builder: (_, _) => const Text('LIST')),
        GoRoute(
          path: '/manage',
          builder: (_, s) => Text('MANAGE tab ${s.uri.queryParameters['tab']}'),
        ),
      ],
    );
    final container = ProviderContainer(overrides: [
      autofillBridgeProvider.overrideWithValue(bridge),
      authStateProvider.overrideWith((ref) => auth),
      routerProvider.overrideWithValue(router),
    ]);
    addTearDown(container.dispose);
    await tester.pumpWidget(UncontrolledProviderScope(
      container: container,
      child: CredentialExchangeScope(child: MaterialApp.router(routerConfig: router)),
    ));
    await tester.pumpAndSettle();
    return (bridge: bridge, auth: auth, container: container);
  }

  testWidgets('received while locked: waits, then opens the import after unlock',
      (tester) async {
    final s = await pump(tester, locked);
    expect(s.bridge.registrations, 1);

    s.bridge.handler!('{"accounts":[]}');
    await tester.pumpAndSettle();
    expect(find.text('LIST'), findsOneWidget);
    expect(s.container.read(pendingCredentialExchangeProvider), '{"accounts":[]}');

    s.auth.set(open);
    await tester.pumpAndSettle();
    expect(find.text('MANAGE tab 0'), findsOneWidget);
    expect(s.container.read(pendingCredentialExchangeProvider), '{"accounts":[]}',
        reason: 'the import screen takes it from here');
  });

  testWidgets('received while open: the import opens at once, and only once',
      (tester) async {
    final s = await pump(tester, open);
    s.bridge.handler!('{"accounts":[]}');
    await tester.pumpAndSettle();
    expect(find.text('MANAGE tab 0'), findsOneWidget);

    s.container.read(routerProvider).go('/entries');
    await tester.pumpAndSettle();
    s.auth.set(locked);
    await tester.pumpAndSettle();
    s.auth.set(open);
    await tester.pumpAndSettle();
    expect(find.text('LIST'), findsOneWidget, reason: 'a later unlock does not open it again');
  });

  testWidgets('a logout drops data still waiting', (tester) async {
    final s = await pump(tester, locked);
    s.bridge.handler!('{"accounts":[]}');
    await tester.pumpAndSettle();
    s.auth.set(const AuthState());
    await tester.pumpAndSettle();
    expect(s.container.read(pendingCredentialExchangeProvider), isNull);
  });

  testWidgets('the handler goes with the scope', (tester) async {
    final s = await pump(tester, open);
    await tester.pumpWidget(const SizedBox());
    expect(s.bridge.handler, isNull);
  });
}

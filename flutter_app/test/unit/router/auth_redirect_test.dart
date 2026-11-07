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

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/core/router/router.dart';

const _loggedOut = AuthState();
const _locked = AuthState(isLoggedIn: true, userId: 'u');
const _unlocked = AuthState(isLoggedIn: true, isUnlocked: true, userId: 'u');

String? _redirect(AuthState auth, String location) =>
    authRedirect(auth, Uri.parse(location), apiConfigured: true);

void main() {
  group('authRedirect', () {
    test('locking sends the current page along as from', () {
      expect(_redirect(_locked, '/entries/e1'), '/unlock?from=%2Fentries%2Fe1');
      expect(_redirect(_locked, '/entries/e1/edit'),
          '/unlock?from=%2Fentries%2Fe1%2Fedit');
      expect(_redirect(_locked, '/entries?tag=work'),
          '/unlock?from=%2Fentries%3Ftag%3Dwork');
      expect(_redirect(_locked, '/settings'), '/unlock?from=%2Fsettings');
    });

    test('the vault list needs no from', () {
      expect(_redirect(_locked, '/entries'), '/unlock');
      expect(_redirect(_loggedOut, '/entries'), '/login');
    });

    test('logging out sends the current page along as from', () {
      expect(_redirect(_loggedOut, '/settings/update'),
          '/login?from=%2Fsettings%2Fupdate');
    });

    test('a session that ends on the unlock screen keeps its from', () {
      expect(_redirect(_loggedOut, '/unlock?from=%2Fentries%2Fe1'),
          '/login?from=%2Fentries%2Fe1');
      expect(_redirect(_loggedOut, '/unlock'), '/login');
    });

    test('unlock and login return to from', () {
      expect(_redirect(_unlocked, '/unlock?from=%2Fentries%2Fe1'),
          '/entries/e1');
      expect(_redirect(_unlocked, '/login?from=%2Fentries%3Ftag%3Dwork'),
          '/entries?tag=work');
      expect(_redirect(_unlocked, '/unlock'), '/entries');
    });

    test('a from outside the app falls back to the vault list', () {
      for (final from in [
        'https://evil.example/entries',
        '//evil.example/entries',
        'javascript:alert(1)',
        '/login',
        '/unlock',
        '/setup',
        '/share/abc',
        '/entries/../setup',
        '/entriesx',
        '/',
      ]) {
        final location =
            Uri(path: '/unlock', queryParameters: {'from': from}).toString();
        expect(_redirect(_unlocked, location), '/entries', reason: from);
        expect(_redirect(_loggedOut, location), '/login', reason: from);
      }
    });

    test('the open vault and the public pages stay put', () {
      expect(_redirect(_unlocked, '/entries/e1'), isNull);
      expect(_redirect(_locked, '/unlock?from=%2Fentries%2Fe1'), isNull);
      expect(_redirect(_loggedOut, '/login?from=%2Fentries%2Fe1'), isNull);
      expect(_redirect(_loggedOut, '/share/t?k=x'), isNull);
      expect(_redirect(_loggedOut, '/register?token=a'), isNull);
    });

    test('a missing server URL still leads to setup', () {
      expect(
        authRedirect(_unlocked, Uri.parse('/entries/e1'),
            apiConfigured: false),
        '/setup',
      );
    });
  });

  group('launchLocation', () {
    test('a reloaded page opens where it was', () {
      expect(launchLocation('/entries/e1'), '/entries/e1');
      expect(launchLocation('/settings/update'), '/settings/update');
      expect(launchLocation('/unlock?from=%2Fentries%2Fe1'), '/entries/e1');
      expect(launchLocation('/login?from=%2Fentries%2Fe1'), '/entries/e1');
    });

    test('public deep links open as they are', () {
      expect(launchLocation('/share/t?k=x'), '/share/t?k=x');
      expect(launchLocation('/register?token=a'), '/register?token=a');
    });

    test('anything else opens the vault list', () {
      for (final fragment in [
        '',
        '/',
        '/setup',
        '/unlock',
        '/nowhere',
        '/unlock?from=https%3A%2F%2Fevil.example',
      ]) {
        expect(launchLocation(fragment), '/entries', reason: fragment);
      }
    });
  });

  group('with go_router', () {
    late AuthState auth;
    late ValueNotifier<int> refresh;

    GoRouter build(String initialLocation) {
      refresh = ValueNotifier(0);
      addTearDown(refresh.dispose);
      Widget page(String text) => Scaffold(body: Text(text));
      final router = GoRouter(
        initialLocation: initialLocation,
        refreshListenable: refresh,
        redirect: (_, state) =>
            authRedirect(auth, state.uri, apiConfigured: true),
        routes: [
          GoRoute(path: '/login', builder: (_, _) => page('login')),
          GoRoute(path: '/unlock', builder: (_, _) => page('unlock')),
          GoRoute(
            path: '/entries',
            builder: (_, _) => page('list'),
            routes: [
              GoRoute(
                path: ':id',
                builder: (_, state) =>
                    page('entry ${state.pathParameters['id']}'),
              ),
            ],
          ),
        ],
      );
      addTearDown(router.dispose);
      return router;
    }

    Future<void> setAuth(WidgetTester tester, AuthState next) async {
      auth = next;
      refresh.value++;
      await tester.pumpAndSettle();
    }

    String location(GoRouter router) =>
        router.routerDelegate.currentConfiguration.uri.toString();

    testWidgets('auto-lock, session expiry and login lead back to the entry',
        (tester) async {
      auth = _unlocked;
      final router = build('/entries/e1');
      await tester.pumpWidget(MaterialApp.router(routerConfig: router));
      await tester.pumpAndSettle();
      expect(find.text('entry e1'), findsOneWidget);

      await setAuth(tester, _locked);
      expect(location(router), '/unlock?from=%2Fentries%2Fe1');
      expect(find.text('unlock'), findsOneWidget);

      await setAuth(tester, _loggedOut);
      expect(location(router), '/login?from=%2Fentries%2Fe1');

      await setAuth(tester, _unlocked);
      expect(location(router), '/entries/e1');
      expect(find.text('entry e1'), findsOneWidget);
    });

    testWidgets('a reload while locked returns to the entry after unlock',
        (tester) async {
      auth = _locked;
      final router = build(launchLocation('/entries/e1'));
      await tester.pumpWidget(MaterialApp.router(routerConfig: router));
      await tester.pumpAndSettle();
      expect(location(router), '/unlock?from=%2Fentries%2Fe1');

      await setAuth(tester, _unlocked);
      expect(find.text('entry e1'), findsOneWidget);
    });

    testWidgets('a deliberate sign-out ends on the plain login screen',
        (tester) async {
      auth = _unlocked;
      final router = build('/entries/e1');
      await tester.pumpWidget(MaterialApp.router(routerConfig: router));
      await tester.pumpAndSettle();

      // What the Sign Out buttons do: log out, then go to /login.
      await setAuth(tester, _loggedOut);
      router.go('/login');
      await tester.pumpAndSettle();
      expect(location(router), '/login');

      await setAuth(tester, _unlocked);
      expect(location(router), '/entries');
    });
  });
}

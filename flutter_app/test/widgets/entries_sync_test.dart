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
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/api/api_client.dart';
import 'package:passbubble/core/api/models.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/features/entries/entries_list_screen.dart';

// Counts how often the entry list is fetched, so we can assert the screen
// re-syncs on app resume.
class _CountingApiClient extends ApiClient {
  int listEntriesCalls = 0;
  Object? failWith;

  @override
  Future<List<EntryResponse>> listEntries() async {
    listEntriesCalls++;
    if (failWith != null) throw failWith!;
    return const [];
  }

  @override
  Future<List<FolderResponse>> listFolders() async => const [];
}

// The list only loads for a logged-in session.
class _LoggedInAuth extends AuthNotifier {
  _LoggedInAuth(super.svc) {
    state = const AuthState(isLoggedIn: true, isUnlocked: true, userId: 'u');
  }
}

void main() {
  testWidgets('resuming the app re-syncs the entry list', (tester) async {
    final api = _CountingApiClient();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          apiClientProvider.overrideWithValue(api),
          authStateProvider
              .overrideWith((ref) => _LoggedInAuth(AuthService(api))),
        ],
        child: const MaterialApp(home: EntriesListScreen()),
      ),
    );
    await tester.pumpAndSettle();

    final initialCalls = api.listEntriesCalls;
    expect(initialCalls, greaterThanOrEqualTo(1));

    // Simulate the app returning to the foreground (e.g. after saving a password
    // in the browser extension or on another device).
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpAndSettle();

    expect(api.listEntriesCalls, greaterThan(initialCalls),
        reason: 'entries should be re-fetched on resume');
  });

  testWidgets('a load error offers Retry, which reloads the list',
      (tester) async {
    final api = _CountingApiClient()
      ..failWith = Exception('Cannot reach the server. Check your connection.');
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          apiClientProvider.overrideWithValue(api),
          authStateProvider
              .overrideWith((ref) => _LoggedInAuth(AuthService(api))),
        ],
        child: const MaterialApp(home: EntriesListScreen()),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Cannot reach the server. Check your connection.'),
        findsOneWidget);

    api.failWith = null;
    await tester.tap(find.text('RETRY'));
    await tester.pumpAndSettle();
    expect(find.text('RETRY'), findsNothing);
    expect(find.text('Your vault is empty'), findsOneWidget);
  });
}

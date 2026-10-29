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

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/api/api_client.dart';
import 'package:passbubble/core/api/models.dart';
import 'package:passbubble/core/auth/auth_service.dart';
import 'package:passbubble/features/entries/entries_list_screen.dart';

class _FakeApi extends ApiClient {
  int listEntriesCalls = 0;
  int listFoldersCalls = 0;
  Object? failWith;

  @override
  Future<List<EntryResponse>> listEntries() async {
    listEntriesCalls++;
    if (failWith != null) throw failWith!;
    return const [];
  }

  @override
  Future<List<FolderResponse>> listFolders() async {
    listFoldersCalls++;
    return const [];
  }
}

/// AuthNotifier whose state the test drives directly.
class _TestAuth extends AuthNotifier {
  _TestAuth(super.svc, AuthState initial) {
    state = initial;
  }

  void set(AuthState s) => state = s;
}

const _alice = AuthState(isLoggedIn: true, isUnlocked: true, userId: 'alice');
const _bob = AuthState(isLoggedIn: true, isUnlocked: true, userId: 'bob');

void main() {
  late _FakeApi api;
  late _TestAuth auth;
  late ProviderContainer container;

  setUp(() {
    api = _FakeApi();
    auth = _TestAuth(AuthService(api), _alice);
    container = ProviderContainer(overrides: [
      apiClientProvider.overrideWithValue(api),
      authStateProvider.overrideWith((ref) => auth),
    ]);
    // Keep the providers alive like a mounted list screen would.
    container.listen(entriesProvider, (_, _) {});
    container.listen(foldersProvider, (_, _) {});
  });

  tearDown(() => container.dispose());

  Future<void> settle() async {
    await container.read(entriesProvider.future);
    await container.read(foldersProvider.future);
  }

  test('logout drops the list without fetching, login refetches', () async {
    await settle();
    expect(api.listEntriesCalls, 1);

    auth.set(const AuthState());
    await settle();
    expect(container.read(entriesProvider).value, isEmpty);
    expect(api.listEntriesCalls, 1, reason: 'no fetch while logged out');

    auth.set(_bob);
    await settle();
    expect(api.listEntriesCalls, 2);
    expect(api.listFoldersCalls, 2);
  });

  test('a stale error does not survive a re-login', () async {
    await settle();
    api.failWith = Exception('Session expired. Please log in again.');
    container.invalidate(entriesProvider);
    await expectLater(container.read(entriesProvider.future), throwsException);
    expect(container.read(entriesProvider).hasError, isTrue);

    api.failWith = null;
    auth.set(const AuthState());
    auth.set(_alice);
    await settle();
    expect(container.read(entriesProvider).hasError, isFalse);
  });

  test('unlock refetches, lock does not', () async {
    await settle();
    auth.set(const AuthState(isLoggedIn: true, userId: 'alice'));
    await Future<void>.delayed(Duration.zero);
    expect(api.listEntriesCalls, 1, reason: 'locking keeps the cache');

    auth.set(_alice);
    await settle();
    expect(api.listEntriesCalls, 2, reason: 'unlocking shows fresh data');
  });
}

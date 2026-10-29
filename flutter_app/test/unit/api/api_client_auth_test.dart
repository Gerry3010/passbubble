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

import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/api/api_client.dart';

/// What a request looked like when it was sent (Dio reuses the options
/// object for the retry, so the fields are copied out).
typedef _Sent = ({String path, String? bearer, Object? data});

/// Routes every request to [handle]; records them for assertions.
class _FakeAdapter implements HttpClientAdapter {
  _FakeAdapter(this.handle);

  final FutureOr<ResponseBody> Function(RequestOptions req) handle;
  final requests = <_Sent>[];

  int count(String path) => requests.where((r) => r.path.endsWith(path)).length;

  @override
  Future<ResponseBody> fetch(RequestOptions options,
      Stream<Uint8List>? requestStream, Future<void>? cancelFuture) async {
    requests.add(
        (path: options.path, bearer: _bearer(options), data: options.data));
    return handle(options);
  }

  @override
  void close({bool force = false}) {}
}

ResponseBody _json(int status, Object body) => ResponseBody.fromString(
      jsonEncode(body),
      status,
      headers: {
        Headers.contentTypeHeader: [Headers.jsonContentType],
      },
    );

String? _bearer(RequestOptions r) => r.headers['Authorization'] as String?;

void main() {
  const entries = '/api/v1/entries';
  const refresh = '/api/v1/auth/refresh';

  late Map<String, String> store;
  late bool sessionExpired;

  setUp(() {
    store = {'access_token': 'A1', 'refresh_token': 'RT1'};
    FlutterSecureStorage.setMockInitialValues(store);
    sessionExpired = false;
  });

  Future<ApiClient> client(_FakeAdapter adapter,
      {Duration settle = Duration.zero}) async {
    final api =
        ApiClient(httpClientAdapter: adapter, rotationSettleDelay: settle);
    await api.init();
    api.setBaseUrlEphemeral('http://test');
    api.setSessionExpiredCallback(() => sessionExpired = true);
    return api;
  }

  test('expired access token: refreshes once and retries the request',
      () async {
    final adapter = _FakeAdapter((r) {
      if (r.path.endsWith(refresh)) {
        return _json(200, {'access_token': 'A2', 'refresh_token': 'RT2'});
      }
      return _bearer(r) == 'Bearer A2'
          ? _json(200, [])
          : _json(401, {'error': 'unauthorized'});
    });
    final api = await client(adapter);

    expect(await api.listEntries(), isEmpty);
    expect(adapter.count(refresh), 1);
    expect(store['refresh_token'], 'RT2');
    expect(sessionExpired, isFalse);
  });

  test('concurrent 401s share one refresh', () async {
    final adapter = _FakeAdapter((r) async {
      if (r.path.endsWith(refresh)) {
        await Future<void>.delayed(const Duration(milliseconds: 10));
        return _json(200, {'access_token': 'A2', 'refresh_token': 'RT2'});
      }
      return _bearer(r) == 'Bearer A2'
          ? _json(200, [])
          : _json(401, {'error': 'unauthorized'});
    });
    final api = await client(adapter);

    await Future.wait([api.listEntries(), api.listEntries(), api.listFolders()]);
    expect(adapter.count(refresh), 1);
  });

  group('cross-tab refresh race', () {
    test('adopts the token another tab rotated in the meantime', () async {
      final adapter = _FakeAdapter((r) {
        if (r.path.endsWith(refresh)) {
          // The other tab won the race: it spent RT1 and stored its new pair.
          store['access_token'] = 'A2';
          store['refresh_token'] = 'RT2';
          return _json(401, {'error': 'session expired or revoked'});
        }
        return _bearer(r) == 'Bearer A2'
            ? _json(200, [])
            : _json(401, {'error': 'unauthorized'});
      });
      final api = await client(adapter);

      expect(await api.listEntries(), isEmpty);
      expect(sessionExpired, isFalse);
      expect(api.accessToken, 'A2');
      expect(store['refresh_token'], 'RT2');
    });

    test('waits for a winner that is still writing its tokens', () async {
      final adapter = _FakeAdapter((r) {
        if (r.path.endsWith(refresh)) {
          Timer(const Duration(milliseconds: 20), () {
            store['access_token'] = 'A2';
            store['refresh_token'] = 'RT2';
          });
          return _json(401, {'error': 'session expired or revoked'});
        }
        return _bearer(r) == 'Bearer A2'
            ? _json(200, [])
            : _json(401, {'error': 'unauthorized'});
      });
      final api =
          await client(adapter, settle: const Duration(milliseconds: 100));

      expect(await api.listEntries(), isEmpty);
      expect(sessionExpired, isFalse);
    });

    test('a genuinely rejected refresh token still ends the session',
        () async {
      final adapter = _FakeAdapter((r) => r.path.endsWith(refresh)
          ? _json(401, {'error': 'session expired or revoked'})
          : _json(401, {'error': 'unauthorized'}));
      final api = await client(adapter);

      await expectLater(api.listEntries(), throwsException);
      expect(sessionExpired, isTrue);
      expect(store.containsKey('refresh_token'), isFalse);
      expect(store.containsKey('access_token'), isFalse);
    });
  });

  group('credential endpoints bypass the refresh interceptor', () {
    test('wrong password: no refresh, no session wipe, server message shown',
        () async {
      final adapter = _FakeAdapter((r) => r.path.endsWith('/auth/login')
          ? _json(401, {'error': 'invalid credentials'})
          : _json(500, {'error': 'unexpected request'}));
      final api = await client(adapter);

      await expectLater(
        api.login('a@b.c', 'wrong'),
        throwsA(predicate((e) => e.toString().contains('invalid credentials'))),
      );
      expect(adapter.count(refresh), 0);
      expect(adapter.requests, hasLength(1));
      expect(sessionExpired, isFalse);
      expect(store['refresh_token'], 'RT1');
    });

    test('2FA verify and recovery 401s behave the same', () async {
      final adapter = _FakeAdapter((r) => _json(401, {'error': 'invalid code'}));
      final api = await client(adapter);

      await expectLater(api.verifyTotp('pending', '000000'), throwsException);
      await expectLater(api.requestTotpRecovery('pending'), throwsException);
      expect(adapter.count(refresh), 0);
      expect(sessionExpired, isFalse);
    });
  });

  group('retry after a successful refresh', () {
    test("forwards the retry's error, not the stale 401", () async {
      final adapter = _FakeAdapter((r) {
        if (r.path.endsWith(refresh)) {
          return _json(200, {'access_token': 'A2', 'refresh_token': 'RT2'});
        }
        return _bearer(r) == 'Bearer A2'
            ? _json(500, {'error': 'database unavailable'})
            : _json(401, {'error': 'unauthorized'});
      });
      final api = await client(adapter);

      await expectLater(
        api.listEntries(),
        throwsA(predicate((e) =>
            e.toString().contains('database unavailable') &&
            !e.toString().contains('Session expired'))),
      );
      expect(sessionExpired, isFalse);
    });

    test('a second 401 on the retry does not start another refresh', () async {
      final adapter = _FakeAdapter((r) => r.path.endsWith(refresh)
          ? _json(200, {'access_token': 'A2', 'refresh_token': 'RT2'})
          : _json(401, {'error': 'unauthorized'}));
      final api = await client(adapter);

      await expectLater(api.listEntries(), throwsException);
      expect(adapter.count(refresh), 1);
      expect(adapter.count(entries), 2);
    });
  });

  test('logout after a forced refresh revokes the rotated token', () async {
    final adapter = _FakeAdapter((r) {
      if (r.path.endsWith(refresh)) {
        return _json(200, {'access_token': 'A2', 'refresh_token': 'RT2'});
      }
      if (_bearer(r) != 'Bearer A2') return _json(401, {'error': 'expired'});
      return ResponseBody.fromString('', 204);
    });
    final api = await client(adapter);

    await api.logout();
    final logouts =
        adapter.requests.where((r) => r.path.endsWith('/auth/logout')).toList();
    expect(logouts, hasLength(2));
    expect(logouts.first.bearer, 'Bearer A1');
    expect((logouts.first.data as Map)['refresh_token'], 'RT1');
    expect(logouts.last.bearer, 'Bearer A2');
    expect((logouts.last.data as Map)['refresh_token'], 'RT2');
  });
}

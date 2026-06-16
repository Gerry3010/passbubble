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

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'models.dart';

const _kServerUrlKey = 'server_url';
const _kAccessTokenKey = 'access_token';
const _kRefreshTokenKey = 'refresh_token';

/// Riverpod provider for the ApiClient.
final apiClientProvider = Provider<ApiClient>((ref) => ApiClient());

class ApiClient {
  late final Dio _dio;
  late final FlutterSecureStorage _storage;

  String? _accessToken;
  String? _baseUrl;

  ApiClient() {
    _storage = const FlutterSecureStorage(
      aOptions: AndroidOptions(encryptedSharedPreferences: true),
    );
    _dio = Dio()
      ..interceptors.add(InterceptorsWrapper(
        onRequest: _onRequest,
        onError: _onError,
      ));
  }

  Future<void> init() async {
    _baseUrl = await _storage.read(key: _kServerUrlKey);
    _accessToken = await _storage.read(key: _kAccessTokenKey);
  }

  bool get isConfigured => _baseUrl != null && _baseUrl!.isNotEmpty;

  Future<void> setServerUrl(String url) async {
    _baseUrl = url.replaceAll(RegExp(r'/$'), '');
    await _storage.write(key: _kServerUrlKey, value: _baseUrl);
    _dio.options.baseUrl = _baseUrl!;
  }

  Future<void> setTokens(String access, String refresh) async {
    _accessToken = access;
    await _storage.write(key: _kAccessTokenKey, value: access);
    await _storage.write(key: _kRefreshTokenKey, value: refresh);
  }

  Future<void> clearTokens() async {
    _accessToken = null;
    await _storage.delete(key: _kAccessTokenKey);
    await _storage.delete(key: _kRefreshTokenKey);
  }

  Future<String?> getRefreshToken() => _storage.read(key: _kRefreshTokenKey);

  // ── Auth ──────────────────────────────────────────────────────────────────

  Future<LoginResponse> login(String email, String password) async {
    final resp = await _post('/api/v1/auth/login', {
      'email': email,
      'password': password,
    });
    return LoginResponse.fromJson(resp.data as Map<String, dynamic>);
  }

  Future<LoginResponse> register(RegisterRequest req) async {
    final resp = await _post('/api/v1/auth/register', req.toJson());
    return LoginResponse.fromJson(resp.data as Map<String, dynamic>);
  }

  Future<RefreshResponse> refresh(String refreshToken) async {
    final resp = await _post('/api/v1/auth/refresh', {
      'refresh_token': refreshToken,
    }, skipAuth: true);
    return RefreshResponse.fromJson(resp.data as Map<String, dynamic>);
  }

  Future<void> logout(String refreshToken) async {
    await _post('/api/v1/auth/logout', {'refresh_token': refreshToken});
  }

  Future<UserResponse> me() async {
    final resp = await _get('/api/v1/auth/me');
    return UserResponse.fromJson(resp.data as Map<String, dynamic>);
  }

  // ── Entries ───────────────────────────────────────────────────────────────

  Future<List<EntryResponse>> listEntries() async {
    final resp = await _get('/api/v1/entries');
    return (resp.data as List)
        .map((e) => EntryResponse.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<EntryResponse> getEntry(String id) async {
    final resp = await _get('/api/v1/entries/$id');
    return EntryResponse.fromJson(resp.data as Map<String, dynamic>);
  }

  Future<EntryResponse> createEntry(CreateEntryRequest req) async {
    final resp = await _post('/api/v1/entries', req.toJson());
    return EntryResponse.fromJson(resp.data as Map<String, dynamic>);
  }

  Future<EntryResponse> updateEntry(String id, UpdateEntryRequest req) async {
    final resp = await _put('/api/v1/entries/$id', req.toJson());
    return EntryResponse.fromJson(resp.data as Map<String, dynamic>);
  }

  Future<void> deleteEntry(String id) => _delete('/api/v1/entries/$id');

  Future<List<EntryResponse>> searchEntries(String query) async {
    final resp = await _get('/api/v1/entries/search?q=${Uri.encodeQueryComponent(query)}');
    return (resp.data as List)
        .map((e) => EntryResponse.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<void> shareEntry(String id, ShareEntryRequest req) =>
      _post('/api/v1/entries/$id/share', req.toJson());

  // ── Folders ───────────────────────────────────────────────────────────────

  Future<List<FolderResponse>> listFolders() async {
    final resp = await _get('/api/v1/folders');
    return (resp.data as List)
        .map((e) => FolderResponse.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  // ── Users ─────────────────────────────────────────────────────────────────

  Future<UserPublicKeys> getUserKeys(String userId) async {
    final resp = await _get('/api/v1/users/$userId/keys');
    return UserPublicKeys.fromJson(resp.data as Map<String, dynamic>);
  }

  Future<List<UserResponse>> searchUsers(String query) async {
    final resp = await _get('/api/v1/users/search?q=${Uri.encodeQueryComponent(query)}');
    return (resp.data as List)
        .map((e) => UserResponse.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  // ── Generate ──────────────────────────────────────────────────────────────

  Future<GenerateResponse> generate({
    int length = 20,
    String type = 'strong',
    int count = 1,
    bool noAmbiguous = false,
    String? excludeChars,
  }) async {
    final resp = await _post('/api/v1/generate', {
      'length': length,
      'type': type,
      'count': count,
      'no_ambiguous': noAmbiguous,
      'exclude_chars': ?excludeChars,
    });
    return GenerateResponse.fromJson(resp.data as Map<String, dynamic>);
  }

  // ── Admin ─────────────────────────────────────────────────────────────────

  Future<List<UserResponse>> adminListUsers() async {
    final resp = await _get('/api/v1/admin/users');
    return (resp.data as List)
        .map((e) => UserResponse.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<InvitationResponse> adminInvite(String email) async {
    final resp = await _post('/api/v1/admin/invite', {'email': email});
    return InvitationResponse.fromJson(resp.data as Map<String, dynamic>);
  }

  Future<List<InvitationResponse>> adminListInvitations() async {
    final resp = await _get('/api/v1/admin/invitations');
    return (resp.data as List)
        .map((e) => InvitationResponse.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  // ── HTTP helpers ──────────────────────────────────────────────────────────

  Future<Response<dynamic>> _get(String path) =>
      _dio.get(_url(path));

  Future<Response<dynamic>> _post(String path, Map<String, dynamic> data,
      {bool skipAuth = false}) =>
      _dio.post(_url(path), data: data);

  Future<Response<dynamic>> _put(String path, Map<String, dynamic> data) =>
      _dio.put(_url(path), data: data);

  Future<Response<dynamic>> _delete(String path) => _dio.delete(_url(path));

  String _url(String path) => '${_baseUrl ?? ''}$path';

  void _onRequest(RequestOptions options, RequestInterceptorHandler handler) {
    if (_accessToken != null) {
      options.headers['Authorization'] = 'Bearer $_accessToken';
    }
    handler.next(options);
  }

  Future<void> _onError(
    DioException err,
    ErrorInterceptorHandler handler,
  ) async {
    // Auto-refresh on 401
    if (err.response?.statusCode == 401) {
      final refreshToken = await getRefreshToken();
      if (refreshToken != null) {
        try {
          final resp = await refresh(refreshToken);
          await setTokens(resp.accessToken, resp.refreshToken);
          // Retry original request
          final opts = err.requestOptions;
          opts.headers['Authorization'] = 'Bearer $_accessToken';
          final retried = await _dio.fetch(opts);
          handler.resolve(retried);
          return;
        } catch (_) {
          await clearTokens();
        }
      }
    }
    handler.next(err);
  }
}

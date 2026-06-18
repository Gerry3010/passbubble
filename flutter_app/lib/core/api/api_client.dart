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
import 'package:flutter/foundation.dart';
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
  VoidCallback? _onSessionExpired;

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

  void setSessionExpiredCallback(VoidCallback cb) => _onSessionExpired = cb;

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

  // ── Health ────────────────────────────────────────────────────────────────

  Future<Map<String, dynamic>> health() async {
    final resp = await _dio.get(_url('/health'));
    return resp.data as Map<String, dynamic>;
  }

  String? get baseUrl => _baseUrl;

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

  /// Creates an entry. The backend only returns `{"id": "..."}` on create,
  /// not a full entry — so that's all we parse here.
  Future<String> createEntry(CreateEntryRequest req) async {
    final resp = await _post('/api/v1/entries', req.toJson());
    return (resp.data as Map<String, dynamic>)['id'] as String;
  }

  /// Updates an entry. The backend returns 204 No Content on success.
  Future<void> updateEntry(String id, UpdateEntryRequest req) async {
    await _put('/api/v1/entries/$id', req.toJson());
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

  // ── Jobs ──────────────────────────────────────────────────────────────────

  Future<List<JobResponse>> listJobs() async {
    final resp = await _get('/api/v1/jobs');
    return (resp.data as List)
        .map((e) => JobResponse.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  // ── Shares ────────────────────────────────────────────────────────────────

  Future<MySharesResponse> listMyShares() async {
    final resp = await _get('/api/v1/shares');
    return MySharesResponse.fromJson(resp.data as Map<String, dynamic>);
  }

  Future<void> revokeShareLink(String linkId) =>
      _delete('/api/v1/shares/links/$linkId');

  Future<void> revokeEntryShare(String entryId, String userId) =>
      _delete('/api/v1/entries/$entryId/share/$userId');

  Future<void> revokeFolderShare(String folderId, String userId) =>
      _delete('/api/v1/folders/$folderId/share/$userId');

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
      _dio.post(_url(path), data: data,
          options: skipAuth ? Options(extra: {'skipAuth': true}) : null);

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
    // Don't intercept requests that explicitly skip auth (e.g. the refresh
    // call itself) — prevents an infinite retry loop.
    if (err.requestOptions.extra['skipAuth'] == true) {
      handler.next(err);
      return;
    }

    if (err.response?.statusCode == 401) {
      final refreshToken = await getRefreshToken();
      if (refreshToken != null) {
        try {
          final resp = await refresh(refreshToken);
          await setTokens(resp.accessToken, resp.refreshToken);
          final opts = err.requestOptions;
          opts.headers['Authorization'] = 'Bearer $_accessToken';
          final retried = await _dio.fetch(opts);
          handler.resolve(retried);
          return;
        } catch (_) {
          // Refresh failed — session is gone. Clear everything and signal
          // the app to return to the login screen.
          await clearTokens();
          _onSessionExpired?.call();
        }
      }
    }
    handler.next(err);
  }
}

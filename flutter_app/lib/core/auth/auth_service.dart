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

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../crypto/vault_crypto.dart';

const _kUserIdKey = 'user_id';
const _kEmailKey = 'email';
const _kRoleKey = 'role';
const _kNameKey = 'name';
const _kKdfSaltKey = 'kdf_salt';
const _kKdfTimeKey = 'kdf_time';
const _kKdfMemoryKey = 'kdf_memory';
const _kEncPrivX25519Key = 'enc_priv_x25519';
const _kEncPrivMlkemKey = 'enc_priv_mlkem768';
const _kPubX25519Key = 'pub_x25519';
const _kPubMlkemKey = 'pub_mlkem768';

final authServiceProvider = Provider<AuthService>(
  (ref) => AuthService(ref.watch(apiClientProvider)),
);

final authStateProvider = StateNotifierProvider<AuthNotifier, AuthState>(
  (ref) => AuthNotifier(ref.watch(authServiceProvider)),
);

class AuthState {
  final bool isLoggedIn;
  final bool isUnlocked;
  final String? userId;
  final String? email;
  final String? name;
  final String? role;

  const AuthState({
    this.isLoggedIn = false,
    this.isUnlocked = false,
    this.userId,
    this.email,
    this.name,
    this.role,
  });

  bool get isAdmin => role == 'admin';

  AuthState copyWith({
    bool? isLoggedIn,
    bool? isUnlocked,
    String? userId,
    String? email,
    String? name,
    String? role,
  }) =>
      AuthState(
        isLoggedIn: isLoggedIn ?? this.isLoggedIn,
        isUnlocked: isUnlocked ?? this.isUnlocked,
        userId: userId ?? this.userId,
        email: email ?? this.email,
        name: name ?? this.name,
        role: role ?? this.role,
      );
}

class AuthNotifier extends StateNotifier<AuthState> {
  final AuthService _svc;
  AuthNotifier(this._svc) : super(const AuthState());

  Future<void> init() async {
    final (loggedIn, userId, email, name, role) = await _svc.loadSession();
    state = AuthState(
      isLoggedIn: loggedIn,
      userId: userId,
      email: email,
      name: name,
      role: role,
    );
  }

  Future<void> login(String email, String password) async {
    final session = await _svc.login(email, password);
    state = AuthState(
      isLoggedIn: true,
      isUnlocked: true,
      userId: session.userId,
      email: session.email,
      name: session.name,
      role: session.role,
    );
  }

  Future<void> register(String email, String name, String password,
      String invitationToken) async {
    final session =
        await _svc.register(email, name, password, invitationToken);
    state = AuthState(
      isLoggedIn: true,
      isUnlocked: true,
      userId: session.userId,
      email: session.email,
      name: session.name,
      role: session.role,
    );
  }

  Future<bool> unlock(String masterPassword) async {
    final ok = await _svc.unlock(masterPassword);
    if (ok) state = state.copyWith(isUnlocked: true);
    return ok;
  }

  Future<void> logout() async {
    await _svc.logout();
    state = const AuthState();
  }
}

typedef AuthSession = ({String userId, String email, String name, String role});

/// AuthService manages login/logout/unlock and persists key material securely.
class AuthService {
  final ApiClient _api;
  final _storage = const FlutterSecureStorage(
    aOptions: AndroidOptions(encryptedSharedPreferences: true),
  );

  // In-memory private keys (cleared on logout)
  Uint8List? _privX25519;

  // Exposed for vault operations
  Uint8List? get privX25519 => _privX25519;

  AuthService(this._api);

  Future<(bool, String?, String?, String?, String?)> loadSession() async {
    final userId = await _storage.read(key: _kUserIdKey);
    final email = await _storage.read(key: _kEmailKey);
    final name = await _storage.read(key: _kNameKey);
    final role = await _storage.read(key: _kRoleKey);
    return (userId != null, userId, email, name, role);
  }

  Future<AuthSession> login(String email, String password) async {
    final resp = await _api.login(email, password);
    await _api.setTokens(resp.accessToken, resp.refreshToken);
    await _persistKeyMaterial(resp);

    // Unlock private keys immediately
    await _deriveAndUnlock(password, resp);

    return (
      userId: resp.userId,
      email: resp.email,
      name: resp.name,
      role: resp.role,
    );
  }

  Future<AuthSession> register(
    String email,
    String name,
    String password,
    String invitationToken,
  ) async {
    // Generate key material client-side
    final keyPair = await VaultCrypto.generateX25519KeyPair();
    final pubBytes =
        Uint8List.fromList((await keyPair.extractPublicKey()).bytes);
    final privBytes = Uint8List.fromList(
      (await keyPair.extract()).bytes,
    );
    final salt = VaultCrypto.randomSalt();
    final masterKey = await VaultCrypto.deriveMasterKey(password, salt);
    final encPrivX25519 = await VaultCrypto.encrypt(masterKey, privBytes);

    // ML-KEM placeholder — same X25519 key used for now
    final encPrivMlkem = encPrivX25519;
    final pubMlkem = pubBytes;

    final req = RegisterRequest(
      email: email,
      name: name,
      password: password,
      invitationToken: invitationToken,
      pubX25519: base64.encode(pubBytes),
      pubMlkem768: base64.encode(pubMlkem),
      encPrivX25519: base64.encode(encPrivX25519),
      encPrivMlkem768: base64.encode(encPrivMlkem),
      kdfSalt: base64.encode(salt),
    );
    final resp = await _api.register(req);
    await _api.setTokens(resp.accessToken, resp.refreshToken);

    // Store the key material that came back (server echoes pub keys)
    await _storage.write(key: _kUserIdKey, value: resp.userId);
    await _storage.write(key: _kEmailKey, value: resp.email);
    await _storage.write(key: _kNameKey, value: resp.name);
    await _storage.write(key: _kRoleKey, value: resp.role);
    await _storage.write(key: _kKdfSaltKey, value: base64.encode(salt));
    await _storage.write(key: _kKdfTimeKey, value: '3');
    await _storage.write(key: _kKdfMemoryKey, value: '65536');
    await _storage.write(
        key: _kEncPrivX25519Key, value: base64.encode(encPrivX25519));
    await _storage.write(
        key: _kEncPrivMlkemKey, value: base64.encode(encPrivMlkem));
    await _storage.write(key: _kPubX25519Key, value: base64.encode(pubBytes));
    await _storage.write(key: _kPubMlkemKey, value: base64.encode(pubMlkem));

    _privX25519 = privBytes;

    return (
      userId: resp.userId,
      email: resp.email,
      name: resp.name,
      role: resp.role,
    );
  }

  Future<bool> unlock(String masterPassword) async {
    try {
      final saltB64 = await _storage.read(key: _kKdfSaltKey);
      final timeStr = await _storage.read(key: _kKdfTimeKey);
      final memStr = await _storage.read(key: _kKdfMemoryKey);
      final encPrivX25519B64 = await _storage.read(key: _kEncPrivX25519Key);
      if (saltB64 == null || encPrivX25519B64 == null) return false;

      final salt = base64.decode(saltB64);
      final time = int.tryParse(timeStr ?? '3') ?? 3;
      final memory = int.tryParse(memStr ?? '65536') ?? 65536;

      final masterKey = await VaultCrypto.deriveMasterKey(
        masterPassword,
        salt,
        memory: memory,
        iterations: time,
      );
      _privX25519 = await VaultCrypto.decrypt(
        masterKey,
        base64.decode(encPrivX25519B64),
      );
      return true;
    } catch (_) {
      return false;
    }
  }

  Future<void> logout() async {
    final refreshToken = await _api.getRefreshToken();
    if (refreshToken != null) {
      try {
        await _api.logout(refreshToken);
      } catch (_) {}
    }
    await _api.clearTokens();
    await _storage.deleteAll();
    _privX25519 = null;
  }

  Future<String?> getPubX25519() => _storage.read(key: _kPubX25519Key);
  Future<String?> getUserId() => _storage.read(key: _kUserIdKey);

  Future<void> _persistKeyMaterial(LoginResponse resp) async {
    await _storage.write(key: _kUserIdKey, value: resp.userId);
    await _storage.write(key: _kEmailKey, value: resp.email);
    await _storage.write(key: _kNameKey, value: resp.name);
    await _storage.write(key: _kRoleKey, value: resp.role);
    await _storage.write(key: _kKdfSaltKey, value: resp.kdfSalt);
    await _storage.write(key: _kKdfTimeKey, value: resp.kdfTime.toString());
    await _storage.write(key: _kKdfMemoryKey, value: resp.kdfMemory.toString());
    await _storage.write(key: _kEncPrivX25519Key, value: resp.encPrivX25519);
    await _storage.write(key: _kEncPrivMlkemKey, value: resp.encPrivMlkem768);
    await _storage.write(key: _kPubX25519Key, value: resp.pubX25519);
    await _storage.write(key: _kPubMlkemKey, value: resp.pubMlkem768);
  }

  Future<void> _deriveAndUnlock(String password, LoginResponse resp) async {
    if (resp.kdfSalt.isEmpty || resp.encPrivX25519.isEmpty) return;
    final salt = base64.decode(resp.kdfSalt);
    final masterKey = await VaultCrypto.deriveMasterKey(
      password,
      salt,
      memory: resp.kdfMemory,
      iterations: resp.kdfTime,
    );
    _privX25519 = await VaultCrypto.decrypt(
      masterKey,
      base64.decode(resp.encPrivX25519),
    );
  }
}

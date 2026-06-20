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

import 'package:cryptography/cryptography.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../api/api_client.dart';
import '../api/models.dart';
import '../crypto/vault_crypto.dart';
import '../crypto/ml_kem.dart';
import '../crypto/pin_crypto.dart';

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

// PIN quick-unlock (device-local; the PIN itself is never stored).
const _kBioMasterKey = 'biometric_master';

const _kPinEnabledKey = 'pin_enabled';
const _kPinSaltKey = 'pin_salt';
const _kPinWrappedKey = 'pin_wrapped_master_key';
const _kPinMaxTriesKey = 'pin_max_tries';
const _kPinFailCountKey = 'pin_fail_count';
const _kPinIntervalDaysKey = 'pin_pw_interval_days';
const _kPinLastMasterUnlockKey = 'pin_last_master_unlock';

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

  AuthService get svc => _svc;

  Future<void> init() async {
    // When the API layer detects a terminal 401 (refresh rejected), clear
    // everything locally and drop back to the login screen.
    _svc.api.setSessionExpiredCallback(() async {
      await _svc.clearLocalSession();
      state = const AuthState();
    });

    final (loggedIn, userId, email, name, role) = await _svc.loadSession();
    state = AuthState(
      isLoggedIn: loggedIn,
      userId: userId,
      email: email,
      name: name,
      role: role,
    );
  }

  /// Performs the password step. Returns a non-empty pending token when the
  /// account requires a 2FA code (caller should then prompt and call
  /// [verifyTotp]); returns null when login completed directly.
  Future<String?> login(String email, String password) async {
    final result = await _svc.login(email, password);
    if (result.session == null) return result.pendingToken;
    _applySession(result.session!);
    return null;
  }

  /// Completes a 2FA login with the user's code.
  Future<void> verifyTotp(
      String pendingToken, String code, String password) async {
    final session = await _svc.completeTotp(pendingToken, code, password);
    _applySession(session);
  }

  /// Requests a 2FA recovery email for an account stuck at the TOTP step.
  Future<void> requestTotpRecovery(String pendingToken) =>
      _svc.requestTotpRecovery(pendingToken);

  void _applySession(AuthSession session) {
    state = AuthState(
      isLoggedIn: true,
      isUnlocked: true,
      userId: session.userId,
      email: session.email,
      name: session.name,
      role: session.role,
    );
  }

  /// Returns null when logged in directly, or a pending-verification message
  /// that the UI should display before redirecting to the login screen.
  Future<String?> register(String email, String name, String password,
      String invitationToken) async {
    final pending =
        await _svc.register(email, name, password, invitationToken);
    if (pending != null) return pending;

    // SMTP disabled — session is live, load profile to populate state.
    final (_, userId, userEmail, userName, role) = await _svc.loadSession();
    state = AuthState(
      isLoggedIn: true,
      isUnlocked: true,
      userId: userId,
      email: userEmail,
      name: userName,
      role: role,
    );
    return null;
  }

  Future<bool> unlock(String masterPassword) async {
    final ok = await _svc.unlock(masterPassword);
    if (ok) state = state.copyWith(isUnlocked: true);
    return ok;
  }

  /// Unlocks via PIN. On success the vault is unlocked; otherwise the caller
  /// inspects the result (wrong PIN / expired / locked out) to update the UI.
  Future<PinUnlockResult> unlockWithPin(String pin) async {
    final result = await _svc.unlockWithPin(pin);
    if (result.status == PinUnlockStatus.ok) {
      state = state.copyWith(isUnlocked: true);
    }
    return result;
  }

  Future<PinStatus> pinStatus() => _svc.pinStatus();

  /// Enables PIN quick-unlock (requires the master password). Returns false when
  /// the master password is wrong.
  Future<bool> enablePin(String masterPassword, String pin, int intervalDays) =>
      _svc.enablePin(masterPassword, pin, intervalDays: intervalDays);

  Future<void> disablePin() => _svc.disablePin();

  /// Locks the vault: clears the in-memory private keys but keeps the session.
  /// The router redirects to the unlock screen; the master password re-derives
  /// the keys from the still-stored encrypted material.
  void lock() {
    if (!state.isLoggedIn || !state.isUnlocked) return;
    _svc.lock();
    state = state.copyWith(isUnlocked: false);
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
  Uint8List? _privMLKEM;

  // Exposed for vault operations
  Uint8List? get privX25519 => _privX25519;
  Uint8List? get privMLKEM => _privMLKEM;

  AuthService(this._api);

  ApiClient get api => _api;

  Future<void> clearLocalSession() async {
    _privX25519 = null;
    _privMLKEM = null;
    await _api.clearTokens();
    await _storage.deleteAll();
  }

  Future<(bool, String?, String?, String?, String?)> loadSession() async {
    final userId = await _storage.read(key: _kUserIdKey);
    final email = await _storage.read(key: _kEmailKey);
    final name = await _storage.read(key: _kNameKey);
    final role = await _storage.read(key: _kRoleKey);
    return (userId != null, userId, email, name, role);
  }

  /// Performs the password step. When the account has 2FA enabled the returned
  /// record has a null [session] and a non-empty [pendingToken]; the caller must
  /// then call [completeTotp] with the user's code. Otherwise the session is
  /// fully established (tokens persisted, vault unlocked).
  Future<({AuthSession? session, String pendingToken})> login(
      String email, String password) async {
    final resp = await _api.login(email, password);
    if (resp.requiresTotp) {
      return (session: null, pendingToken: resp.pendingToken);
    }
    return (session: await _establishSession(password, resp), pendingToken: '');
  }

  /// Completes a 2FA login: verifies the code, then establishes the session.
  /// [password] is needed to re-derive the master key for the local unlock.
  Future<AuthSession> completeTotp(
      String pendingToken, String code, String password) async {
    final resp = await _api.verifyTotp(pendingToken, code);
    return _establishSession(password, resp);
  }

  /// Requests a 2FA recovery email for an account stuck at the TOTP step.
  Future<void> requestTotpRecovery(String pendingToken) =>
      _api.requestTotpRecovery(pendingToken);

  Future<AuthSession> _establishSession(
      String password, LoginResponse resp) async {
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

  /// Registers a new account. Returns null when the user is immediately
  /// logged in (SMTP disabled), or a pending-verification message when the
  /// server requires email confirmation before the account is active.
  Future<String?> register(
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

    // Real ML-KEM-768 keypair → post-quantum hybrid encryption from the start.
    final (privMlkemBytes, pubMlkem) = await mlKemGenerate();
    final encPrivMlkem = await VaultCrypto.encrypt(masterKey, privMlkemBytes);

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
    final result = await _api.register(req);

    // Server requires email verification — no tokens yet.
    if (result.session == null) return result.pendingMessage;

    final resp = result.session!;
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
    _privMLKEM = privMlkemBytes;

    return null;
  }

  Future<bool> unlock(String masterPassword) async {
    try {
      final saltB64 = await _storage.read(key: _kKdfSaltKey);
      final timeStr = await _storage.read(key: _kKdfTimeKey);
      final memStr = await _storage.read(key: _kKdfMemoryKey);
      final encPrivX25519B64 = await _storage.read(key: _kEncPrivX25519Key);
      final encPrivMlkemB64 = await _storage.read(key: _kEncPrivMlkemKey);
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
      if (encPrivMlkemB64 != null) {
        _privMLKEM = await VaultCrypto.decrypt(masterKey, base64.decode(encPrivMlkemB64));
      }
      // A successful master-password unlock restarts the PIN re-auth interval.
      await _onMasterUnlock();
      return true;
    } catch (_) {
      return false;
    }
  }

  /// Clears the in-memory private keys without touching the persisted session
  /// or tokens, so the vault can be re-unlocked with the master password.
  void lock() {
    _privX25519 = null;
    _privMLKEM = null;
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
    _privMLKEM = null;
  }

  Future<String?> getPubX25519() => _storage.read(key: _kPubX25519Key);
  Future<String?> getPubMlkem768() => _storage.read(key: _kPubMlkemKey);
  Future<String?> getUserId() => _storage.read(key: _kUserIdKey);

  // ── Biometric quick-unlock ─────────────────────────────────────────────────

  /// Caches the master password so biometric auth can retrieve and use it.
  Future<void> saveBiometricMasterPassword(String password) =>
      _storage.write(key: _kBioMasterKey, value: password);

  /// Returns the cached master password if biometric unlock was previously set up.
  Future<String?> loadBiometricMasterPassword() =>
      _storage.read(key: _kBioMasterKey);

  Future<bool> hasBiometricMasterPassword() async =>
      (await _storage.read(key: _kBioMasterKey)) != null;

  Future<void> disableBiometric() => _storage.delete(key: _kBioMasterKey);

  // ── Post-quantum key upgrade ────────────────────────────────────────────────

  /// ML-KEM-768 encapsulation (public) key size. X25519-only accounts (created by
  /// older app versions) store a 32-byte placeholder, so a mismatch means the
  /// account has no real post-quantum key yet.
  static const int _mlkem768PubLen = 1184;

  /// True when [pubMlkem768Base64] is a placeholder rather than a real ML-KEM key.
  static bool isPlaceholderMlkemKey(String? pubMlkem768Base64) {
    if (pubMlkem768Base64 == null || pubMlkem768Base64.isEmpty) return true;
    try {
      return base64.decode(pubMlkem768Base64).length != _mlkem768PubLen;
    } catch (_) {
      return true;
    }
  }

  /// Whether this account still needs the post-quantum (hybrid) upgrade.
  Future<bool> needsKeyUpgrade() async =>
      isPlaceholderMlkemKey(await _storage.read(key: _kPubMlkemKey));

  /// Retrofits a real ML-KEM-768 keypair onto an X25519-only account and re-wraps
  /// every owned entry's data key to hybrid. The X25519 keypair is kept (so
  /// entries shared to us stay readable); entries that fail to re-wrap remain
  /// classical and readable, so this is safe to re-run. Returns the counts.
  Future<({int rewrapped, int failed})> upgradeToHybrid(String masterPassword) async {
    final saltB64 = await _storage.read(key: _kKdfSaltKey);
    final timeStr = await _storage.read(key: _kKdfTimeKey);
    final memStr = await _storage.read(key: _kKdfMemoryKey);
    final encPrivX25519B64 = await _storage.read(key: _kEncPrivX25519Key);
    final pubXB64 = await _storage.read(key: _kPubX25519Key);
    final userId = await _storage.read(key: _kUserIdKey);
    if (saltB64 == null || encPrivX25519B64 == null || pubXB64 == null || userId == null) {
      throw Exception('Missing key material — please log in again');
    }

    final masterKey = await VaultCrypto.deriveMasterKey(
      masterPassword,
      base64.decode(saltB64),
      memory: int.tryParse(memStr ?? '65536') ?? 65536,
      iterations: int.tryParse(timeStr ?? '3') ?? 3,
    );

    // Verify the master password by decrypting the existing X25519 private key.
    final Uint8List oldPrivX;
    try {
      oldPrivX = await VaultCrypto.decrypt(masterKey, base64.decode(encPrivX25519B64));
    } catch (_) {
      throw Exception('Wrong master password');
    }

    // Generate the real ML-KEM keypair and persist it (X25519 unchanged).
    final (newPrivM, newPubM) = await mlKemGenerate();
    final encNewPrivM = await VaultCrypto.encrypt(masterKey, newPrivM);
    final newPubMB64 = base64.encode(newPubM);
    final encNewPrivMB64 = base64.encode(encNewPrivM);

    await _api.updateKeys(UpdateKeysRequest(
      pubX25519: pubXB64,
      pubMlkem768: newPubMB64,
      encPrivX25519: encPrivX25519B64,
      encPrivMlkem768: encNewPrivMB64,
    ));
    await _storage.write(key: _kPubMlkemKey, value: newPubMB64);
    await _storage.write(key: _kEncPrivMlkemKey, value: encNewPrivMB64);
    _privX25519 = oldPrivX;
    _privMLKEM = newPrivM;

    // Re-wrap each owned entry: decrypt with old keys (legacy needs only X25519),
    // re-encrypt to (X25519, new ML-KEM).
    var rewrapped = 0;
    var failed = 0;
    final entries = await _api.listEntries();
    for (final e in entries) {
      try {
        final full = await _api.getEntry(e.id);
        final ek = full.entryKey;
        if (ek == null) {
          failed++;
          continue;
        }
        final dataKey = await VaultCrypto.decryptDataKey(ek.encryptedKey, oldPrivX, newPrivM);
        final newEnc = await VaultCrypto.encryptDataKey(
            Uint8List.fromList(dataKey), pubXB64, newPubMB64);
        await _api.updateEntry(
          e.id,
          UpdateEntryRequest(
            folderId: full.folderId,
            entryKeys: [EntryKey(userId: userId, encryptedKey: newEnc)],
          ),
        );
        rewrapped++;
      } catch (_) {
        failed++;
      }
    }
    return (rewrapped: rewrapped, failed: failed);
  }

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

  // ── PIN quick-unlock ───────────────────────────────────────────────────────

  Future<bool> isPinEnabled() async =>
      (await _storage.read(key: _kPinEnabledKey)) == '1';

  /// Returns the current PIN configuration for the settings/unlock UI.
  Future<PinStatus> pinStatus() async {
    if (!await isPinEnabled()) return const PinStatus(enabled: false);
    final intervalDays = int.tryParse(
            await _storage.read(key: _kPinIntervalDaysKey) ?? '') ??
        PinCrypto.defaultIntervalDays;
    final maxTries =
        int.tryParse(await _storage.read(key: _kPinMaxTriesKey) ?? '') ??
            PinCrypto.defaultMaxTries;
    final failCount =
        int.tryParse(await _storage.read(key: _kPinFailCountKey) ?? '') ?? 0;
    return PinStatus(
      enabled: true,
      expired: await _pinExpired(intervalDays),
      intervalDays: intervalDays,
      triesRemaining: (maxTries - failCount).clamp(0, maxTries),
    );
  }

  Future<bool> _pinExpired(int intervalDays) async {
    final lastStr = await _storage.read(key: _kPinLastMasterUnlockKey);
    final last = int.tryParse(lastStr ?? '');
    if (last == null) return true;
    final deadline = last + PinCrypto.clampIntervalDays(intervalDays) * 86400000;
    return DateTime.now().millisecondsSinceEpoch >= deadline;
  }

  /// Resets the PIN re-auth interval + failure counter after a master-password
  /// unlock. No-op when no PIN is configured.
  Future<void> _onMasterUnlock() async {
    if (!await isPinEnabled()) return;
    await _storage.write(
      key: _kPinLastMasterUnlockKey,
      value: DateTime.now().millisecondsSinceEpoch.toString(),
    );
    await _storage.write(key: _kPinFailCountKey, value: '0');
  }

  /// Enables PIN quick-unlock. Requires the master password (re-derives the
  /// master key and authorizes the change). [intervalDays] is clamped to 1..60.
  /// Returns false if the master password is wrong.
  Future<bool> enablePin(
    String masterPassword,
    String pin, {
    int intervalDays = PinCrypto.defaultIntervalDays,
  }) async {
    final saltB64 = await _storage.read(key: _kKdfSaltKey);
    final encPrivX25519B64 = await _storage.read(key: _kEncPrivX25519Key);
    if (saltB64 == null || encPrivX25519B64 == null) return false;

    final salt = base64.decode(saltB64);
    final time = int.tryParse(await _storage.read(key: _kKdfTimeKey) ?? '3') ?? 3;
    final memory =
        int.tryParse(await _storage.read(key: _kKdfMemoryKey) ?? '65536') ?? 65536;

    final masterKey = await VaultCrypto.deriveMasterKey(
      masterPassword,
      salt,
      memory: memory,
      iterations: time,
    );
    // Verify the master password before trusting the derived key.
    try {
      await VaultCrypto.decrypt(masterKey, base64.decode(encPrivX25519B64));
    } catch (_) {
      return false;
    }

    final masterKeyBytes = Uint8List.fromList(await masterKey.extractBytes());
    final pinSalt = VaultCrypto.randomSalt(PinCrypto.saltLen);
    final wrapped = await PinCrypto.wrapMasterKey(masterKeyBytes, pin, pinSalt);

    await _storage.write(key: _kPinEnabledKey, value: '1');
    await _storage.write(key: _kPinSaltKey, value: base64.encode(pinSalt));
    await _storage.write(key: _kPinWrappedKey, value: base64.encode(wrapped));
    await _storage.write(
        key: _kPinMaxTriesKey, value: PinCrypto.defaultMaxTries.toString());
    await _storage.write(key: _kPinFailCountKey, value: '0');
    await _storage.write(
      key: _kPinIntervalDaysKey,
      value: PinCrypto.clampIntervalDays(intervalDays).toString(),
    );
    await _storage.write(
      key: _kPinLastMasterUnlockKey,
      value: DateTime.now().millisecondsSinceEpoch.toString(),
    );
    return true;
  }

  /// Removes all PIN quick-unlock state.
  Future<void> disablePin() async {
    await _storage.delete(key: _kPinEnabledKey);
    await _storage.delete(key: _kPinSaltKey);
    await _storage.delete(key: _kPinWrappedKey);
    await _storage.delete(key: _kPinMaxTriesKey);
    await _storage.delete(key: _kPinFailCountKey);
    await _storage.delete(key: _kPinIntervalDaysKey);
    await _storage.delete(key: _kPinLastMasterUnlockKey);
  }

  /// Unlocks the vault using the PIN. On the Nth wrong attempt the PIN is wiped
  /// (PinUnlockStatus.lockedOut). If the re-auth interval elapsed it returns
  /// PinUnlockStatus.expired without consuming an attempt.
  Future<PinUnlockResult> unlockWithPin(String pin) async {
    if (!await isPinEnabled()) {
      return const PinUnlockResult(PinUnlockStatus.notEnabled);
    }
    final intervalDays = int.tryParse(
            await _storage.read(key: _kPinIntervalDaysKey) ?? '') ??
        PinCrypto.defaultIntervalDays;
    if (await _pinExpired(intervalDays)) {
      return const PinUnlockResult(PinUnlockStatus.expired);
    }

    final maxTries =
        int.tryParse(await _storage.read(key: _kPinMaxTriesKey) ?? '') ??
            PinCrypto.defaultMaxTries;
    final failCount =
        int.tryParse(await _storage.read(key: _kPinFailCountKey) ?? '') ?? 0;

    // Persist the incremented counter BEFORE the attempt so killing the app
    // mid-attempt cannot reset it and bypass the lockout.
    final newCount = failCount + 1;
    await _storage.write(key: _kPinFailCountKey, value: newCount.toString());

    final saltB64 = await _storage.read(key: _kPinSaltKey);
    final wrappedB64 = await _storage.read(key: _kPinWrappedKey);
    final encPrivX25519B64 = await _storage.read(key: _kEncPrivX25519Key);
    final encPrivMlkemB64 = await _storage.read(key: _kEncPrivMlkemKey);
    if (saltB64 == null || wrappedB64 == null || encPrivX25519B64 == null) {
      return const PinUnlockResult(PinUnlockStatus.notEnabled);
    }

    try {
      final masterKeyBytes = await PinCrypto.unwrapMasterKey(
        base64.decode(wrappedB64),
        pin,
        base64.decode(saltB64),
      );
      final masterKey = SecretKey(masterKeyBytes);
      _privX25519 = await VaultCrypto.decrypt(
        masterKey,
        base64.decode(encPrivX25519B64),
      );
      if (encPrivMlkemB64 != null) {
        _privMLKEM = await VaultCrypto.decrypt(masterKey, base64.decode(encPrivMlkemB64));
      }
    } catch (_) {
      if (newCount >= maxTries) {
        await disablePin();
        return const PinUnlockResult(PinUnlockStatus.lockedOut);
      }
      return PinUnlockResult(
        PinUnlockStatus.wrongPin,
        triesRemaining: maxTries - newCount,
      );
    }

    // Success: reset the failure counter (the interval is NOT reset — only a
    // master-password unlock restarts it).
    await _storage.write(key: _kPinFailCountKey, value: '0');
    return const PinUnlockResult(PinUnlockStatus.ok);
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
    if (resp.encPrivMlkem768.isNotEmpty) {
      _privMLKEM = await VaultCrypto.decrypt(masterKey, base64.decode(resp.encPrivMlkem768));
    }
  }
}

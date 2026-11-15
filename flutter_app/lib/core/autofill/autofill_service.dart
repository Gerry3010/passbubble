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

import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Bridge to the native autofill integrations: the Android Autofill Framework
/// (`MainActivity` + `PassbubbleAutofillService`) and the iOS/macOS AutoFill
/// credential-provider extensions (see docs/passkeys.md for the channel
/// contract). Each method is a no-op where its platform is absent and never
/// throws — failures degrade to "not supported / not enabled".
class AutofillBridge {
  const AutofillBridge();

  static const MethodChannel _ch =
      MethodChannel('net.geraldhofbauer.passbubble/autofill');

  bool get _platformOk => !kIsWeb && Platform.isAndroid;
  bool get _appleOk => !kIsWeb && (Platform.isIOS || Platform.isMacOS);

  /// Whether this is a native iOS build (where the credential-provider
  /// extension consumes the cached credentials).
  bool get isIos => !kIsWeb && Platform.isIOS;

  /// Whether this is a native iOS or macOS build. Both answer the same channel
  /// methods (`iosSyncCredentials`, `syncPasskeys`, …) for their AutoFill
  /// extension.
  bool get isApple => _appleOk;

  /// Whether the OS exposes an autofill framework Passbubble can plug into.
  Future<bool> isSupported() async {
    if (!_platformOk) return false;
    try {
      return await _ch.invokeMethod<bool>('isSupported') ?? false;
    } catch (_) {
      return false;
    }
  }

  /// Whether Passbubble is the *currently selected* system autofill service.
  Future<bool> isEnabled() async {
    if (!_platformOk) return false;
    try {
      return await _ch.invokeMethod<bool>('isEnabled') ?? false;
    } catch (_) {
      return false;
    }
  }

  /// Opens the system autofill-service picker pre-targeted at Passbubble.
  /// Returns false if the picker couldn't be launched.
  Future<bool> requestEnable() async {
    if (!_platformOk) return false;
    try {
      return await _ch.invokeMethod<bool>('requestEnable') ?? false;
    } catch (_) {
      return false;
    }
  }

  /// Hands the decrypted, ready-to-fill credentials (a JSON array string) to the
  /// native service, stored at-rest encrypted. Called on unlock.
  Future<void> updateCredentials(String credentialsJson) async {
    if (!_platformOk) return;
    try {
      await _ch.invokeMethod<void>('updateCredentials', {
        'credentials': credentialsJson,
      });
    } catch (_) {
      // Best-effort: autofill simply won't have fresh data.
    }
  }

  /// Wipes the native bridge — called on vault lock / logout. Clears both the
  /// Android service cache and the iOS App Group (defaults + shared keychain).
  Future<void> clearVault() async {
    if (_platformOk) {
      try {
        await _ch.invokeMethod<void>('clearVault');
      } catch (_) {}
    }
    if (_appleOk) {
      try {
        await _ch.invokeMethod<void>('iosClearVault');
      } catch (_) {}
    }
  }

  /// iOS and macOS: hands the already-decrypted credentials (a JSON array
  /// string) to the AutoFill Credential Provider extension, stored in the
  /// shared keychain. The app owns the hybrid-KEM crypto, so the extension does
  /// none. No-op on other platforms; never throws.
  Future<void> syncIosCredentials(String credentialsJson) async {
    if (!_appleOk) return;
    try {
      await _ch.invokeMethod<void>('iosSyncCredentials', {
        'credentials': credentialsJson,
      });
    } catch (_) {
      // Best-effort: autofill simply won't have fresh data.
    }
  }

  /// iOS and macOS: replaces the extension's `autofill_passkeys` with
  /// [passkeysJson], a JSON array of every passkey record in the vault plus
  /// `entry_id` and `entry_name`. Sent as `{'passkeys': json}`, like
  /// `iosSyncCredentials`. No-op elsewhere; never throws.
  Future<void> syncPasskeys(String passkeysJson) async {
    if (!_appleOk) return;
    try {
      await _ch.invokeMethod<void>('syncPasskeys', {'passkeys': passkeysJson});
    } catch (_) {
      // Best-effort, like the credentials.
    }
  }

  /// iOS and macOS: the passkeys the AutoFill extension created that are not in
  /// the vault yet, as a JSON array string. `[]` elsewhere and on any error.
  Future<String> readPendingPasskeys() async {
    if (!_appleOk) return '[]';
    try {
      return await _ch.invokeMethod<String>('readPendingPasskeys') ?? '[]';
    } catch (_) {
      return '[]';
    }
  }

  /// iOS and macOS: tells the extension that the passkeys with these
  /// `credential_id`s (exactly as it wrote them) are in the vault now, so it
  /// drops them from `pending_passkeys`. Sent as `{'ids': [...]}`. No-op
  /// elsewhere; never throws.
  Future<void> clearPendingPasskeys(List<String> ids) async {
    if (!_appleOk || ids.isEmpty) return;
    try {
      await _ch.invokeMethod<void>('clearPendingPasskeys', {'ids': ids});
    } catch (_) {
      // They stay pending and are saved again (in place) after the next unlock.
    }
  }

  /// Registers [onReceived] for the native → Dart call
  /// `credentialExchangeReceived(json)`: CXF data from Apple Passwords. The
  /// argument is the JSON string (a map with `json` is accepted too). Null
  /// removes the handler.
  void setCredentialExchangeHandler(void Function(String json)? onReceived) {
    if (onReceived == null) {
      _ch.setMethodCallHandler(null);
      return;
    }
    _ch.setMethodCallHandler((call) => handleNativeCall(call, onReceived));
  }

  /// Answers one native → Dart [call]; see [setCredentialExchangeHandler].
  @visibleForTesting
  static Future<Object?> handleNativeCall(
      MethodCall call, void Function(String json) onCredentialExchange) async {
    if (call.method != 'credentialExchangeReceived') {
      throw MissingPluginException('No handler for ${call.method}');
    }
    final arg = call.arguments;
    final json = switch (arg) {
      final String s => s,
      final Map m => m['json'] as String?,
      _ => null,
    };
    if (json == null || json.trim().isEmpty) {
      throw PlatformException(code: 'bad-args', message: 'Expected the CXF JSON as a string');
    }
    onCredentialExchange(json);
    return true;
  }
}

final autofillBridgeProvider =
    Provider<AutofillBridge>((ref) => const AutofillBridge());

/// True when the OS supports autofill at all (Android 8+). Cached for the
/// session; UI hides the autofill affordances entirely when false.
final autofillSupportedProvider = FutureProvider<bool>(
  (ref) => ref.read(autofillBridgeProvider).isSupported(),
);

/// Whether Passbubble is the active autofill provider. Refreshable via
/// `ref.invalidate(autofillEnabledProvider)` (e.g. after the user returns from
/// the system picker on app resume).
final autofillEnabledProvider = FutureProvider<bool>(
  (ref) => ref.read(autofillBridgeProvider).isEnabled(),
);

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

/// Bridge to the native Android Autofill Framework integration
/// (`MainActivity` + `PassbubbleAutofillService`). All methods are no-ops on
/// platforms without the system autofill service (iOS/web/desktop) and never
/// throw — failures degrade to "not supported / not enabled".
class AutofillBridge {
  const AutofillBridge();

  static const MethodChannel _ch =
      MethodChannel('de.gerry3010.passbubble/autofill');

  bool get _platformOk => !kIsWeb && Platform.isAndroid;

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

  /// Wipes the native bridge — called on vault lock / logout.
  Future<void> clearVault() async {
    if (!_platformOk) return;
    try {
      await _ch.invokeMethod<void>('clearVault');
    } catch (_) {}
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

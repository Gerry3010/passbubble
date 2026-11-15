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

import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../auth/auth_service.dart';
import '../importexport/credential_exchange.dart';
import '../router/router.dart';
import 'autofill_service.dart';

/// Receives Credential Exchange data from the native side (see
/// [AutofillBridge.setCredentialExchangeHandler]) and opens the import screen
/// with it once the vault is open. While the vault is locked or nobody is
/// logged in, the data waits in [pendingCredentialExchangeProvider] and the
/// screen opens after the next unlock. Each hand-over opens it once; a logout
/// drops data still waiting.
class CredentialExchangeScope extends ConsumerStatefulWidget {
  final Widget child;
  const CredentialExchangeScope({super.key, required this.child});

  @override
  ConsumerState<CredentialExchangeScope> createState() => _CredentialExchangeScopeState();
}

class _CredentialExchangeScopeState extends ConsumerState<CredentialExchangeScope> {
  late final AutofillBridge _bridge;

  /// The hand-over the import screen was opened for.
  String? _openedFor;

  @override
  void initState() {
    super.initState();
    _bridge = ref.read(autofillBridgeProvider);
    _bridge.setCredentialExchangeHandler(_received);
  }

  @override
  void dispose() {
    _bridge.setCredentialExchangeHandler(null);
    super.dispose();
  }

  void _received(String json) {
    ref.read(pendingCredentialExchangeProvider.notifier).state = json;
    _openedFor = null;
    _openWhenReady();
  }

  void _openWhenReady() {
    final auth = ref.read(authStateProvider);
    final pending = ref.read(pendingCredentialExchangeProvider);
    if (!auth.isLoggedIn || !auth.isUnlocked || pending == null) return;
    if (identical(pending, _openedFor)) return;
    _openedFor = pending;
    // After the frame, so it lands after the unlock redirect.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || ref.read(pendingCredentialExchangeProvider) == null) return;
      ref.read(routerProvider).go('/manage?tab=0');
    });
    WidgetsBinding.instance.scheduleFrame();
  }

  @override
  Widget build(BuildContext context) {
    ref.listen<AuthState>(authStateProvider, (prev, next) {
      if (prev != null && prev.isLoggedIn && !next.isLoggedIn) {
        ref.read(pendingCredentialExchangeProvider.notifier).state = null;
        _openedFor = null;
      }
    });
    // Bumped on every login and unlock.
    ref.listen<int>(vaultSessionProvider, (_, _) => _openWhenReady());
    return widget.child;
  }
}

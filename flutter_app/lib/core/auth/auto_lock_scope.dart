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

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'auth_service.dart';
import 'auto_lock_provider.dart';

/// Wraps the app and locks the vault after a configurable period of user
/// inactivity. Any pointer interaction re-arms the idle timer. The timer only
/// runs while the vault is logged in and unlocked, and is disabled when the
/// configured interval is 0 ("off").
class AutoLockScope extends ConsumerStatefulWidget {
  final Widget child;
  const AutoLockScope({super.key, required this.child});

  @override
  ConsumerState<AutoLockScope> createState() => _AutoLockScopeState();
}

class _AutoLockScopeState extends ConsumerState<AutoLockScope> {
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _rearm());
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  void _rearm() {
    _timer?.cancel();
    final minutes = ref.read(autoLockProvider);
    final auth = ref.read(authStateProvider);
    if (minutes <= 0 || !auth.isLoggedIn || !auth.isUnlocked) return;
    _timer = Timer(Duration(minutes: minutes), _lock);
  }

  void _lock() {
    ref.read(authStateProvider.notifier).lock();
  }

  @override
  Widget build(BuildContext context) {
    // Re-arm whenever the session state or the configured interval changes.
    ref.listen(authStateProvider, (_, _) => _rearm());
    ref.listen(autoLockProvider, (_, _) => _rearm());

    return Listener(
      behavior: HitTestBehavior.translucent,
      onPointerDown: (_) => _rearm(),
      onPointerMove: (_) => _rearm(),
      onPointerSignal: (_) => _rearm(),
      child: widget.child,
    );
  }
}

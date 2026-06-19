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

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'core/api/api_client.dart';
import 'core/auth/auth_service.dart';
import 'core/auth/auto_lock_scope.dart';
import 'core/router/router.dart';
import 'core/theme/app_theme.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const ProviderScope(child: PassbubbleApp()));
}

class PassbubbleApp extends ConsumerStatefulWidget {
  const PassbubbleApp({super.key});

  @override
  ConsumerState<PassbubbleApp> createState() => _PassbubbleAppState();
}

class _PassbubbleAppState extends ConsumerState<PassbubbleApp> {
  bool _initialized = false;

  @override
  void initState() {
    super.initState();
    _init();
  }

  Future<void> _init() async {
    await ref.read(apiClientProvider).init();
    await ref.read(authStateProvider.notifier).init();
    if (mounted) setState(() => _initialized = true);
  }

  @override
  Widget build(BuildContext context) {
    if (!_initialized) {
      return MaterialApp(
        theme: AppTheme.dark,
        home: const Scaffold(
          body: Center(
            child: CircularProgressIndicator(color: AppTheme.green),
          ),
        ),
      );
    }

    final router = ref.watch(routerProvider);
    return AutoLockScope(
      child: MaterialApp.router(
        title: 'Passbubble',
        theme: AppTheme.dark,
        routerConfig: router,
        debugShowCheckedModeBanner: false,
      ),
    );
  }
}

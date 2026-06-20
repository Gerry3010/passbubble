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
import 'core/jobs/job_messenger.dart';
import 'core/router/router.dart';
import 'core/theme/app_theme.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  // Snapshot the launch URL before any MaterialApp can rewrite the browser
  // fragment — otherwise a `/share/...` deep link is lost.
  captureLaunchUri();

  // Initialise *before* the first MaterialApp is mounted. Previously a plain
  // (non-router) splash MaterialApp showed a spinner during init, but mounting
  // one on web rewrites the URL fragment to `#/`, dropping an incoming
  // `/share/...` deep link (the "bounces to login on first open" bug). The init
  // is storage-only and fast, so we run it up front and mount MaterialApp.router
  // straight away, leaving the launch URL intact for the share viewer.
  final container = ProviderContainer();
  await container.read(apiClientProvider).init();
  await container.read(authStateProvider.notifier).init();

  runApp(UncontrolledProviderScope(
    container: container,
    child: const PassbubbleApp(),
  ));
}

class PassbubbleApp extends ConsumerWidget {
  const PassbubbleApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    return AutoLockScope(
      child: MaterialApp.router(
        title: 'Passbubble',
        theme: AppTheme.dark,
        routerConfig: router,
        scaffoldMessengerKey: scaffoldMessengerKey,
        debugShowCheckedModeBanner: false,
      ),
    );
  }
}

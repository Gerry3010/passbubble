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

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/auth/auth_service.dart';
import '../../features/setup/setup_screen.dart';
import '../../features/auth/login_screen.dart';
import '../../features/share/share_viewer_screen.dart';
import '../../features/auth/unlock_screen.dart';
import '../../features/auth/register_screen.dart';
import '../../features/entries/entries_list_screen.dart';
import '../../features/entries/entry_detail_screen.dart';
import '../../features/entries/add_edit_screen.dart';
import '../../features/generate/generate_screen.dart';
import '../../features/manage/manage_screen.dart';
import '../../features/settings/settings_screen.dart';
import '../../features/settings/update_screen.dart';

/// The route the app was launched with, captured once at process start.
///
/// On web the app gates startup behind an async-init splash that is a plain
/// (non-router) [MaterialApp]. Mounting such an app on web **rewrites the
/// browser fragment to `#/`**, destroying an incoming `/share/...` deep link
/// before [routerProvider] (and thus [_initialLocation]) is ever built. So we
/// snapshot `Uri.base.fragment` in [captureLaunchUri], which `main()` calls
/// *before* `runApp` — while the original URL is still intact.
String? _launchFragment;

/// Must be called from `main()` before the first `runApp`, so the share deep
/// link survives the splash MaterialApp overwriting the URL fragment.
void captureLaunchUri() => _launchFragment ??= Uri.base.fragment;

/// Recovers a public share link from the captured launch fragment (hash routing
/// puts the route after '#') so it opens without bouncing to /login.
String _initialLocation() {
  final frag = _launchFragment ?? Uri.base.fragment;
  // Preserve public/unauthenticated deep links (share viewer, invitation
  // register link) so they open without bouncing to /login on first load.
  if (frag.startsWith('/share') || frag.startsWith('/register')) return frag;
  return '/entries';
}

final routerProvider = Provider<GoRouter>((ref) {
  // Build the GoRouter exactly ONCE. Watching authStateProvider here would
  // recreate the whole router on every auth change, which resets navigation and
  // drops the initial deep link (the "first open bounces to /login" bug).
  // Instead, refresh redirects via a listenable and read auth inside redirect.
  final refresh = ValueNotifier(0);
  ref.listen(authStateProvider, (_, _) => refresh.value++);
  ref.onDispose(refresh.dispose);

  return GoRouter(
    initialLocation: _initialLocation(),
    refreshListenable: refresh,
    redirect: (context, state) {
      final auth = ref.read(authStateProvider);
      final path = state.matchedLocation;
      final isSetup = path.startsWith('/setup');
      final isLogin = path.startsWith('/login');
      final isRegister = path.startsWith('/register');
      // Public share-link viewer is reachable without an account.
      if (path.startsWith('/share')) return null;

      if (!auth.isLoggedIn && !isSetup && !isLogin && !isRegister) {
        return '/login';
      }
      if (auth.isLoggedIn && !auth.isUnlocked &&
          !path.startsWith('/unlock') && !isLogin) {
        return '/unlock';
      }
      if (auth.isLoggedIn && auth.isUnlocked &&
          (isLogin || path.startsWith('/unlock'))) {
        return '/entries';
      }
      return null;
    },
    routes: [
      GoRoute(path: '/setup', builder: (_, _) => const SetupScreen()),
      GoRoute(path: '/login', builder: (_, _) => const LoginScreen()),
      GoRoute(
        path: '/register',
        builder: (_, state) => RegisterScreen(
          initialToken: state.uri.queryParameters['token'],
          initialEmail: state.uri.queryParameters['email'],
        ),
      ),
      GoRoute(path: '/unlock', builder: (_, _) => const UnlockScreen()),
      GoRoute(
        path: '/share/:token',
        builder: (_, state) => ShareViewerScreen(
          token: state.pathParameters['token'] ?? '',
          k: state.uri.queryParameters['k'] ?? '',
        ),
      ),
      GoRoute(
        path: '/entries',
        builder: (_, _) => const EntriesListScreen(),
        routes: [
          GoRoute(
            path: 'new',
            builder: (_, state) => AddEditScreen(
              folderId: state.uri.queryParameters['folderId'],
            ),
          ),
          GoRoute(
            path: ':id/edit',
            builder: (_, state) =>
                AddEditScreen(editId: state.pathParameters['id']),
          ),
          GoRoute(
            path: ':id',
            builder: (_, state) =>
                EntryDetailScreen(id: state.pathParameters['id']!),
          ),
        ],
      ),
      GoRoute(path: '/generate', builder: (_, _) => const GenerateScreen()),
      GoRoute(path: '/manage', builder: (_, _) => const ManageScreen()),
      GoRoute(
        path: '/settings',
        builder: (_, _) => const SettingsScreen(),
        routes: [
          GoRoute(
            path: 'update',
            builder: (_, _) => const UpdateScreen(),
          ),
        ],
      ),
    ],
  );
});

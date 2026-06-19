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

/// On web the app gates startup behind an async-init splash (a plain
/// MaterialApp), which makes GoRouter lose the deep-link route and fall back to
/// this initialLocation. Recover public share links from the URL fragment (hash
/// routing puts the route after '#') so they open without bouncing to /login.
String _initialLocation() {
  final frag = Uri.base.fragment;
  return frag.startsWith('/share') ? frag : '/entries';
}

final routerProvider = Provider<GoRouter>((ref) {
  final auth = ref.watch(authStateProvider);
  return GoRouter(
    initialLocation: _initialLocation(),
    redirect: (context, state) {
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
      GoRoute(path: '/register', builder: (_, _) => const RegisterScreen()),
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

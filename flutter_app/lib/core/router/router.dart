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
import 'package:go_router/go_router.dart';

import '../../core/api/api_client.dart';
import '../../core/auth/auth_service.dart';
import '../../features/setup/setup_screen.dart';
import '../../features/auth/login_screen.dart';
import '../../features/share/share_viewer_screen.dart';
import '../../features/auth/unlock_screen.dart';
import '../../features/auth/register_screen.dart';
import '../../features/entries/entries_list_screen.dart';
import '../../features/entries/entry_detail_screen.dart';
import '../../features/entries/add_edit_screen.dart';
import '../../features/entries/trash_screen.dart';
import '../../features/health/health_screen.dart';
import '../../features/generate/generate_screen.dart';
import '../../features/manage/manage_screen.dart';
import '../../features/settings/settings_screen.dart';
import '../../features/wallet/wallet_screen.dart';
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

/// Index of the tab shown last, so the next tab switch knows its direction.
/// Seeded to 0 (`/entries`, the launch tab).
///
/// The bottom-nav tab order is: 0 `/entries` (vault), 1 `/wallet`,
/// 2 `/generate`, 3 `/manage`, 4 `/settings` — see [_tabPage] call sites.
int _lastTabIndex = 0;

/// Wraps a tab screen in a horizontal slide keyed to its position relative to
/// the previously shown tab: switching to a higher [index] slides in from the
/// right, a lower index from the left — so the motion matches the tab bar's
/// left-to-right layout instead of every tab animating in from the same side.
///
/// Used only for the five bottom-nav roots; nested routes (entry detail,
/// add/edit, settings sub-pages) keep the default push transition.
CustomTransitionPage<void> _tabPage(
  GoRouterState state,
  int index,
  Widget child,
) {
  final fromRight = index >= _lastTabIndex;
  _lastTabIndex = index;
  return CustomTransitionPage<void>(
    key: state.pageKey,
    transitionDuration: const Duration(milliseconds: 260),
    reverseTransitionDuration: const Duration(milliseconds: 260),
    child: child,
    transitionsBuilder: (context, animation, _, child) {
      final curved = CurvedAnimation(
        parent: animation,
        curve: Curves.easeOutCubic,
        reverseCurve: Curves.easeInCubic,
      );
      return SlideTransition(
        position: Tween<Offset>(
          begin: Offset(fromRight ? 1 : -1, 0),
          end: Offset.zero,
        ).animate(curved),
        child: FadeTransition(opacity: curved, child: child),
      );
    },
  );
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

      // Without a configured server URL every API call fails with "no host
      // specified in URI" — so funnel to /setup regardless of auth state. This
      // also self-heals an already-logged-in client that somehow lost its URL
      // (the old session-wipe bug), which would otherwise be stuck on a screen
      // it can never load. Web always has a URL (the serving origin).
      final api = ref.read(apiClientProvider);
      if (!api.isConfigured && !isSetup && !isRegister) {
        return '/setup';
      }

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
        pageBuilder: (_, state) => _tabPage(state, 0, const EntriesListScreen()),
        routes: [
          GoRoute(
            path: 'new',
            builder: (_, state) => AddEditScreen(
              folderId: state.uri.queryParameters['folderId'],
              initialType: state.uri.queryParameters['type'],
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
      GoRoute(
        path: '/wallet',
        pageBuilder: (_, state) => _tabPage(state, 1, const WalletScreen()),
      ),
      GoRoute(path: '/trash', builder: (_, _) => const TrashScreen()),
      GoRoute(path: '/health', builder: (_, _) => const HealthScreen()),
      GoRoute(
        path: '/generate',
        pageBuilder: (_, state) => _tabPage(state, 2, const GenerateScreen()),
      ),
      GoRoute(
        path: '/manage',
        pageBuilder: (_, state) => _tabPage(
          state,
          3,
          ManageScreen(
            initialTab: int.tryParse(state.uri.queryParameters['tab'] ?? ''),
          ),
        ),
      ),
      GoRoute(
        path: '/settings',
        pageBuilder: (_, state) => _tabPage(state, 4, const SettingsScreen()),
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

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

import '../../features/entries/entries_list_screen.dart';
import '../api/models.dart';
import '../auth/auth_service.dart';
import '../router/router.dart';
import 'desktop_window.dart';
import 'tray_host.dart';
import 'tray_menu.dart';
import 'tray_settings.dart';

/// Keeps the tray icon in step with the vault: starts or stops it with the
/// setting, rebuilds the menu on lock, unlock and entry changes, and carries
/// out the menu's commands. While a tray host shows the icon, closing the
/// window only hides it.
class TrayScope extends ConsumerStatefulWidget {
  const TrayScope({super.key, required this.child});

  final Widget child;

  @override
  ConsumerState<TrayScope> createState() => _TrayScopeState();
}

class _TrayScopeState extends ConsumerState<TrayScope> {
  TrayHost? _host;
  bool _starting = false;
  bool _wasAvailable = false;

  @override
  void initState() {
    super.initState();
    if (traySupported) _init();
  }

  @override
  void dispose() {
    final host = _host;
    _host = null;
    host?.available.removeListener(_onAvailability);
    host?.close();
    super.dispose();
  }

  Future<void> _init() async {
    await ref.read(trayEnabledProvider.notifier).ready;
    if (mounted) await _apply(ref.read(trayEnabledProvider));
  }

  Future<void> _apply(bool enabled) async {
    if (enabled && _host == null && !_starting) {
      _starting = true;
      final host = await ref.read(trayHostFactoryProvider)(
        onSelected: _onSelected,
        onActivate: (token) => _show(token),
      );
      _starting = false;
      if (host == null) return;
      if (!mounted || !ref.read(trayEnabledProvider)) {
        await host.close();
        return;
      }
      _host = host;
      host.available.addListener(_onAvailability);
      _onAvailability();
      await _refresh();
    } else if (!enabled && _host != null) {
      final host = _host!;
      _host = null;
      host.available.removeListener(_onAvailability);
      _onAvailability();
      await host.close();
    }
  }

  void _onAvailability() {
    if (!mounted) return;
    final available = _host?.available.value ?? false;
    ref.read(trayAvailableProvider.notifier).state = available;
    final window = ref.read(desktopWindowProvider);
    window.setHideOnClose(available);
    // The tray vanished (extension turned off, setting switched off) while
    // the window may sit hidden in it: bring the window back.
    if (_wasAvailable && !available) window.show();
    _wasAvailable = available;
  }

  Future<void> _refresh() async {
    final host = _host;
    if (host == null) return;
    final auth = ref.read(authStateProvider);
    final unlocked = auth.isLoggedIn && auth.isUnlocked;
    final favorites = unlocked
        ? [
            for (final e in ref.read(entriesProvider).valueOrNull ??
                const <EntryResponse>[])
              if (e.favorite) (id: e.id, name: e.name),
          ]
        : const <TrayFavorite>[];
    await host.update(
      menu: buildTrayMenu(
          loggedIn: auth.isLoggedIn, unlocked: unlocked, favorites: favorites),
      unlocked: unlocked,
      toolTip: !auth.isLoggedIn
          ? 'Signed out'
          : unlocked
              ? 'Vault unlocked'
              : 'Vault locked',
    );
  }

  Future<void> _show(String? token) =>
      ref.read(desktopWindowProvider).show(activationToken: token);

  /// Goes to [location] unless an entry is being edited: a jump from the
  /// tray would throw the unsaved changes away, so the window just comes
  /// back with the editor open.
  void _go(String location) {
    final router = ref.read(routerProvider);
    final path = router.routerDelegate.currentConfiguration.uri.path;
    final editing = path == '/entries/new' || path.endsWith('/edit');
    if (!editing) router.go(location);
  }

  Future<void> _onSelected(TrayMenuItem item, String? token) async {
    switch (item.command) {
      case TrayCommand.open:
        await _show(token);
      case TrayCommand.search:
        ref.read(vaultSearchFocusRequestProvider.notifier).state = true;
        _go('/entries');
        await _show(token);
      case TrayCommand.newEntry:
        _go('/entries/new');
        await _show(token);
      case TrayCommand.generator:
        _go('/generate');
        await _show(token);
      case TrayCommand.openEntry:
        _go('/entries/${item.entryId}');
        await _show(token);
      case TrayCommand.lock:
        ref.read(authStateProvider.notifier).lock();
      case TrayCommand.quit:
        await ref.read(desktopWindowProvider).quit();
      case null:
        break;
    }
  }

  @override
  Widget build(BuildContext context) {
    if (traySupported) {
      ref.listen(trayEnabledProvider, (_, enabled) => _apply(enabled));
      ref.listen(authStateProvider, (_, _) => _refresh());
      ref.listen(entriesProvider, (_, _) => _refresh());
    }
    return widget.child;
  }
}

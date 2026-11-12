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

import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// The window controls of the Linux runner (`linux/runner/my_application.cc`).
class DesktopWindow {
  const DesktopWindow();

  static const _channel =
      MethodChannel('net.geraldhofbauer.passbubble/window');

  /// Shows the window and asks for focus. [activationToken] comes from the
  /// tray host and lets the compositor hand focus over.
  Future<void> show({String? activationToken}) =>
      _channel.invokeMethod('show', activationToken);

  Future<void> hide() => _channel.invokeMethod('hide');

  /// While on, closing the window hides it instead of ending the app.
  Future<void> setHideOnClose(bool value) =>
      _channel.invokeMethod('setHideOnClose', value);

  Future<void> quit() => _channel.invokeMethod('quit');
}

final desktopWindowProvider =
    Provider<DesktopWindow>((_) => const DesktopWindow());

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

// Debug-only entrypoint that boots Passbubble with the Marionette binding so the
// Marionette MCP can inspect and drive the running app. It lives OUTSIDE lib/ on
// purpose: production lib/main.dart never references marionette_flutter (a
// dev_dependency), so normal builds and `flutter build` are completely
// unaffected. Nothing here is shipped.
//
// Run it explicitly:
//   flutter run -t dev/main_marionette.dart -d linux
//
// Then connect the Marionette MCP to the VM Service URI printed in the console.

import 'package:marionette_flutter/marionette_flutter.dart';
import 'package:passbubble/main.dart' as app;

Future<void> main() async {
  // MarionetteBinding must be the ONLY WidgetsBinding initialised in the
  // process, so we init it here instead of the WidgetsFlutterBinding that
  // production main() uses, then hand off to the shared bootstrap.
  MarionetteBinding.ensureInitialized();
  await app.bootstrap();
}

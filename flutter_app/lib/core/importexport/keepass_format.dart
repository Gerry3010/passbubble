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

import 'dart:typed_data';

import 'entry_record.dart';

// KeePass (.kdbx) import is not supported on Flutter Web because the required
// native libraries are not available in the browser. Use the CLI instead:
//   pwmgr import file.kdbx --format keepass
Future<List<EntryRecord>> parseKeePass(Uint8List bytes, String password) {
  throw UnsupportedError(
    'KeePass import is not available in the web app. '
    'Use the CLI: pwmgr import <file.kdbx> --format keepass',
  );
}

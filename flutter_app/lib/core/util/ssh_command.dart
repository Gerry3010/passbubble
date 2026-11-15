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

/// The `ssh` command line for a `server` entry: `ssh [-p PORT] [user@]host`.
/// The port is left out when it is empty or 22, the `user@` when there is no
/// username.
String sshCommand(String host, {String? port, String? user}) {
  final p = port?.trim() ?? '';
  final u = user?.trim() ?? '';
  return [
    'ssh',
    if (p.isNotEmpty && p != '22') '-p $p',
    u.isEmpty ? host.trim() : '$u@${host.trim()}',
  ].join(' ');
}

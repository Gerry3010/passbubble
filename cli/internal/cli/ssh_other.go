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

//go:build !unix

package cli

import "errors"

// errSSHUnsupported: the in-process agent and the askpass handshake use Unix
// sockets and /dev/tty.
var errSSHUnsupported = errors.New("pwmgr ssh runs on Linux, macOS and the BSDs only; " +
	"here, use 'pwmgr ssh <entry> --print' and 'pwmgr get <entry>' to connect by hand")

func runSSH(sshSession) (int, error) { return 0, errSSHUnsupported }

// runAskpass is never reached: runSSH sets no askpass here.
func runAskpass(string, string) int { return 1 }

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

package cli

import (
	"os"
	"testing"
)

// TestMain lets the test binary stand in for the programs 'pwmgr ssh' starts:
// ssh runs os.Executable() as its askpass program, which is this binary in a
// test, and the ssh tests start this binary as a fake ssh.
func TestMain(m *testing.M) {
	if code, ran := RunAskpassIfRequested(); ran {
		os.Exit(code)
	}
	if code, ran := runFakeSSHIfRequested(); ran {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

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
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh/agent"
)

// The fake ssh is this test binary started through a link named ssh. It
// writes down what it got to the file in fakeSSHReportEnv, asks the askpass
// program like ssh would, and exits with the status in fakeSSHExitEnv.
const (
	fakeSSHReportEnv = "PWMGR_TEST_FAKE_SSH_REPORT"
	fakeSSHExitEnv   = "PWMGR_TEST_FAKE_SSH_EXIT"
)

// fakeSSHPrompt is the prompt the fake ssh asks, twice.
const fakeSSHPrompt = "tester@web01.example.com's password: "

type fakeSSHReport struct {
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	AgentKeys []string          `json:"agent_keys"`
	AgentErr  string            `json:"agent_err"`
	// FirstAnswer is what the askpass program printed for the first password
	// prompt; SecondAnswered whether the parent answered a second one.
	FirstAnswer    string `json:"first_answer"`
	AskpassErr     string `json:"askpass_err"`
	SecondAnswered bool   `json:"second_answered"`
	// SocketDir is where the sockets were, to check it is gone afterwards.
	SocketDir string `json:"socket_dir"`
}

func runFakeSSHIfRequested() (int, bool) {
	reportPath := os.Getenv(fakeSSHReportEnv)
	if reportPath == "" || filepath.Base(os.Args[0]) != "ssh" {
		return 0, false
	}
	r := fakeSSHReport{Args: os.Args[1:], Env: map[string]string{}}
	for _, k := range []string{"SSH_AUTH_SOCK", "SSH_ASKPASS", "SSH_ASKPASS_REQUIRE", askpassSocketEnv} {
		if val, ok := os.LookupEnv(k); ok {
			r.Env[k] = val
		}
	}
	if sock := r.Env["SSH_AUTH_SOCK"]; sock != "" && strings.Contains(sock, "pwmgr-ssh-") {
		r.SocketDir = filepath.Dir(sock)
		if conn, err := net.DialTimeout("unix", sock, 5*time.Second); err != nil {
			r.AgentErr = err.Error()
		} else {
			keys, err := agent.NewClient(conn).List()
			if err != nil {
				r.AgentErr = err.Error()
			}
			for _, k := range keys {
				r.AgentKeys = append(r.AgentKeys, k.Comment+" "+k.Format)
			}
			_ = conn.Close()
		}
	}
	if askpass := r.Env["SSH_ASKPASS"]; askpass != "" {
		r.SocketDir = filepath.Dir(r.Env[askpassSocketEnv])
		// Like ssh: the prompt is the only argument, the answer comes on stdout.
		out, err := exec.Command(askpass, fakeSSHPrompt).Output()
		if err != nil {
			r.AskpassErr = err.Error()
		}
		r.FirstAnswer = strings.TrimRight(string(out), "\n")
		// A second password prompt must not get the password again. Asked
		// directly: the askpass program would turn to the terminal.
		_, r.SecondAnswered = askParent(r.Env[askpassSocketEnv], fakeSSHPrompt)
	}
	b, _ := json.Marshal(r)
	if err := os.WriteFile(reportPath, b, 0o600); err != nil {
		return 99, true
	}
	code, _ := strconv.Atoi(os.Getenv(fakeSSHExitEnv))
	return code, true
}

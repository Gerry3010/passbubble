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

//go:build integration && unix

package cli

// pwmgr ssh against a real OpenSSH server, for example a throwaway container:
//
//	ssh-keygen -t ed25519 -N '' -f $D/plain; ssh-keygen -t ed25519 -N "$PP" -f $D/enc
//	docker run -d --rm --name pwmgr-ssh-test -p 127.0.0.1:$PORT:2222 \
//	  -e USER_NAME=tester -e PASSWORD_ACCESS=true -e USER_PASSWORD=$PW \
//	  -e PUBLIC_KEY="$(cat $D/plain.pub $D/enc.pub)" lscr.io/linuxserver/openssh-server:latest
//	PWMGR_E2E_SSH_PORT=$PORT PWMGR_E2E_SSH_USER=tester PWMGR_E2E_SSH_PASSWORD=$PW \
//	PWMGR_E2E_SSH_KEY=$D/plain PWMGR_E2E_SSH_KEY_ENC=$D/enc PWMGR_E2E_SSH_KEY_PASSPHRASE="$PP" \
//	  go test -tags integration -run E2E ./internal/cli
//
// The ssh options keep the run away from ~/.ssh: no config, no known_hosts,
// no default identity files, and the user's agent is never asked.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
)

type e2eServer struct {
	port, user, password, passphrase string
	key, keyEnc                      string
}

func e2eSettings(t *testing.T) e2eServer {
	t.Helper()
	get := func(k string) string {
		v := os.Getenv(k)
		if v == "" {
			t.Skipf("%s not set: no SSH server to test against", k)
		}
		return v
	}
	read := func(k string) string {
		b, err := os.ReadFile(get(k))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	return e2eServer{
		port: get("PWMGR_E2E_SSH_PORT"), user: get("PWMGR_E2E_SSH_USER"),
		password: get("PWMGR_E2E_SSH_PASSWORD"), passphrase: get("PWMGR_E2E_SSH_KEY_PASSPHRASE"),
		key: read("PWMGR_E2E_SSH_KEY"), keyEnc: read("PWMGR_E2E_SSH_KEY_ENC"),
	}
}

// e2eSSHOptions keep ssh off the user's files. keyOnly forbids passwords,
// passwordOnly forbids keys.
func e2eSSHOptions(keyOnly bool) []string {
	opts := []string{
		"-F", "/dev/null",
		"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "IdentityFile=none", "-o", "LogLevel=ERROR", "-o", "ConnectTimeout=10",
	}
	if keyOnly {
		return append(opts, "-o", "BatchMode=yes", "-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no")
	}
	return append(opts, "-o", "PubkeyAuthentication=no", "-o", "IdentityAgent=none")
}

// e2eEntries stores one server entry per way to log in.
func e2eEntries(t *testing.T, cfg e2eServer) *commandVault {
	t.Helper()
	s := newCommandVault(t, `{"username":"octo","password":"pw"}`)
	add := func(id, name string, data map[string]string) {
		data["host"], data["port"], data["username"] = "127.0.0.1", cfg.port, cfg.user
		b, _ := json.Marshal(data)
		s.addEntry(t, apiclient.EntryResponse{ID: id, Name: name, Type: "server"}, string(b))
	}
	add("k1", "key-plain", map[string]string{"private_key": cfg.key})
	add("k2", "key-encrypted", map[string]string{"private_key": cfg.keyEnc, "key_passphrase": cfg.passphrase})
	add("p1", "password", map[string]string{"password": cfg.password})
	add("p2", "password-wrong", map[string]string{"password": "wrong-" + cfg.password})
	add("n1", "nothing", map[string]string{})
	return s
}

func TestE2ESSHInProcess(t *testing.T) {
	cfg := e2eSettings(t)
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("no ssh")
	}
	e2eEntries(t, cfg)
	t.Setenv("SSH_AUTH_SOCK", "") // the user's agent stays out of it

	for _, tc := range []struct {
		entry   string
		keyOnly bool
		code    int
	}{
		{"key-plain", true, 0},
		{"key-encrypted", true, 0},
		{"password", false, 0},
		{"nothing", true, 255}, // no key, no password: ssh fails and pwmgr says so
	} {
		t.Run(tc.entry, func(t *testing.T) {
			args := append([]string{tc.entry, "--"}, e2eSSHOptions(tc.keyOnly)...)
			args = append(args, "echo", "ok-"+tc.entry)
			out, err := runSSHCommand(t, args...)
			sshCmd.SilenceErrors = false
			if got := exitCodeOf(err); got != tc.code {
				t.Fatalf("exit %d (err %v), want %d; output %q", got, err, tc.code, out)
			}
			if tc.code == 0 && out != "ok-"+tc.entry+"\n" {
				t.Fatalf("remote output %q", out)
			}
		})
	}
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var status *exitStatusError
	if errors.As(err, &status) {
		return status.code
	}
	return -1
}

// The real binary, against a fake API server, in a pseudo terminal for the
// master password prompt: what a user runs.
func TestE2ESSHRealBinary(t *testing.T) {
	cfg := e2eSettings(t)
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("needs script(1) for a pseudo terminal")
	}
	e2eEntries(t, cfg)

	dir := t.TempDir()
	bin := filepath.Join(dir, "pwmgr")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/pwmgr")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cfgPath := filepath.Join(dir, "pwmgr.yaml")
	if err := v.Config().Save(cfgPath); err != nil {
		t.Fatal(err)
	}

	type step struct{ waitFor, send string }
	unlock := step{"Master password:", "correct horse\n"}
	for _, tc := range []struct {
		entry   string
		options []string
		steps   []step
	}{
		{"key-plain", e2eSSHOptions(true), []step{unlock}},
		{"key-encrypted", e2eSSHOptions(true), []step{unlock}},
		{"password", e2eSSHOptions(false), []step{unlock}},
		// The host key question comes to the terminal through the askpass
		// program, visible; the stored password is wrong, so the second
		// password prompt comes to the terminal too, hidden.
		{"password-wrong", append([]string{"-o", "StrictHostKeyChecking=ask"}, e2eSSHOptions(false)...), []step{
			unlock,
			{"(yes/no", "yes\n"},
			{"password:", cfg.password + "\n"},
		}},
	} {
		t.Run(tc.entry, func(t *testing.T) {
			args := []string{bin, "--config", cfgPath, "ssh", tc.entry, "--"}
			args = append(args, tc.options...)
			args = append(args, "echo", "ok-real-"+tc.entry)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "script", "-qec", shellJoin(args), "/dev/null")
			cmd.Env = append(os.Environ(), "SSH_AUTH_SOCK=")
			var output lockedBuffer
			cmd.Stdout, cmd.Stderr = &output, &output
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// Type each answer once its prompt is on the terminal. Sent earlier,
			// the master password would answer the terminal queries pwmgr makes
			// at start-up, which script(1) leaves unanswered.
			seen := 0
			for _, st := range tc.steps {
				for ctx.Err() == nil {
					if i := strings.Index(output.String()[seen:], st.waitFor); i >= 0 {
						seen += i + len(st.waitFor)
						break
					}
					time.Sleep(20 * time.Millisecond)
				}
				_, _ = stdin.Write([]byte(st.send))
			}
			_ = stdin.Close()
			err = cmd.Wait()
			out := output.String()
			if err != nil {
				t.Fatalf("pwmgr ssh: %v\n%s", err, out)
			}
			if !strings.Contains(out, "ok-real-"+tc.entry) {
				t.Fatalf("no remote output:\n%s", out)
			}
			if strings.Contains(out, cfg.password) || strings.Contains(out, cfg.passphrase) {
				t.Fatalf("a secret reached the terminal:\n%s", out)
			}
			t.Logf("%q", out)
		})
	}

	// No pwmgr-ssh-* directory is left behind.
	leftovers, _ := filepath.Glob(filepath.Join(os.TempDir(), "pwmgr-ssh-*"))
	if len(leftovers) != 0 {
		t.Errorf("left behind: %v", leftovers)
	}
}

// lockedBuffer collects output written from another goroutine.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

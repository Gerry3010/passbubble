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

//go:build unix

package cli

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// listAgent connects to an agent socket and lists its keys.
func listAgent(t *testing.T, sock string) []*agent.Key {
	t.Helper()
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	keys, err := agent.NewClient(conn).List()
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func TestAgentServesTheEntrysKey(t *testing.T) {
	for name, passphrase := range map[string]string{"unencrypted": "", "with stored passphrase": "s3cret"} {
		t.Run(name, func(t *testing.T) {
			pem, pub := testSSHKey(t, "")
			if passphrase != "" {
				pem, pub = sharedEncryptedKey(t)
			}
			raw, err := parseServerKey("web01", pem, passphrase)
			if err != nil {
				t.Fatal(err)
			}
			srv, sock, err := startAgent(t.TempDir(), raw, "pwmgr: web01")
			if err != nil {
				t.Fatal(err)
			}
			defer srv.close()

			info, err := os.Stat(sock)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("socket mode %v, err %v", info.Mode(), err)
			}
			keys := listAgent(t, sock)
			if len(keys) != 1 || keys[0].Comment != "pwmgr: web01" || string(keys[0].Blob) != string(pub.Marshal()) {
				t.Fatalf("agent lists %v", keys)
			}

			// It signs with it, which is what ssh asks of it.
			conn, err := net.Dial("unix", sock)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			sig, err := agent.NewClient(conn).Sign(pub, []byte("data"))
			if err != nil {
				t.Fatal(err)
			}
			if err := pub.Verify([]byte("data"), sig); err != nil {
				t.Fatalf("signature does not verify: %v", err)
			}

			srv.close()
			if _, err := net.Dial("unix", sock); err == nil {
				t.Error("the agent still accepts connections after close")
			}
		})
	}
}

func TestAskpassSocketEndToEnd(t *testing.T) {
	srv, sock, err := startAskpassServer(t.TempDir(), &askpassAnswerer{password: "hunter2", host: "web01"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.close()
	if answer, ok := askParent(sock, "root@web01's password: "); !ok || answer != "hunter2" {
		t.Fatalf("first: %q %v", answer, ok)
	}
	if _, ok := askParent(sock, "root@web01's password: "); ok {
		t.Fatal("second password prompt answered")
	}
	if _, ok := askParent(filepath.Join(t.TempDir(), "missing.sock"), "Password: "); ok {
		t.Fatal("no parent, no answer")
	}
}

// fakeSSH puts this test binary in place of ssh and returns where its report
// lands.
func fakeSSH(t *testing.T, exitCode string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "ssh")
	if err := os.Symlink(self, link); err != nil {
		t.Fatal(err)
	}
	orig := lookPath
	lookPath = func(name string) (string, error) {
		if name != "ssh" {
			return "", errors.New("unexpected lookup " + name)
		}
		return link, nil
	}
	t.Cleanup(func() { lookPath = orig })
	report := filepath.Join(dir, "report.json")
	t.Setenv(fakeSSHReportEnv, report)
	t.Setenv(fakeSSHExitEnv, exitCode)
	return report
}

func readFakeSSHReport(t *testing.T, path string) fakeSSHReport {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ssh did not run: %v", err)
	}
	var r fakeSSHReport
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// The whole command against a stand-in ssh: arguments, agent, askpass, exit
// status and the cleanup.
func TestSSHRunsSSHWithAgentAndAskpass(t *testing.T) {
	key, pub := sharedEncryptedKey(t)
	s := newCommandVault(t, `{"username":"octo","password":"pw"}`)
	stored, _ := json.Marshal(map[string]string{
		"username": "tester", "password": "hunter2", "host": "web01.example.com", "port": "2222",
		"private_key": key, "key_passphrase": "s3cret",
	})
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "web01", Type: "server"}, string(stored))
	t.Setenv("SSH_AUTH_SOCK", "/run/user/the-users-own-agent")
	report := fakeSSH(t, "3")

	_, err := runSSHCommand(t, "web01", "--", "-o", "StrictHostKeyChecking=no", "echo", "ok")
	var status *exitStatusError
	if !errors.As(err, &status) || status.code != 3 || ExitCode(err) != 3 {
		t.Fatalf("ssh's exit status must come through, err = %v", err)
	}
	if !sshCmd.SilenceErrors {
		t.Error("ssh has reported the failure; pwmgr should not add its own message")
	}
	sshCmd.SilenceErrors = false

	r := readFakeSSHReport(t, report)
	sock := r.Env["SSH_AUTH_SOCK"]
	if !strings.Contains(sock, "pwmgr-ssh-") || sock == "/run/user/the-users-own-agent" {
		t.Fatalf("ssh got the agent socket %q", sock)
	}
	wantArgs := []string{"-p", "2222", "-l", "tester", "-o", `IdentityAgent="` + sock + `"`, "web01.example.com",
		"-o", "StrictHostKeyChecking=no", "echo", "ok"}
	if !slices.Equal(r.Args, wantArgs) {
		t.Errorf("args\n got: %q\nwant: %q", r.Args, wantArgs)
	}
	if r.AgentErr != "" || len(r.AgentKeys) != 1 || r.AgentKeys[0] != "pwmgr: web01 "+pub.Type() {
		t.Errorf("agent keys %v, err %q", r.AgentKeys, r.AgentErr)
	}
	self, _ := os.Executable()
	if r.Env["SSH_ASKPASS"] != self || r.Env["SSH_ASKPASS_REQUIRE"] != "force" ||
		filepath.Dir(r.Env[askpassSocketEnv]) != filepath.Dir(sock) {
		t.Errorf("askpass env = %v", r.Env)
	}
	if r.AskpassErr != "" || r.FirstAnswer != "hunter2" {
		t.Errorf("askpass printed %q (err %q), want the stored password", r.FirstAnswer, r.AskpassErr)
	}
	if r.SecondAnswered {
		t.Error("the password went out twice")
	}
	if _, err := os.Stat(r.SocketDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the socket directory %s is still there: %v", r.SocketDir, err)
	}
}

func TestSSHWithoutSecretsLeavesTheEnvironmentAlone(t *testing.T) {
	s := newCommandVault(t, `{"username":"octo","password":"pw"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "web01", Type: "server"}, `{"host":"web01"}`)
	t.Setenv("SSH_AUTH_SOCK", "/run/user/the-users-own-agent")
	report := fakeSSH(t, "0")

	if _, err := runSSHCommand(t, "web01"); err != nil {
		t.Fatal(err)
	}
	r := readFakeSSHReport(t, report)
	if !slices.Equal(r.Args, []string{"web01"}) {
		t.Errorf("args = %q", r.Args)
	}
	if r.Env["SSH_AUTH_SOCK"] != "/run/user/the-users-own-agent" {
		t.Errorf("without a key the user's agent stays: %v", r.Env)
	}
	for _, k := range []string{"SSH_ASKPASS", "SSH_ASKPASS_REQUIRE", askpassSocketEnv} {
		if _, ok := r.Env[k]; ok {
			t.Errorf("without a password %s must not be set: %v", k, r.Env)
		}
	}
}

func TestSSHAsksForAKeyPassphraseItDoesNotStore(t *testing.T) {
	key, pub := sharedEncryptedKey(t)
	s := newCommandVault(t, `{"username":"octo","password":"pw"}`)
	stored, _ := json.Marshal(map[string]string{"host": "web01", "private_key": key})
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "web01", Type: "server"}, string(stored))
	asked := withKeyPassphrasePrompt(t, "s3cret")
	report := fakeSSH(t, "0")

	if _, err := runSSHCommand(t, "web01"); err != nil {
		t.Fatal(err)
	}
	r := readFakeSSHReport(t, report)
	if *asked != 1 || len(r.AgentKeys) != 1 || r.AgentKeys[0] != "pwmgr: web01 "+ssh.KeyAlgoED25519 || pub.Type() != ssh.KeyAlgoED25519 {
		t.Fatalf("asked %d, agent keys %v", *asked, r.AgentKeys)
	}
}

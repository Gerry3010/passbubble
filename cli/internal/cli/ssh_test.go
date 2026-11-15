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
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/vault"
	"golang.org/x/crypto/ssh"
)

// testSSHKey returns a fresh ed25519 key in OpenSSH format, encrypted when a
// passphrase is given, and its public key.
func testSSHKey(t *testing.T, passphrase string) (string, ssh.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(priv, "test")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "test", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block)), sshPub
}

func TestSSHArgs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target sshTarget
		agent  string
		extra  []string
		want   []string
	}{
		{"host only", sshTarget{host: "web01"}, "", nil, []string{"web01"}},
		{"port and user", sshTarget{host: "web01", port: "2222", user: "root"}, "", nil,
			[]string{"-p", "2222", "-l", "root", "web01"}},
		{"default port stays explicit when stored", sshTarget{host: "web01", port: "22"}, "", nil,
			[]string{"-p", "22", "web01"}},
		{"agent and extra args after the host", sshTarget{host: "::1", port: "2222", user: "tester"}, "/tmp/pwmgr-ssh-1/agent.sock",
			[]string{"-o", "StrictHostKeyChecking=no", "echo", "ok"},
			[]string{"-p", "2222", "-l", "tester", "-o", `IdentityAgent="/tmp/pwmgr-ssh-1/agent.sock"`, "::1",
				"-o", "StrictHostKeyChecking=no", "echo", "ok"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sshArgs(tc.target, tc.agent, tc.extra); !slices.Equal(got, tc.want) {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestSSHTargetRefusesWhatSSHWouldReadAsOptions(t *testing.T) {
	good, err := sshTargetOf(&vault.EntryData{Host: " web01 ", Port: "0022", Username: "root"})
	if err != nil || good != (sshTarget{host: "web01", port: "22", user: "root"}) {
		t.Fatalf("target = %+v, %v", good, err)
	}
	for name, d := range map[string]vault.EntryData{
		"no host":           {},
		"option as host":    {Host: "-oProxyCommand=evil"},
		"user in host":      {Host: "root@web01"},
		"space in host":     {Host: "web01 -v"},
		"port out of range": {Host: "web01", Port: "70000"},
		"port not a number": {Host: "web01", Port: "ssh"},
		"option as user":    {Host: "web01", Username: "-oProxyCommand=evil"},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := sshTargetOf(&d); err == nil {
				t.Fatalf("accepted %+v", got)
			}
		})
	}
}

func TestSSHCommandLineForGet(t *testing.T) {
	for _, tc := range []struct {
		data vault.EntryData
		want string
	}{
		{vault.EntryData{Host: "web01"}, "ssh web01"},
		{vault.EntryData{Host: "web01", Port: "22", Username: "root"}, "ssh root@web01"},
		{vault.EntryData{Host: "web01", Port: "2222", Username: "root"}, "ssh -p 2222 root@web01"},
		{vault.EntryData{Host: "web01", Username: "o'neil"}, `ssh 'o'\''neil@web01'`},
	} {
		if got := sshCommandLine(&tc.data); got != tc.want {
			t.Errorf("%+v: got %q, want %q", tc.data, got, tc.want)
		}
	}
}

func TestPromptClassification(t *testing.T) {
	for _, tc := range []struct {
		prompt           string
		password, hidden bool
	}{
		{"tester@127.0.0.1's password: ", true, true},
		{"(tester@127.0.0.1) Password: ", true, true},
		{"Password:", true, true},
		{"  PASSWORD:  ", true, true},
		{"Password for tester@host: ", false, true},
		{"Enter passphrase for key '/home/x/.ssh/id_ed25519': ", false, true},
		{"Verification code: ", false, true},
		{"(tester@host) One-time password (OATH) for `tester': ", false, true},
		{"Enter PIN for authenticator: ", false, true},
		{"The authenticity of host 'web01 (10.0.0.1)' can't be established.\nED25519 key fingerprint is SHA256:x.\nAre you sure you want to continue connecting (yes/no/[fingerprint])? ", false, false},
		{"Allow use of key? ", false, false},
	} {
		if got := isPasswordPrompt(tc.prompt); got != tc.password {
			t.Errorf("isPasswordPrompt(%q) = %v", tc.prompt, got)
		}
		if got := isHiddenPrompt(tc.prompt); got != tc.hidden {
			t.Errorf("isHiddenPrompt(%q) = %v", tc.prompt, got)
		}
	}
}

func TestAskpassAnswersThePasswordOnceAndOnlyForTheTarget(t *testing.T) {
	ask := func(a *askpassAnswerer, prompt string) askpassReply {
		t.Helper()
		parent, child := net.Pipe()
		done := make(chan error, 1)
		go func() {
			done <- a.serve(parent)
			_ = parent.Close()
		}()
		reply, err := queryAskpass(child, prompt)
		if err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		_ = child.Close()
		return reply
	}

	a := &askpassAnswerer{password: "hunter2", host: "web01.example.com"}
	for _, prompt := range []string{
		"Enter passphrase for key '/home/x/.ssh/id_ed25519': ", // not a password prompt
		"Are you sure you want to continue connecting (yes/no/[fingerprint])? ",
		"jumper@bastion.example.com's password: ", // a ProxyJump host
	} {
		if r := ask(a, prompt); r.Answered || r.Answer != "" {
			t.Fatalf("%q was answered: %+v", prompt, r)
		}
	}
	if r := ask(a, "tester@WEB01.example.com's password: "); !r.Answered || r.Answer != "hunter2" {
		t.Fatalf("the password prompt was not answered: %+v", r)
	}
	if r := ask(a, "tester@web01.example.com's password: "); r.Answered {
		t.Fatalf("a second password prompt means the password was wrong, got %+v", r)
	}

	// A prompt without user@host is answered too (keyboard-interactive of old
	// OpenSSH versions).
	if r := ask(&askpassAnswerer{password: "pw", host: "h"}, "Password: "); !r.Answered || r.Answer != "pw" {
		t.Fatalf("plain prompt: %+v", r)
	}
	// Nothing stored, nothing answered.
	if r := ask(&askpassAnswerer{host: "h"}, "Password: "); r.Answered {
		t.Fatalf("empty password: %+v", r)
	}
}

func TestAskpassRejectsGarbage(t *testing.T) {
	parent, child := net.Pipe()
	defer func() { _ = child.Close() }()
	done := make(chan error, 1)
	go func() { done <- (&askpassAnswerer{password: "pw"}).serve(parent) }()
	if _, err := child.Write([]byte("not json\n")); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("garbage must be an error")
	}
}

func TestIsAskpassInvocation(t *testing.T) {
	for _, tc := range []struct {
		socket string
		args   []string
		want   bool
	}{
		{"/tmp/s", []string{"/usr/bin/pwmgr", "tester@host's password: "}, true},
		{"/tmp/s", []string{"/usr/bin/pwmgr", "Enter passphrase for key: "}, true},
		{"", []string{"/usr/bin/pwmgr", "tester@host's password: "}, false},
		// pwmgr commands run under ssh, e.g. from a ProxyCommand, stay commands.
		{"/tmp/s", []string{"/usr/bin/pwmgr", "list"}, false},
		{"/tmp/s", []string{"/usr/bin/pwmgr", "edit"}, false},
		{"/tmp/s", []string{"/usr/bin/pwmgr", "help"}, false},
		{"/tmp/s", []string{"/usr/bin/pwmgr", "--version"}, false},
		{"/tmp/s", []string{"/usr/bin/pwmgr"}, false},
		{"/tmp/s", []string{"/usr/bin/pwmgr", "get", "x"}, false},
	} {
		if got := isAskpassInvocation(tc.socket, tc.args); got != tc.want {
			t.Errorf("isAskpassInvocation(%q, %q) = %v", tc.socket, tc.args, got)
		}
	}
}

func TestExitCode(t *testing.T) {
	if got := ExitCode(&exitStatusError{code: 255}); got != 255 {
		t.Errorf("got %d", got)
	}
	if got := ExitCode(errors.New("other")); got != 1 {
		t.Errorf("got %d", got)
	}
}

func withKeyPassphrasePrompt(t *testing.T, answer string) *int {
	t.Helper()
	asked := 0
	orig := promptKeyPassphrase
	promptKeyPassphrase = func(string) (string, error) { asked++; return answer, nil }
	t.Cleanup(func() { promptKeyPassphrase = orig })
	return &asked
}

func TestParseServerKey(t *testing.T) {
	plain, plainPub := testSSHKey(t, "")
	encrypted, encPub := sharedEncryptedKey(t)
	signerOf := func(t *testing.T, raw any) ssh.PublicKey {
		t.Helper()
		s, err := ssh.NewSignerFromKey(raw)
		if err != nil {
			t.Fatal(err)
		}
		return s.PublicKey()
	}
	same := func(a, b ssh.PublicKey) bool { return string(a.Marshal()) == string(b.Marshal()) }

	t.Run("unencrypted", func(t *testing.T) {
		asked := withKeyPassphrasePrompt(t, "")
		raw, err := parseServerKey("web01", plain, "")
		if err != nil || !same(signerOf(t, raw), plainPub) || *asked != 0 {
			t.Fatalf("raw %T, err %v, asked %d", raw, err, *asked)
		}
	})
	t.Run("stored passphrase", func(t *testing.T) {
		asked := withKeyPassphrasePrompt(t, "")
		raw, err := parseServerKey("web01", encrypted, "s3cret")
		if err != nil || !same(signerOf(t, raw), encPub) || *asked != 0 {
			t.Fatalf("raw %T, err %v, asked %d", raw, err, *asked)
		}
	})
	t.Run("no passphrase stored: asked", func(t *testing.T) {
		asked := withKeyPassphrasePrompt(t, "s3cret")
		raw, err := parseServerKey("web01", encrypted, "")
		if err != nil || !same(signerOf(t, raw), encPub) || *asked != 1 {
			t.Fatalf("raw %T, err %v, asked %d", raw, err, *asked)
		}
	})
	t.Run("wrong stored passphrase: asked", func(t *testing.T) {
		asked := withKeyPassphrasePrompt(t, "s3cret")
		var raw any
		var err error
		captureStderr(t, func() { raw, err = parseServerKey("web01", encrypted, "wrong") })
		if err != nil || !same(signerOf(t, raw), encPub) || *asked != 1 {
			t.Fatalf("raw %T, err %v, asked %d", raw, err, *asked)
		}
	})
	t.Run("wrong answer", func(t *testing.T) {
		withKeyPassphrasePrompt(t, "nope")
		if _, err := parseServerKey("web01", encrypted, ""); err == nil {
			t.Fatal("a wrong passphrase must fail")
		}
	})
	t.Run("not a key", func(t *testing.T) {
		asked := withKeyPassphrasePrompt(t, "")
		if _, err := parseServerKey("web01", "hello\n", ""); err == nil || *asked != 0 {
			t.Fatalf("err %v, asked %d", err, *asked)
		}
	})
}

func runSSHCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Cleanup(func() { _ = sshCmd.Flags().Set("print", "false") })
	var err error
	out := captureStdout(t, func() {
		sshCmd.SetOut(nil)
		if err = sshCmd.ParseFlags(args); err != nil {
			return
		}
		err = sshCmd.RunE(sshCmd, sshCmd.Flags().Args())
	})
	return out, err
}

func TestSSHPrintShowsTheCommand(t *testing.T) {
	s := newCommandVault(t, `{"username":"octo","password":"pw"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "web01", Type: "server"},
		`{"username":"root","password":"pw","host":"web01.example.com","port":"2222","private_key":"KEY"}`)

	out, err := runSSHCommand(t, "--print", "web01", "--", "-o", "ConnectTimeout=5", "uptime")
	if err != nil {
		t.Fatal(err)
	}
	if want := "ssh -p 2222 -l root web01.example.com -o ConnectTimeout=5 uptime\n"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	if strings.Contains(out, "KEY") || strings.Contains(out, "pw") {
		t.Errorf("secrets in the printed command: %q", out)
	}
}

func TestSSHOnlyTakesServerEntries(t *testing.T) {
	newCommandVault(t, `{"username":"octo","password":"pw"}`) // GitHub, a password entry
	_, err := runSSHCommand(t, "--print", "GitHub")
	var wrongType *wrongTypeError
	if !errors.As(err, &wrongType) {
		t.Fatalf("err = %v", err)
	}
}

func TestSSHNeedsAHost(t *testing.T) {
	s := newCommandVault(t, `{"username":"octo","password":"pw"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "web01", Type: "server"}, `{"username":"root"}`)
	if _, err := runSSHCommand(t, "--print", "web01"); err == nil || !strings.Contains(err.Error(), "no host") {
		t.Fatalf("err = %v", err)
	}
}

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
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

// runSSH runs ssh for the session with stdin, stdout and stderr of pwmgr and
// returns its exit status. The agent and askpass sockets live in a private
// temporary directory that is gone when runSSH returns.
func runSSH(s sshSession) (int, error) {
	dir, err := os.MkdirTemp("", "pwmgr-ssh-")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.Chmod(dir, 0o700); err != nil {
		return 0, err
	}

	var servers []*socketServer
	defer func() {
		for _, srv := range servers {
			srv.close()
		}
	}()

	env := os.Environ()
	identityAgent := ""
	if s.key != nil {
		srv, sock, err := startAgent(dir, s.key, s.keyComment)
		if err != nil {
			return 0, err
		}
		servers = append(servers, srv)
		identityAgent = sock
		env = setEnv(env, "SSH_AUTH_SOCK", sock)
	}
	if s.password != "" {
		self, err := os.Executable()
		if err != nil {
			return 0, fmt.Errorf("find pwmgr for SSH_ASKPASS: %w", err)
		}
		answerer := &askpassAnswerer{password: s.password, host: s.target.host}
		srv, sock, err := startAskpassServer(dir, answerer)
		if err != nil {
			return 0, err
		}
		servers = append(servers, srv)
		env = setEnv(env, "SSH_ASKPASS", self)
		env = setEnv(env, "SSH_ASKPASS_REQUIRE", "force")
		env = setEnv(env, askpassSocketEnv, sock)
	}

	if strings.ContainsAny(identityAgent, `"%$`) {
		return 0, fmt.Errorf("temporary directory %s has characters ssh cannot take in a path", dir)
	}
	cmd := exec.Command(s.sshPath, sshArgs(s.target, identityAgent, s.extra)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = env

	// Ctrl-C reaches ssh from the terminal directly; pwmgr only has to stay
	// alive to clean up. SIGTERM and SIGHUP are for pwmgr alone, so they are
	// passed on.
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(sigs)

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start ssh: %w", err)
	}
	done := make(chan struct{})
	var forwarding sync.WaitGroup
	forwarding.Add(1)
	go func() {
		defer forwarding.Done()
		for {
			select {
			case sig := <-sigs:
				if sig == syscall.SIGTERM || sig == syscall.SIGHUP {
					_ = cmd.Process.Signal(sig)
				}
			case <-done:
				return
			}
		}
	}()
	err = cmd.Wait()
	close(done)
	forwarding.Wait()

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal()), nil
		}
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return 0, fmt.Errorf("ssh: %w", err)
	}
	return 0, nil
}

// setEnv replaces or adds key in an environment list.
func setEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return append(out, key+"="+value)
}

// socketServer accepts connections on a Unix socket until closed.
type socketServer struct {
	ln    net.Listener
	conns sync.WaitGroup
	once  sync.Once
}

// listenUnix listens on dir/name, a socket only the user can reach.
func listenUnix(dir, name string, handle func(net.Conn)) (*socketServer, string, error) {
	path := filepath.Join(dir, name)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, "", err
	}
	srv := &socketServer{ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			srv.conns.Add(1)
			go func() {
				defer srv.conns.Done()
				defer func() { _ = conn.Close() }()
				handle(conn)
			}()
		}
	}()
	return srv, path, nil
}

// close stops accepting. Open agent connections end when ssh closes them,
// which it has done once it exited.
func (s *socketServer) close() {
	s.once.Do(func() { _ = s.ln.Close() })
}

// startAgent serves the key from an in-memory keyring.
func startAgent(dir string, key any, comment string) (*socketServer, string, error) {
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: key, Comment: comment}); err != nil {
		return nil, "", fmt.Errorf("load the private key: %w", err)
	}
	return listenUnix(dir, "agent.sock", func(conn net.Conn) {
		_ = agent.ServeAgent(keyring, conn)
	})
}

// startAskpassServer answers the askpass processes ssh starts.
func startAskpassServer(dir string, a *askpassAnswerer) (*socketServer, string, error) {
	return listenUnix(dir, "askpass.sock", func(conn net.Conn) {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
		_ = a.serve(conn)
	})
}

// runAskpass is pwmgr as ssh's askpass program: the parent answers what it
// can, everything else is asked on the terminal. The answer goes to stdout,
// where ssh reads it; a status other than 0 cancels the prompt.
func runAskpass(socket, prompt string) int {
	if answer, ok := askParent(socket, prompt); ok {
		fmt.Println(answer)
		return 0
	}
	answer, err := askTerminal(prompt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pwmgr: %v\n", err)
		return 1
	}
	fmt.Println(answer)
	return 0
}

// askTerminal asks on the controlling terminal, without echo for anything
// that looks like a secret.
func askTerminal(prompt string) (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("no terminal to ask %q: %w", strings.TrimSpace(prompt), err)
	}
	defer func() { _ = tty.Close() }()
	if _, err := fmt.Fprint(tty, prompt); err != nil {
		return "", err
	}
	if isHiddenPrompt(prompt) {
		b, err := term.ReadPassword(int(tty.Fd()))
		_, _ = fmt.Fprintln(tty)
		return string(b), err
	}
	line, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

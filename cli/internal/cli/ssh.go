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
	"bufio"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

// pwmgr ssh starts the system's ssh with the login of a server entry. A private
// key goes to an agent that lives inside this process for as long as ssh runs,
// so it never touches the disk and never enters the user's own agent. A stored
// password reaches ssh through SSH_ASKPASS: ssh runs pwmgr again as its askpass
// program, and that process asks this one over a Unix socket.

// askpassSocketEnv points the askpass process at the parent's socket.
const askpassSocketEnv = "PWMGR_ASKPASS_SOCKET"

func init() {
	rootCmd.AddCommand(sshCmd)
	sshCmd.Flags().Bool("print", false, "Print the ssh command instead of running it")
}

var sshCmd = &cobra.Command{
	Use:   "ssh <entry> [-- ssh arguments]",
	Short: "Connect to a server entry with ssh",
	Long: `Connect to a server entry with the system's ssh.

The entry's private key is served by an agent inside pwmgr for as long as ssh
runs: it never touches the disk and never enters your own ssh-agent. A stored
password is answered once, for ssh's password prompt; everything else ssh asks
(host keys, one-time codes, other passphrases) comes to your terminal.

Everything after -- goes to ssh after the host: options first, then a remote
command.

Examples:
  pwmgr ssh web01
  pwmgr ssh web01 -- uptime
  pwmgr ssh web01 -- -L 8080:localhost:80 -N
  pwmgr ssh web01 --print`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printOnly, _ := cmd.Flags().GetBool("print")
		if err := ensureUnlocked(); err != nil {
			return err
		}
		match, err := resolveEntry(args[0], resolveOptions{entryType: "server"})
		if err != nil {
			return err
		}
		entry, err := v.GetEntry(match.ID)
		if err != nil {
			return err
		}
		if entry.Data == nil {
			return fmt.Errorf("entry '%s' has no data", entry.Name)
		}
		target, err := sshTargetOf(entry.Data)
		if err != nil {
			return fmt.Errorf("entry '%s': %w", entry.Name, err)
		}
		extra := args[1:]

		if printOnly {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), shellJoin(append([]string{"ssh"}, sshArgs(target, "", extra)...)))
			return nil
		}

		sshPath, err := lookPath("ssh")
		if err != nil {
			return fmt.Errorf("ssh not found in PATH: %w", err)
		}
		session := sshSession{sshPath: sshPath, target: target, extra: extra, password: entry.Data.Password}
		if entry.Data.PrivateKey != "" {
			if session.key, err = parseServerKey(entry.Name, entry.Data.PrivateKey, entry.Data.KeyPassphrase); err != nil {
				return err
			}
			session.keyComment = "pwmgr: " + entry.Name
		}
		code, err := runSSH(session)
		if err != nil {
			return err
		}
		if code != 0 {
			// ssh has said what went wrong; pwmgr only passes the status on.
			cmd.SilenceErrors = true
			return &exitStatusError{code: code}
		}
		return nil
	},
}

// lookPath finds ssh. A variable so tests can stand in their own.
var lookPath = exec.LookPath

// promptKeyPassphrase asks for a key's passphrase the entry does not store.
// A variable so tests do not need a terminal.
var promptKeyPassphrase = promptPassword

// sshTarget is where ssh connects to.
type sshTarget struct {
	host, port, user string
}

// sshTargetOf reads the target of a server entry and refuses values ssh would
// take for options.
func sshTargetOf(d *vault.EntryData) (sshTarget, error) {
	host, err := normaliseHost(d.Host)
	if err != nil {
		if strings.TrimSpace(d.Host) == "" {
			return sshTarget{}, errors.New("no host stored")
		}
		return sshTarget{}, err
	}
	port, err := normalisePort(d.Port)
	if err != nil {
		return sshTarget{}, err
	}
	if strings.HasPrefix(d.Username, "-") {
		return sshTarget{}, fmt.Errorf("username %q must not start with '-'", d.Username)
	}
	return sshTarget{host: host, port: port, user: d.Username}, nil
}

// sshArgs builds ssh's arguments: ssh -p <port> [-l user] [-o IdentityAgent=…]
// host [extra…]. ssh reads options after the host as well, so extra may hold
// options and then a remote command.
func sshArgs(t sshTarget, identityAgent string, extra []string) []string {
	var args []string
	if t.port != "" {
		args = append(args, "-p", t.port)
	}
	if t.user != "" {
		args = append(args, "-l", t.user)
	}
	if identityAgent != "" {
		// Quoted, as ssh splits option values on spaces.
		args = append(args, "-o", `IdentityAgent="`+identityAgent+`"`)
	}
	args = append(args, t.host)
	return append(args, extra...)
}

// parseServerKey opens the entry's private key: unencrypted, with the stored
// passphrase, or with one asked for when none is stored or the stored one is
// wrong.
func parseServerKey(entryName, key, passphrase string) (any, error) {
	raw, err := ssh.ParseRawPrivateKey([]byte(key))
	if err == nil {
		return raw, nil
	}
	var missing *ssh.PassphraseMissingError
	if !errors.As(err, &missing) {
		return nil, fmt.Errorf("entry '%s': cannot read the private key: %w", entryName, err)
	}
	if passphrase != "" {
		raw, err = ssh.ParseRawPrivateKeyWithPassphrase([]byte(key), []byte(passphrase))
		if err == nil {
			return raw, nil
		}
		if !errors.Is(err, x509.IncorrectPasswordError) {
			return nil, fmt.Errorf("entry '%s': cannot read the private key: %w", entryName, err)
		}
		fmt.Fprintf(os.Stderr, "The stored passphrase does not open the key of '%s'.\n", entryName)
	}
	asked, err := promptKeyPassphrase(fmt.Sprintf("Passphrase for the key of '%s': ", entryName))
	if err != nil {
		return nil, err
	}
	raw, err = ssh.ParseRawPrivateKeyWithPassphrase([]byte(key), []byte(asked))
	if err != nil {
		return nil, fmt.Errorf("entry '%s': cannot open the private key: %w", entryName, err)
	}
	return raw, nil
}

// sshSession is one ssh run.
type sshSession struct {
	sshPath    string
	target     sshTarget
	extra      []string
	key        any // parsed private key; nil = none
	keyComment string
	password   string // "" = none, then ssh asks itself
}

// exitStatusError carries ssh's exit status to the process exit.
type exitStatusError struct{ code int }

func (e *exitStatusError) Error() string { return fmt.Sprintf("ssh exited with status %d", e.code) }

// ExitCode is the process exit status for an error Execute returned.
func ExitCode(err error) int {
	var status *exitStatusError
	if errors.As(err, &status) {
		return status.code
	}
	return 1
}

// RunAskpassIfRequested runs pwmgr as ssh's askpass program when ssh started it
// as one: the socket variable is set and the only argument is a prompt, not a
// pwmgr command. main calls it before anything else.
func RunAskpassIfRequested() (code int, ran bool) {
	if !isAskpassInvocation(os.Getenv(askpassSocketEnv), os.Args) {
		return 0, false
	}
	return runAskpass(os.Getenv(askpassSocketEnv), os.Args[1]), true
}

// isAskpassInvocation tells an askpass call from a pwmgr command run by
// something ssh started (a ProxyCommand inherits the variable, too).
func isAskpassInvocation(socket string, args []string) bool {
	if socket == "" || len(args) != 2 || strings.HasPrefix(args[1], "-") {
		return false
	}
	for _, c := range rootCmd.Commands() {
		if c.Name() == args[1] || c.HasAlias(args[1]) {
			return false
		}
	}
	return args[1] != "help" && args[1] != "completion"
}

// isPasswordPrompt is a prompt the stored password answers: one that ends in
// "password:", like "root@web01's password:" or "(root@web01) Password:".
func isPasswordPrompt(prompt string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(prompt)), "password:")
}

var (
	promptUserAtHost = regexp.MustCompile(`[^\s@()'"]+@([^\s@()'":]+)`)
	promptSecretWord = regexp.MustCompile(`(?i)password|passphrase|passcode|code|\bpin\b|\botp\b|token`)
)

// promptNamesOtherHost reports whether a prompt names user@host for a host
// other than the target: a ProxyJump host asks through the same askpass, and
// the stored password is not for it.
func promptNamesOtherHost(prompt, host string) bool {
	matches := promptUserAtHost.FindAllStringSubmatch(prompt, -1)
	if len(matches) == 0 {
		return false
	}
	want := strings.ToLower(strings.Trim(host, "[]"))
	for _, m := range matches {
		if strings.ToLower(strings.Trim(m[1], "[]")) == want {
			return false
		}
	}
	return true
}

// isHiddenPrompt is a prompt whose answer must not echo on the terminal.
func isHiddenPrompt(prompt string) bool {
	return promptSecretWord.MatchString(prompt)
}

// The askpass protocol: the askpass process sends one JSON line with the
// prompt, the parent answers one JSON line. Answered false means: ask the
// user on the terminal.
type askpassRequest struct {
	Prompt string `json:"prompt"`
}

type askpassReply struct {
	Answered bool   `json:"answered"`
	Answer   string `json:"answer,omitempty"`
}

// maxAskpassMessage bounds what either side reads.
const maxAskpassMessage = 64 << 10

// askpassAnswerer hands out the stored password once, for a password prompt of
// the target host. A second password prompt means it was wrong, and the user
// gets asked.
type askpassAnswerer struct {
	mu       sync.Mutex
	password string
	host     string
	used     bool
}

func (a *askpassAnswerer) answer(prompt string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.used || a.password == "" || !isPasswordPrompt(prompt) || promptNamesOtherHost(prompt, a.host) {
		return "", false
	}
	a.used = true
	return a.password, true
}

// serveAskpass answers one askpass request on conn.
func (a *askpassAnswerer) serve(conn io.ReadWriter) error {
	line, err := bufio.NewReader(io.LimitReader(conn, maxAskpassMessage)).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	var req askpassRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return fmt.Errorf("askpass request: %w", err)
	}
	var reply askpassReply
	reply.Answer, reply.Answered = a.answer(req.Prompt)
	b, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	_, err = conn.Write(append(b, '\n'))
	return err
}

// askParent asks the pwmgr that started ssh. Any failure means asking the user.
func askParent(socket, prompt string) (string, bool) {
	conn, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		return "", false
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	reply, err := queryAskpass(conn, prompt)
	if err != nil || !reply.Answered {
		return "", false
	}
	return reply.Answer, true
}

// queryAskpass asks the parent on conn about prompt.
func queryAskpass(conn io.ReadWriter, prompt string) (askpassReply, error) {
	b, err := json.Marshal(askpassRequest{Prompt: prompt})
	if err != nil {
		return askpassReply{}, err
	}
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return askpassReply{}, err
	}
	line, err := bufio.NewReader(io.LimitReader(conn, maxAskpassMessage)).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return askpassReply{}, err
	}
	var reply askpassReply
	if err := json.Unmarshal(line, &reply); err != nil {
		return askpassReply{}, fmt.Errorf("askpass reply: %w", err)
	}
	return reply, nil
}

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
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/Gerry3010/passbubble/cli/internal/vault"
	"golang.org/x/crypto/ssh"
)

// A server entry holds an SSH login: host, port, username, and a password, a
// private key (with its passphrase) or both.

// serverFlags are the values `pwmgr add --type server` takes from flags
// instead of asking.
type serverFlags struct {
	host, port, keyFile string
	username            string
	hasUsername         bool // the username came as an argument, "" included
}

// prompter asks for one value at a time. Visible answers come from in, hidden
// ones from secret, which reads the terminal without echo.
type prompter struct {
	in     *bufio.Reader
	out    io.Writer
	secret func(prompt string) (string, error)
}

// line asks for a visible value and returns it without the line ending. The
// last line of input counts even without one.
func (p prompter) line(prompt string) (string, error) {
	if prompt != "" {
		_, _ = fmt.Fprint(p.out, prompt)
	}
	s, err := p.in.ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || s == "") {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}

// collectServerData asks for what a server entry holds, or takes it from the
// flags. Host is required; everything else may stay empty.
func collectServerData(p prompter, flags serverFlags) (*vault.EntryData, error) {
	data := &vault.EntryData{}

	if flags.host != "" {
		host, err := normaliseHost(flags.host)
		if err != nil {
			return nil, err
		}
		data.Host = host
	} else {
		for data.Host == "" {
			in, err := p.line("Host: ")
			if err != nil {
				return nil, err
			}
			host, err := normaliseHost(in)
			if err != nil {
				_, _ = fmt.Fprintf(p.out, "  %v\n", err)
				continue
			}
			data.Host = host
		}
	}

	if flags.port != "" {
		port, err := normalisePort(flags.port)
		if err != nil {
			return nil, err
		}
		data.Port = port
	} else {
		for {
			in, err := p.line("Port [22]: ")
			if err != nil {
				return nil, err
			}
			port, err := normalisePort(in)
			if err != nil {
				_, _ = fmt.Fprintf(p.out, "  %v\n", err)
				continue
			}
			data.Port = port
			break
		}
	}

	if flags.hasUsername {
		data.Username = flags.username
	} else {
		in, err := p.line("Username (leave empty for ssh's default): ")
		if err != nil {
			return nil, err
		}
		data.Username = strings.TrimSpace(in)
	}
	if strings.HasPrefix(data.Username, "-") {
		return nil, fmt.Errorf("username %q must not start with '-'", data.Username)
	}

	password, err := p.secret("Password (leave empty for none): ")
	if err != nil {
		return nil, err
	}
	data.Password = password

	if flags.keyFile != "" {
		key, err := os.ReadFile(flags.keyFile)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		if strings.TrimSpace(string(key)) == "" {
			return nil, fmt.Errorf("key file %s is empty", flags.keyFile)
		}
		data.PrivateKey = string(key)
	} else if data.PrivateKey, err = pastePrivateKey(p); err != nil {
		return nil, err
	}

	if data.PrivateKey != "" {
		if data.KeyPassphrase, err = askKeyPassphrase(p, data.PrivateKey); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// pastePrivateKey reads a pasted key up to a line holding only END. An empty
// first line means no key. The key is kept as pasted, with a final newline:
// OpenSSH refuses keys without one.
func pastePrivateKey(p prompter) (string, error) {
	_, _ = fmt.Fprintln(p.out, "Private key: paste it, then a line with only END (leave the first line empty for none):")
	var lines []string
	for {
		line, err := p.line("")
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if len(lines) == 0 && strings.TrimSpace(line) == "" {
			return "", nil
		}
		if strings.TrimSpace(line) == "END" {
			break
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "", nil
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// askKeyPassphrase asks for the passphrase of a key that needs one and checks
// it opens the key. An unencrypted key needs none, so nothing is asked. A key
// pwmgr cannot read is saved as it is, with whatever passphrase comes.
func askKeyPassphrase(p prompter, key string) (string, error) {
	_, err := ssh.ParseRawPrivateKey([]byte(key))
	if err == nil {
		return "", nil
	}
	var missing *ssh.PassphraseMissingError
	encrypted := errors.As(err, &missing)
	if !encrypted {
		_, _ = fmt.Fprintf(p.out, "Warning: pwmgr ssh cannot read this key (%v). It is saved as it is.\n", err)
	}
	for {
		pass, err := p.secret("Key passphrase (leave empty to be asked when connecting): ")
		if err != nil || pass == "" || !encrypted {
			return pass, err
		}
		_, err = ssh.ParseRawPrivateKeyWithPassphrase([]byte(key), []byte(pass))
		if err == nil {
			return pass, nil
		}
		if !errors.Is(err, x509.IncorrectPasswordError) {
			_, _ = fmt.Fprintf(p.out, "Warning: the key could not be opened (%v). The passphrase is saved anyway.\n", err)
			return pass, nil
		}
		_, _ = fmt.Fprintln(p.out, "  That passphrase does not open the key.")
	}
}

// normaliseHost trims a host and refuses what ssh would read as something
// else: an option (leading '-'), a user (an '@') or several arguments.
func normaliseHost(s string) (string, error) {
	host := strings.TrimSpace(s)
	switch {
	case host == "":
		return "", errors.New("host is required")
	case strings.HasPrefix(host, "-"):
		return "", fmt.Errorf("host %q must not start with '-'", host)
	case strings.Contains(host, "@"):
		return "", fmt.Errorf("host %q contains '@': put the user into Username", host)
	case strings.IndexFunc(host, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0:
		return "", fmt.Errorf("host %q must not contain spaces", host)
	}
	return host, nil
}

// normalisePort checks a port; "" stays "" (ssh's default, 22).
func normalisePort(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 || strings.HasPrefix(s, "+") {
		return "", fmt.Errorf("port %q is not a number from 1 to 65535", s)
	}
	return strconv.Itoa(n), nil
}

// sshCommandLine is the ssh command `pwmgr get` shows for a server entry:
// ssh [-p PORT] [user@]host, without -p for the default port.
func sshCommandLine(d *vault.EntryData) string {
	args := []string{"ssh"}
	if d.Port != "" && d.Port != "22" {
		args = append(args, "-p", d.Port)
	}
	target := d.Host
	if d.Username != "" {
		target = d.Username + "@" + d.Host
	}
	return shellJoin(append(args, target))
}

// shellJoin quotes arguments for a POSIX shell where needed, so a printed
// command can be pasted as it is.
func shellJoin(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = shellQuote(a)
	}
	return strings.Join(out, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := func(r rune) bool {
		return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("@%+=:,./-_", r))
	}
	if strings.IndexFunc(s, func(r rune) bool { return !safe(r) }) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

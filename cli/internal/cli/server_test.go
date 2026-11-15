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
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/vault"
	"golang.org/x/crypto/ssh"
)

// Encrypting a key with a passphrase runs bcrypt, which is slow under -race,
// so the tests share one.
var (
	encryptedKeyOnce sync.Once
	encryptedKeyPEM  string
	encryptedKeyPub  ssh.PublicKey
)

// sharedEncryptedKey is an ed25519 key encrypted with "s3cret".
func sharedEncryptedKey(t *testing.T) (string, ssh.PublicKey) {
	t.Helper()
	encryptedKeyOnce.Do(func() { encryptedKeyPEM, encryptedKeyPub = testSSHKey(t, "s3cret") })
	return encryptedKeyPEM, encryptedKeyPub
}

// scriptedPrompter answers visible prompts from input and hidden ones from
// secrets, in order, and records what it was asked.
func scriptedPrompter(input string, secrets ...string) (prompter, *bytes.Buffer, *[]string) {
	var out bytes.Buffer
	var asked []string
	return prompter{
		in:  bufio.NewReader(strings.NewReader(input)),
		out: &out,
		secret: func(prompt string) (string, error) {
			asked = append(asked, prompt)
			if len(secrets) == 0 {
				return "", nil
			}
			s := secrets[0]
			secrets = secrets[1:]
			return s, nil
		},
	}, &out, &asked
}

func TestCollectServerDataAsksForEverything(t *testing.T) {
	key, _ := testSSHKey(t, "")
	input := "-bad\n\nweb01.example.com\n99999\n\nroot\n" + key + "END\n"
	p, out, asked := scriptedPrompter(input, "hunter2")

	data, err := collectServerData(p, serverFlags{})
	if err != nil {
		t.Fatal(err)
	}
	want := &vault.EntryData{Host: "web01.example.com", Username: "root", Password: "hunter2", PrivateKey: key}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("data\n got: %+v\nwant: %+v", data, want)
	}
	// An unencrypted key needs no passphrase.
	if len(*asked) != 1 {
		t.Errorf("hidden prompts: %q", *asked)
	}
	for _, msg := range []string{"must not start with '-'", "host is required", "not a number from 1 to 65535"} {
		if !strings.Contains(out.String(), msg) {
			t.Errorf("the prompt should explain %q:\n%s", msg, out.String())
		}
	}
}

func TestCollectServerDataKeepsThePastedKeyVerbatim(t *testing.T) {
	// Lines as pasted, CRLF endings dropped, one newline at the end.
	p, _, _ := scriptedPrompter("h\n2222\nu\nline one\r\n  indented  \nlast\nEND\n")
	data, err := collectServerData(p, serverFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if data.PrivateKey != "line one\n  indented  \nlast\n" || data.Port != "2222" {
		t.Fatalf("data = %+v", data)
	}
}

func TestCollectServerDataWithoutKeyAsksNoPassphrase(t *testing.T) {
	p, _, asked := scriptedPrompter("h\n\nu\n\n", "")
	data, err := collectServerData(p, serverFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if data.PrivateKey != "" || data.KeyPassphrase != "" || data.Password != "" || data.Port != "" {
		t.Fatalf("data = %+v", data)
	}
	if len(*asked) != 1 {
		t.Errorf("only the password should be asked: %q", *asked)
	}
}

func TestCollectServerDataChecksThePassphrase(t *testing.T) {
	key, _ := sharedEncryptedKey(t)
	p, out, asked := scriptedPrompter("h\n\nu\n"+key+"END\n", "", "wrong", "s3cret")
	data, err := collectServerData(p, serverFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if data.KeyPassphrase != "s3cret" || data.PrivateKey != key {
		t.Fatalf("data = %+v", data)
	}
	if len(*asked) != 3 || !strings.Contains(out.String(), "does not open the key") {
		t.Errorf("asked %q\n%s", *asked, out.String())
	}

	// Empty: the passphrase is asked when connecting.
	p, _, _ = scriptedPrompter("h\n\nu\n"+key+"END\n", "", "")
	if data, err = collectServerData(p, serverFlags{}); err != nil || data.KeyPassphrase != "" {
		t.Fatalf("data = %+v, err %v", data, err)
	}
}

func TestCollectServerDataTakesFlags(t *testing.T) {
	key, _ := sharedEncryptedKey(t)
	keyFile := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyFile, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	p, _, asked := scriptedPrompter("", "pw", "s3cret")
	data, err := collectServerData(p, serverFlags{host: "web01", port: "2222", keyFile: keyFile, hasUsername: true})
	if err != nil {
		t.Fatal(err)
	}
	want := &vault.EntryData{Host: "web01", Port: "2222", Password: "pw", PrivateKey: key, KeyPassphrase: "s3cret"}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("data\n got: %+v\nwant: %+v", data, want)
	}
	if len(*asked) != 2 {
		t.Errorf("hidden prompts: %q", *asked)
	}

	for name, flags := range map[string]serverFlags{
		"bad host":       {host: "-oProxyCommand=x"},
		"bad port":       {host: "h", port: "0"},
		"missing key":    {host: "h", port: "22", hasUsername: true, keyFile: filepath.Join(t.TempDir(), "none")},
		"option as user": {host: "h", port: "22", hasUsername: true, username: "-l"},
	} {
		t.Run(name, func(t *testing.T) {
			p, _, _ := scriptedPrompter("")
			if _, err := collectServerData(p, flags); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestCollectServerDataStopsAtEndOfInput(t *testing.T) {
	p, _, _ := scriptedPrompter("")
	if _, err := collectServerData(p, serverFlags{}); err == nil {
		t.Fatal("no host, no entry")
	}
}

// decryptPost reads back an entry the command created.
func decryptPost(t *testing.T, s *commandVault, i int) map[string]any {
	t.Helper()
	post := s.posts[i]
	s.entries = append(s.entries, apiclient.EntryResponse{
		ID: "created", Name: post.Name, Type: post.Type, EncryptedData: post.EncryptedData, EntryKey: &post.EntryKeys[0],
	})
	created, err := v.GetEntry("created")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(created.Data)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAddServerEntry(t *testing.T) {
	resetFlags(t, addCmd)
	s := newCommandVault(t, `{"password":"pw"}`)
	orig := promptSecret
	promptSecret = func(string) (string, error) { return "hunter2", nil }
	t.Cleanup(func() { promptSecret = orig })
	for flag, value := range map[string]string{"type": "server", "host": "web01.example.com", "port": "2222", "notes": "rack 4"} {
		if err := addCmd.Flags().Set(flag, value); err != nil {
			t.Fatal(err)
		}
	}
	withStdin(t, "\n") // no key

	captureStdout(t, func() {
		if err := addCmd.RunE(addCmd, []string{"web01", "root"}); err != nil {
			t.Error(err)
		}
	})
	if len(s.posts) != 1 || s.posts[0].Type != "server" || s.posts[0].Name != "web01" {
		t.Fatalf("posts = %+v", s.posts)
	}
	want := map[string]any{"host": "web01.example.com", "port": "2222", "username": "root", "password": "hunter2", "notes": "rack 4"}
	if got := decryptPost(t, s, 0); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved\n got: %v\nwant: %v", got, want)
	}
}

func TestServerFlagsNeedTheServerType(t *testing.T) {
	resetFlags(t, addCmd)
	out, err := runRoot(t, "add", "x", "--host", "web01")
	if err == nil || !strings.Contains(err.Error(), "--type server") || !strings.Contains(out, "Usage:") {
		t.Fatalf("err = %v\n%s", err, out)
	}
}

func TestGetPrintsServerAndPasskeys(t *testing.T) {
	s := newCommandVault(t, `{"username":"alice@example.com","passkeys":[
		{"credential_id":"AQID","rp_id":"example.com","user_handle":"BAUG","user_name":"alice@example.com",
		 "key_algorithm":-7,"private_key":"PASSKEY-PRIVATE","sign_count":0,"discoverable":true,
		 "prf_secret":"PASSKEY-PRF","created_at":"2026-11-14T03:00:00Z"},
		{"credential_id":"BAUG","rp_id":"login.example.org","user_handle":"","user_name":"","key_algorithm":-7,
		 "private_key":"PASSKEY-PRIVATE-2","sign_count":0,"discoverable":true,"created_at":""}]}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "web01", Type: "server"},
		`{"username":"root","password":"hunter2","host":"web01.example.com","port":"2222",
		  "private_key":"-----BEGIN OPENSSH PRIVATE KEY-----\nb3Bl\n-----END OPENSSH PRIVATE KEY-----\n","key_passphrase":"s3cret"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e3", Name: "plain", Type: "server"}, `{"host":"10.0.0.1","port":"22"}`)

	get := func(id string) string {
		return captureStdout(t, func() {
			if err := getCmd.RunE(getCmd, []string{id}); err != nil {
				t.Error(err)
			}
		})
	}

	out := get("e2")
	for _, want := range []string{
		"Host:          web01.example.com\n",
		"Port:          2222\n",
		"Username:      root\n",
		"Password:      hunter2\n",
		"Key passphrase: s3cret\n",
		"SSH:           ssh -p 2222 root@web01.example.com\n",
		"Private key:\n-----BEGIN OPENSSH PRIVATE KEY-----\nb3Bl\n-----END OPENSSH PRIVATE KEY-----\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "Passkeys") {
		t.Errorf("no passkeys here:\n%s", out)
	}

	if out := get("e3"); !strings.Contains(out, "SSH:           ssh 10.0.0.1\n") {
		t.Errorf("port 22 and no user: plain ssh host\n%s", out)
	}

	out = get(commandEntryID)
	if !strings.Contains(out, "Passkeys:\n  example.com        alice@example.com  2026-11-14T03:00:00Z\n  login.example.org  -                  -\n") {
		t.Errorf("passkey list:\n%s", out)
	}
	if strings.Contains(out, "PASSKEY-PRIVATE") || strings.Contains(out, "PASSKEY-PRF") || strings.Contains(out, "SSH:") {
		t.Errorf("passkey secrets or an ssh line on screen:\n%s", out)
	}
}

// update rewrites the whole entry: server fields, passkeys and the unknown
// keys inside them have to survive it.
func TestUpdateKeepsServerFieldsAndPasskeys(t *testing.T) {
	resetFlags(t, updateCmd)
	const stored = `{"username":"root","password":"old","host":"web01","port":"2222","private_key":"KEY\n",
		"key_passphrase":"pp","passkeys":[{"credential_id":"AQID","rp_id":"a.example","user_handle":"BAUG",
		"user_name":"root","key_algorithm":-7,"private_key":"MIGH","sign_count":0,"discoverable":true,
		"prf_secret":"prf","created_at":"2026-01-01T00:00:00Z","future_pk":{"x":[1]}}],"future":true}`
	s := newCommandVault(t, stored)
	s.entries[0].Type = "server"
	if err := updateCmd.Flags().Set("notes", "moved to rack 5"); err != nil {
		t.Fatal(err)
	}
	captureStdout(t, func() {
		if err := updateCmd.RunE(updateCmd, []string{commandEntryID}); err != nil {
			t.Error(err)
		}
	})
	var want map[string]any
	if err := json.Unmarshal([]byte(stored), &want); err != nil {
		t.Fatal(err)
	}
	want["notes"] = "moved to rack 5"
	if got := s.saved(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved\n got: %v\nwant: %v", got, want)
	}
}

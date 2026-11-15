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

package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/config"
)

const serverStored = `{
	"username": "root",
	"password": "hunter2",
	"host": "web01.example.com",
	"port": "2222",
	"private_key": "-----BEGIN OPENSSH PRIVATE KEY-----\nb3Bl\n-----END OPENSSH PRIVATE KEY-----\n",
	"key_passphrase": "s3cret",
	"notes": "rack 4",
	"future_flag": {"nested": true}
}`

// A login with passkeys, one of them with a key only a newer client knows.
const passkeyStored = `{
	"username": "alice@example.com",
	"password": "old-password",
	"passkeys": [
		{"credential_id": "q2D0bQ2u3E7XoWlS1yZbVw", "rp_id": "example.com", "rp_name": "Example",
		 "user_handle": "dXNlci0xMjM0", "user_name": "alice@example.com", "user_display_name": "Alice",
		 "key_algorithm": -7, "private_key": "MIGHAgEA", "sign_count": 0, "discoverable": true,
		 "prf_secret": "3q2-7w", "created_at": "2026-11-14T03:00:00Z", "future_pk": {"backup": [1, 2]}},
		{"credential_id": "AQID", "rp_id": "b.example", "user_handle": "", "user_name": "alice",
		 "key_algorithm": -7, "private_key": "MIGH", "sign_count": 0, "discoverable": false, "created_at": ""}
	]
}`

func TestTypeIcons(t *testing.T) {
	m, _ := newTestModel(t, &config.Config{})
	for typ, want := range map[string]string{"server": "[S]", "ssh-key": "[ssh]", "password": "[pw]", "nope": "[?]"} {
		if got := m.getTypeIcon(typ); got != want {
			t.Errorf("getTypeIcon(%q) = %q, want %q", typ, got, want)
		}
	}
}

func TestDetailShowsWhereAServerEntryConnects(t *testing.T) {
	s := newListServer(t, listEntry{"e1", "web01", "server", serverStored})
	m := s.listModel(t)

	m = press(t, m, "enter")
	if m.detailServer == nil {
		t.Fatalf("the server data was not loaded (status %q)", m.status)
	}
	screen := m.renderDetailScreen()
	for _, want := range []string{"[S] server", "Host: web01.example.com", "Port: 2222", "User: root", "Private key: stored", "Password:", "u: copy user"} {
		if !strings.Contains(screen, want) {
			t.Errorf("missing %q:\n%s", want, screen)
		}
	}
	for _, secret := range []string{"hunter2", "s3cret", "b3Bl"} {
		if strings.Contains(screen, secret) {
			t.Errorf("%q on screen before reveal:\n%s", secret, screen)
		}
	}

	m = press(t, m, "s")
	if screen := m.renderDetailScreen(); !strings.Contains(screen, "hunter2") || strings.Contains(screen, "b3Bl") {
		t.Errorf("s reveals the password, never the key:\n%s", screen)
	}

	// Not run: that would write to the real clipboard.
	if _, cmd := m.Update(key("c")); cmd == nil {
		t.Error("c should copy the password")
	}
	if _, cmd := m.Update(key("u")); cmd == nil {
		t.Error("u should copy the username")
	}
}

func TestDetailOfAServerWithoutPortUserOrPassword(t *testing.T) {
	s := newListServer(t, listEntry{"e1", "box", "server", `{"host":"10.0.0.1"}`})
	m := s.listModel(t)

	m = press(t, m, "enter")
	screen := m.renderDetailScreen()
	for _, want := range []string{"Host: 10.0.0.1", "Port: 22", "(default)", "User:", "(none)"} {
		if !strings.Contains(screen, want) {
			t.Errorf("missing %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "Password") || strings.Contains(screen, "Private key") {
		t.Errorf("nothing stored, nothing shown:\n%s", screen)
	}
	for _, k := range []string{"c", "u"} {
		next, cmd := m.Update(key(k))
		if cmd != nil || next.(Model).statusType != "info" {
			t.Errorf("%s has nothing to copy: status %q", k, next.(Model).status)
		}
	}
}

func TestLockForgetsTheServerDetail(t *testing.T) {
	s := newListServer(t, listEntry{"e1", "web01", "server", serverStored})
	m := press(t, s.listModel(t), "enter")
	if m.detailServer == nil {
		t.Fatal("not loaded")
	}
	if m = m.lockVault(); m.detailServer != nil {
		t.Fatal("a locked vault must not keep the server detail")
	}
}

// The edit form is the generic one: username and password. Host, port, key,
// passphrase and everything else stay as they are.
func TestEditOfAServerEntryKeepsEveryField(t *testing.T) {
	s := newEditServer(t, "server", serverStored)
	m := openEdit(t, s.editModel(t))
	if got := m.form.Fields[1].Value; got != "root" {
		t.Errorf("username field = %q", got)
	}
	m = send(t, m, "tab")
	m = clearField(t, m)
	m = typeText(t, m, "admin")
	m = send(t, m, "tab")
	m = typeText(t, m, "new-password")
	submit(t, m)

	want := withField(t, serverStored, "username", "admin")
	want.(map[string]any)["password"] = "new-password"
	if got := s.savedData(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved\n got: %v\nwant: %v", got, want)
	}
}

func TestEditFromTheServerDetailFollowsTheUser(t *testing.T) {
	s := newEditServer(t, "server", serverStored)
	m := s.editModel(t)
	next, cmd := m.openDetail(m.allEntries[0])
	m = drain(t, next.(Model), cmd)

	next, cmd = m.Update(key("e"))
	m = drain(t, next.(Model), cmd)
	m = send(t, m, "tab")
	m = clearField(t, m)
	m = typeText(t, m, "admin")
	m = submit(t, m)

	if m.detailServer == nil || m.detailServer.User != "admin" || m.detailServer.Host != "web01.example.com" {
		t.Fatalf("detail = %+v", m.detailServer)
	}
	if screen := m.renderDetailScreen(); !strings.Contains(screen, "User: admin") || strings.Contains(screen, "Username:") {
		t.Errorf("screen:\n%s", screen)
	}
}

func TestEditKeepsPasskeys(t *testing.T) {
	s := newEditServer(t, "password", passkeyStored)
	m := openEdit(t, s.editModel(t))
	m = send(t, m, "tab", "tab")
	m = typeText(t, m, "new-password")
	submit(t, m)

	want := withField(t, passkeyStored, "password", "new-password")
	if got := s.savedData(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("saved\n got: %v\nwant: %v", got, want)
	}
}

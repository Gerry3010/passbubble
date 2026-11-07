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
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/pkg/backup"
)

// Everything the weak backup used to lose: a TOTP entry's password, custom fields,
// tags, card data and keys only other clients know.
const backupTestStored = `{
	"username": "octo",
	"password": "hunter2",
	"totp_secret": "JBSWY3DPEHPK3PXP",
	"card_number": "4111",
	"tags": ["work"],
	"future_flag": {"nested": [1, 2]},
	"custom_fields": [{"label": "PIN", "value": "1234", "type": "password", "future_cf": true}]
}`

// backupServer serves two same-named entries, one of them full, with HOME pointing
// at a temp dir: nothing here may reach the real ~/Documents.
func backupServer(t *testing.T) (*listServer, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dir, err := backup.DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	s := newListServer(t,
		listEntry{"e1", "AWS", "totp", backupTestStored},
		listEntry{"e2", "AWS", "totp", backupTestStored},
	)
	s.entries[0].URL = "https://aws.example"
	return s, dir
}

func createBackup(t *testing.T, s *listServer, name string) Model {
	t.Helper()
	m := press(t, s.listModel(t), "c")
	if !m.showingForm || m.form.Type != CreateBackupForm {
		t.Fatalf("c should open the backup form (status %q)", m.status)
	}
	m = typeText(t, m, name)
	return press(t, m, "enter")
}

func onlyBackup(t *testing.T, dir string) backup.Info {
	t.Helper()
	backups, err := backup.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("want 1 backup in %s, got %v", dir, backups)
	}
	return backups[0]
}

func canonData(t *testing.T, v any) any {
	t.Helper()
	blob, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(blob, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCreateBackupWritesTheWholeVaultWherePwmgrBackupDoes(t *testing.T) {
	s, dir := backupServer(t)
	m := createBackup(t, s, "")

	if m.statusType != "success" || !strings.Contains(m.status, dir) {
		t.Fatalf("status = %q (%s)", m.status, m.statusType)
	}
	info := onlyBackup(t, dir)
	stat, err := os.Stat(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", stat.Mode())
	}
	if !strings.Contains(m.status, info.Path) {
		t.Errorf("status should name the file: %q", m.status)
	}

	file, err := backup.Read(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Server != s.v.Config().ServerURL {
		t.Errorf("server = %q", file.Server)
	}
	if len(file.Entries) != 2 {
		t.Fatalf("both same-named entries belong in the file, got %d", len(file.Entries))
	}
	first := file.Entries[0]
	if first.Name != "AWS" || first.Type != "totp" || first.URL != "https://aws.example" {
		t.Errorf("metadata = %+v", first)
	}
	if got, want := canonData(t, first.Data), canonJSON(t, backupTestStored); !reflect.DeepEqual(got, want) {
		t.Fatalf("backed up\n got: %v\nwant: %v", got, want)
	}
}

func TestCreateBackupTakesAName(t *testing.T) {
	s, dir := backupServer(t)
	m := createBackup(t, s, "before-upgrade")
	if m.statusType != "success" {
		t.Fatalf("status = %q", m.status)
	}
	if got := onlyBackup(t, dir).Name; got != "backup-before-upgrade.json" {
		t.Errorf("name = %q", got)
	}

	// A name that is taken leaves the old backup alone.
	before, err := os.ReadFile(filepath.Join(dir, "backup-before-upgrade.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.entries = s.entries[:1]
	m = createBackup(t, s, "before-upgrade")
	if m.statusType != "error" || !strings.Contains(m.status, "already exists") {
		t.Errorf("status = %q (%s)", m.status, m.statusType)
	}
	after, err := os.ReadFile(filepath.Join(dir, "backup-before-upgrade.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the old backup changed")
	}
}

func TestCreateBackupRefusesANameThatIsAPath(t *testing.T) {
	s, dir := backupServer(t)
	outside := filepath.Join(filepath.Dir(dir), "outside.json")
	m := createBackup(t, s, outside)
	if m.statusType != "error" || !strings.Contains(m.status, "must not be a path") {
		t.Errorf("status = %q (%s)", m.status, m.statusType)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Errorf("a file was written outside the backup directory: %v", err)
	}
}

func TestCreateBackupOfALockedVaultWritesNothing(t *testing.T) {
	s, dir := backupServer(t)
	m := press(t, s.listModel(t), "c")
	m = typeText(t, m, "locked")
	s.v.Lock()
	m = press(t, m, "enter")
	if m.statusType != "error" || !strings.Contains(m.status, "locked") {
		t.Errorf("status = %q (%s)", m.status, m.statusType)
	}
	if backups, _ := backup.List(dir); len(backups) != 0 {
		t.Errorf("wrote %v", backups)
	}
}

func TestIncompleteBackupFromTheTUISaysSoAndKeepsTheRest(t *testing.T) {
	s, dir := backupServer(t)
	s.entries[1].EncryptedData = "bm90IGEgY2lwaGVydGV4dCBvZiB0aGlzIGtleQ=="
	m := createBackup(t, s, "")

	if m.statusType != "error" || !strings.Contains(m.status, "e2") || !strings.Contains(m.status, "saved to") {
		t.Errorf("status = %q (%s)", m.status, m.statusType)
	}
	file, err := backup.Read(onlyBackup(t, dir).Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Entries) != 1 {
		t.Errorf("entries = %d", len(file.Entries))
	}
}

// The screen shows the files of `pwmgr backup`, which uses the same directory.
func TestBackupScreenListsWhatPwmgrBackupWrote(t *testing.T) {
	s, dir := backupServer(t)
	path, err := backup.NewPath(dir, "from-the-command")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backup.Create(s.v, path, backup.Options{}); err != nil {
		t.Fatal(err)
	}

	m := press(t, s.listModel(t), "b")
	if m.screen != BackupScreen {
		t.Fatalf("screen = %v", m.screen)
	}
	if !strings.Contains(m.renderBackupScreen(), "backup-from-the-command.json") {
		t.Errorf("the backup should be listed:\n%s", m.renderBackupScreen())
	}
}

func openBackupScreen(t *testing.T, s *listServer, dir string) Model {
	t.Helper()
	path, err := backup.NewPath(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backup.Create(s.v, path, backup.Options{}); err != nil {
		t.Fatal(err)
	}
	return press(t, s.listModel(t), "b")
}

func TestRestoreAsksFirstAndCanBeDeclined(t *testing.T) {
	s, dir := backupServer(t)
	m := openBackupScreen(t, s, dir)

	m = press(t, m, "r")
	if !m.showingForm || m.form.Type != ConfirmRestoreForm {
		t.Fatalf("r should ask first (status %q)", m.status)
	}
	if view := m.form.View(); !strings.Contains(view, onlyBackup(t, dir).Name) || !strings.Contains(view, "exist twice") {
		t.Errorf("the question should say what happens:\n%s", view)
	}
	if len(s.created) != 0 {
		t.Fatalf("restored before the answer: %v", s.created)
	}

	m = press(t, m, "n")
	if m.showingForm || len(s.created) != 0 {
		t.Fatalf("form open %v, created %v", m.showingForm, s.created)
	}
}

// The twins of the old path: two entries with one name and username are two
// entries, and the existing ones do not stop a restore.
func TestRestoreBringsBackEveryEntryWithAllItHolds(t *testing.T) {
	s, dir := backupServer(t)
	m := openBackupScreen(t, s, dir)

	m = press(t, m, "r", "y")
	if len(s.created) != 2 {
		t.Fatalf("created %v, want both twins", s.created)
	}
	restored, err := s.v.GetEntry(s.created[0])
	if err != nil {
		t.Fatal(err)
	}
	if restored.Name != "AWS" || restored.Type != "totp" || restored.URL != "https://aws.example" {
		t.Errorf("metadata = %+v", restored)
	}
	if got, want := canonData(t, restored.Data), canonJSON(t, backupTestStored); !reflect.DeepEqual(got, want) {
		t.Fatalf("restored\n got: %v\nwant: %v", got, want)
	}
	if m.statusType != "success" || !strings.Contains(m.status, "Restored 2 entries") {
		t.Errorf("status = %q (%s)", m.status, m.statusType)
	}
	if len(m.allEntries) != 4 {
		t.Errorf("the list should show the restored entries too: %d", len(m.allEntries))
	}
}

func TestRestoreFailureStaysOnTheScreen(t *testing.T) {
	s, dir := backupServer(t)
	m := openBackupScreen(t, s, dir)
	if err := os.WriteFile(m.backups[0].Path, []byte("not a backup"), 0o600); err != nil {
		t.Fatal(err)
	}

	m = press(t, m, "r", "y")
	if m.statusType != "error" || !strings.Contains(m.status, "parse backup file") {
		t.Errorf("status = %q (%s)", m.status, m.statusType)
	}
	if len(s.created) != 0 {
		t.Errorf("created %v", s.created)
	}
}

// Nothing deletes a backup from this screen, so nothing may claim to have.
func TestBackupScreenDoesNotPretendToDelete(t *testing.T) {
	s, dir := backupServer(t)
	m := openBackupScreen(t, s, dir)

	m = press(t, m, "d")
	if strings.Contains(strings.ToLower(m.status), "deleted") {
		t.Errorf("status = %q", m.status)
	}
	if strings.Contains(m.renderBackupScreen(), "d: delete") {
		t.Errorf("the help offers what is not there:\n%s", m.renderBackupScreen())
	}
	if _, err := os.Stat(onlyBackup(t, dir).Path); err != nil {
		t.Error(err)
	}
}

func TestBackupActionsWithoutAVaultFail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _ := newTestModel(t, &config.Config{})
	m.vault = nil

	if msg := m.createBackupCmd(nil)().(BackupCreatedMsg); msg.Error == nil {
		t.Error("create should fail")
	}
	if msg := m.restoreBackupCmd(backup.Info{})().(BackupRestoredMsg); msg.Error == nil {
		t.Error("restore should fail")
	}
}

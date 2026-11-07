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

package backup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/vault"
)

// fakeVault is a vault without the server and the crypto: entries by ID, and a
// record of what a restore creates.
type fakeVault struct {
	entries []vault.Entry
	broken  map[string]bool
	created []vault.Entry
	failOn  string
}

func (f *fakeVault) ListEntries() ([]vault.Entry, error) {
	out := make([]vault.Entry, len(f.entries))
	for i, e := range f.entries {
		e.Data = nil // the list is metadata only
		out[i] = e
	}
	return out, nil
}

func (f *fakeVault) GetEntry(id string) (*vault.Entry, error) {
	if f.broken[id] {
		return nil, errors.New("decrypt failed")
	}
	for i := range f.entries {
		if f.entries[i].ID == id {
			return &f.entries[i], nil
		}
	}
	return nil, os.ErrNotExist
}

func (f *fakeVault) CreateEntry(name, entryType, url string, data *vault.EntryData, _ *string, _, _ string, _ []string) (*vault.Entry, error) {
	if name == f.failOn {
		return nil, errors.New("server says no")
	}
	f.created = append(f.created, vault.Entry{Name: name, Type: entryType, URL: url, Data: data})
	return &vault.Entry{}, nil
}

func entryData(t *testing.T, stored string) *vault.EntryData {
	t.Helper()
	var d vault.EntryData
	if err := json.Unmarshal([]byte(stored), &d); err != nil {
		t.Fatal(err)
	}
	return &d
}

func canon(t *testing.T, v any) any {
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

// Everything the weak backup used to lose.
const fullEntry = `{
	"username": "octo",
	"password": "hunter2",
	"totp_secret": "JBSWY3DPEHPK3PXP",
	"period": 60,
	"notes": "my notes",
	"tags": ["work", "two words"],
	"card_number": "4111",
	"iban": "AT00 1234",
	"future_flag": {"nested": [1, 2]},
	"custom_fields": [
		{"label": "PIN", "value": "1234", "type": "password"},
		{"label": "Scan", "value": "aGVsbG8=", "type": "file", "filename": "scan.png", "mime_type": "image/png", "future_cf": true}
	]
}`

func TestBackupKeepsTheWholeEntryThroughARestore(t *testing.T) {
	src := &fakeVault{entries: []vault.Entry{
		{ID: "e1", Name: "AWS", Type: "totp", URL: "https://aws.example", Data: entryData(t, fullEntry)},
	}}
	path := filepath.Join(t.TempDir(), "backup.json")

	res, err := Create(src, path, Options{Server: "https://pb.example", Email: "max@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != path || res.Saved != 1 || res.Total != 1 {
		t.Errorf("result = %+v", res)
	}

	file, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.Server != "https://pb.example" || file.Email != "max@example.com" || file.CreatedAt == "" {
		t.Errorf("header = %+v", file)
	}

	dst := &fakeVault{}
	if got := Restore(dst, file, nil); got.Restored != 1 || len(got.Failed) != 0 {
		t.Fatalf("restore = %+v", got)
	}
	created := dst.created[0]
	if created.Name != "AWS" || created.Type != "totp" || created.URL != "https://aws.example" {
		t.Errorf("metadata = %+v", created)
	}
	if got, want := canon(t, created.Data), canon(t, entryData(t, fullEntry)); !reflect.DeepEqual(got, want) {
		t.Fatalf("restored data\n got: %v\nwant: %v", got, want)
	}
}

func TestBackupFileIsForItsOwnerOnly(t *testing.T) {
	src := &fakeVault{entries: []vault.Entry{{ID: "e1", Name: "A", Data: &vault.EntryData{Password: "pw"}}}}
	dir := t.TempDir()

	t.Run("new file", func(t *testing.T) {
		path := filepath.Join(dir, "new.json")
		if _, err := Create(src, path, Options{}); err != nil {
			t.Fatal(err)
		}
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v", info.Mode())
		}
	})

	t.Run("a file with wider permissions is not reused", func(t *testing.T) {
		path := filepath.Join(dir, "old.json")
		if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Create(src, path, Options{}); err != nil {
			t.Fatal(err)
		}
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v", info.Mode())
		}
	})

	t.Run("nothing else stays behind", func(t *testing.T) {
		files, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasPrefix(f.Name(), ".") {
				t.Errorf("left behind: %s", f.Name())
			}
		}
	})

	t.Run("a directory that is not there is an error", func(t *testing.T) {
		path := filepath.Join(dir, "missing", "backup.json")
		if _, err := Create(src, path, Options{}); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestBackupOfTheRestNamesWhatItSkipped(t *testing.T) {
	src := &fakeVault{
		entries: []vault.Entry{
			{ID: "e1", Name: "Bank", Data: &vault.EntryData{Password: "pw"}},
			{ID: "e9", Name: "Broken", Data: &vault.EntryData{Password: "pw"}},
			{ID: "e3", Name: "Empty"},
		},
		broken: map[string]bool{"e9": true},
	}
	path := filepath.Join(t.TempDir(), "backup.json")

	res, err := Create(src, path, Options{})
	var incomplete *IncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("err = %v, want an IncompleteError", err)
	}
	for _, want := range []string{"Broken (id e9)", "decrypt failed", "Empty (id e3)", "NOT in it"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error must mention %q: %v", want, err)
		}
	}
	if res.Path != path || res.Saved != 1 || res.Total != 3 {
		t.Errorf("result = %+v", res)
	}
	file, err := Read(path)
	if err != nil {
		t.Fatalf("the rest should have been written: %v", err)
	}
	if len(file.Entries) != 1 || file.Entries[0].Name != "Bank" {
		t.Fatalf("entries = %+v", file.Entries)
	}
}

func TestCreateReportsProgress(t *testing.T) {
	src := &fakeVault{entries: []vault.Entry{
		{ID: "e1", Name: "A", Data: &vault.EntryData{}},
		{ID: "e2", Name: "B", Data: &vault.EntryData{}},
	}}
	var seen []int
	_, err := Create(src, filepath.Join(t.TempDir(), "b.json"), Options{Progress: func(done, total int) {
		if total != 2 {
			t.Errorf("total = %d", total)
		}
		seen = append(seen, done)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seen, []int{1, 2}) {
		t.Errorf("progress = %v", seen)
	}
}

// Twins are two entries, and the backup must not turn them into one.
func TestRestoreKeepsTwins(t *testing.T) {
	file := &File{Entries: []Entry{
		{Name: "GitHub", Type: "password", Data: &vault.EntryData{Username: "octo", Password: "one"}},
		{Name: "GitHub", Type: "password", Data: &vault.EntryData{Username: "octo", Password: "two"}},
	}}
	dst := &fakeVault{entries: []vault.Entry{{ID: "e1", Name: "GitHub", Data: &vault.EntryData{Username: "octo"}}}}

	if got := Restore(dst, file, nil); got.Restored != 2 || len(got.Failed) != 0 {
		t.Fatalf("restore = %+v", got)
	}
	if len(dst.created) != 2 || dst.created[0].Data.Password != "one" || dst.created[1].Data.Password != "two" {
		t.Fatalf("created = %+v", dst.created)
	}
}

func TestRestoreGoesOnAfterAFailureAndNamesIt(t *testing.T) {
	file := &File{Entries: []Entry{
		{Name: "One", Data: &vault.EntryData{}},
		{Name: "Bad", Data: &vault.EntryData{}},
		{Name: "Two", Data: &vault.EntryData{}},
	}}
	dst := &fakeVault{failOn: "Bad"}

	got := Restore(dst, file, nil)
	if got.Restored != 2 || len(got.Failed) != 1 || !strings.Contains(got.Failed[0], "Bad") || !strings.Contains(got.Failed[0], "server says no") {
		t.Fatalf("restore = %+v", got)
	}
}

func TestRestoreDropsAStraySecretAndCopesWithoutData(t *testing.T) {
	file := &File{Entries: []Entry{
		{Name: "Old", Data: &vault.EntryData{Password: "pw", TOTPSecret: "pw"}},
		{Name: "Bare"},
	}}
	dst := &fakeVault{}
	if got := Restore(dst, file, nil); got.Restored != 2 {
		t.Fatalf("restore = %+v", got)
	}
	if dst.created[0].Data.TOTPSecret != "" || dst.created[0].Data.Password != "pw" {
		t.Errorf("stray = %+v", dst.created[0].Data)
	}
	if dst.created[1].Data == nil {
		t.Error("an entry without data must not be stored as JSON null")
	}
}

func TestReadRejectsWhatIsNoBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.tar.gz")
	if err := os.WriteFile(path, []byte("\x1f\x8b not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "parse backup file") {
		t.Errorf("err = %v", err)
	}
	if _, err := Read(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("a missing file should fail")
	}
}

func TestNewPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "backups")

	auto, err := NewPath(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(auto), "backup-") || filepath.Ext(auto) != ".json" {
		t.Errorf("auto name = %s", auto)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("dir = %v, %v", info, err)
	}

	for in, want := range map[string]string{
		"before-upgrade":      "backup-before-upgrade.json",
		"before-upgrade.json": "backup-before-upgrade.json",
		"backup-x":            "backup-x.json",
		"two words":           "backup-two words.json",
	} {
		got, err := NewPath(dir, in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if filepath.Base(got) != want || filepath.Dir(got) != dir {
			t.Errorf("%q = %s", in, got)
		}
	}

	for _, bad := range []string{"../evil", "/etc/passwd", `a\b`, "backup-", ".json"} {
		if got, err := NewPath(dir, bad); err == nil {
			t.Errorf("%q should be refused, got %s", bad, got)
		}
	}

	taken := filepath.Join(dir, "backup-taken.json")
	if err := os.WriteFile(taken, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPath(dir, "taken"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("err = %v", err)
	}
}

func TestListShowsBackupsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	for i, name := range []string{"backup-old.json", "backup-new.json", "backup-middle.json"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		age := map[int]time.Duration{0: 3 * time.Hour, 1: 0, 2: time.Hour}[i]
		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	for _, other := range []string{"notes.txt", "pwmgr-backup-old.tar.gz", "backup-x.txt", ".pwmgr-backup-123"} {
		if err := os.WriteFile(filepath.Join(dir, other), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "backup-dir.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	backups, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range backups {
		names = append(names, b.Name)
		if b.Path != filepath.Join(dir, b.Name) || b.Size != 2 {
			t.Errorf("info = %+v", b)
		}
	}
	if !slices.Equal(names, []string{"backup-new.json", "backup-middle.json", "backup-old.json"}) {
		t.Fatalf("names = %v", names)
	}
}

func TestListOfNoDirectoryIsEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	backups, err := List(dir)
	if err != nil || len(backups) != 0 {
		t.Fatalf("backups %v, err %v", backups, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("looking must not create the directory")
	}
}

func TestDefaultDirIsUnderTheHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err := DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "Documents", "pwmgr-backups"); dir != want {
		t.Fatalf("dir = %s, want %s", dir, want)
	}
}

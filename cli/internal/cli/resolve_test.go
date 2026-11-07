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
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/pkg/backup"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	twinFirstID  = "e1"
	twinSecondID = "e2"
	bankID       = "e3"
	totpID       = "e4"
	totpTwinID   = "e5"
)

// resolverVault has two entries named GitHub, a unique Bank, a password and a
// TOTP entry both named Mixed, and two TOTP entries named Twin.
func resolverVault(t *testing.T) *commandVault {
	t.Helper()
	s := newCommandVault(t, `{"username":"octo","password":"pw-first"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: twinSecondID, Name: "GitHub", Type: "password", URL: "https://work.example/login"},
		`{"username":"hubot","password":"pw-second"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: bankID, Name: "Bank", Type: "password"}, `{"password":"pw-bank"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e6", Name: "Mixed", Type: "password"}, `{"password":"pw-mixed"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: totpID, Name: "Mixed", Type: "totp"}, `{"totp_secret":"JBSWY3DPEHPK3PXP"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: totpTwinID, Name: "Twin", Type: "totp"}, `{"totp_secret":"JBSWY3DPEHPK3PXP"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e7", Name: "Twin", Type: "totp"}, `{"totp_secret":"GEZDGNBVGY3TQOJQ"}`)
	return s
}

// withStdin answers the command's prompt.
func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		_ = r.Close()
	})
}

// captureStdout returns what fn prints.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stdout = orig
	_ = w.Close()
	return <-done
}

func TestResolveEntry(t *testing.T) {
	resolverVault(t)
	for _, tc := range []struct {
		name, arg string
		opts      resolveOptions
		wantID    string
		wantErr   []string
	}{
		{name: "unique exact name", arg: "Bank", wantID: bankID},
		{name: "id", arg: twinSecondID, wantID: twinSecondID},
		{name: "several", arg: "GitHub", wantErr: []string{
			"2 entries", "GitHub", twinFirstID, twinSecondID, "octo", "hubot", "github.com", "work.example", "nothing was done"}},
		{name: "substring", arg: "Git", wantErr: []string{"no entry named exactly 'Git'"}},
		{name: "other case", arg: "bank", wantErr: []string{"no entry named exactly 'bank'"}},
		{name: "nothing", arg: "Nope", wantErr: []string{"no entry named exactly 'Nope'", "pwmgr search"}},
		{name: "type narrows", arg: "Mixed", opts: resolveOptions{entryType: "totp"}, wantID: totpID},
		{name: "type narrows but still several", arg: "Twin", opts: resolveOptions{entryType: "totp"}, wantErr: []string{"2 entries", totpTwinID}},
		{name: "type does not fit", arg: "Bank", opts: resolveOptions{entryType: "totp"}, wantErr: []string{"not a TOTP entry"}},
		{name: "type does not fit an id", arg: bankID, opts: resolveOptions{entryType: "totp"}, wantErr: []string{"not a TOTP entry"}},
		{name: "username narrows", arg: "GitHub", opts: resolveOptions{username: "hubot"}, wantID: twinSecondID},
		{name: "username of nobody", arg: "GitHub", opts: resolveOptions{username: "nobody"}, wantErr: []string{"username 'nobody'"}},
		{name: "id ignores the username", arg: twinFirstID, opts: resolveOptions{username: "hubot"}, wantID: twinFirstID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveEntry(tc.arg, tc.opts)
			if len(tc.wantErr) > 0 {
				if err == nil {
					t.Fatalf("resolved %q, want an error", got.ID)
				}
				for _, want := range tc.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error should mention %q:\n%v", want, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != tc.wantID {
				t.Fatalf("resolved %q, want %q", got.ID, tc.wantID)
			}
		})
	}
}

func TestAmbiguousErrorWithoutUnlockStillListsIDs(t *testing.T) {
	resolverVault(t)
	v.Lock()
	orig := unlockForUsernames
	unlockForUsernames = func() error { return errors.New("no terminal") }
	t.Cleanup(func() { unlockForUsernames = orig })

	_, err := resolveEntry("GitHub", resolveOptions{})
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{twinFirstID, twinSecondID, "(locked)", "github.com"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q:\n%v", want, err)
		}
	}
}

func TestAmbiguousErrorUnlocksForTheUsernames(t *testing.T) {
	resolverVault(t)
	testVault := v
	const password = "correct horse"
	testVault.Lock()
	orig := unlockForUsernames
	unlockForUsernames = func() error { return testVault.Unlock(password) }
	t.Cleanup(func() { unlockForUsernames = orig })

	_, err := resolveEntry("GitHub", resolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "hubot") {
		t.Fatalf("err = %v", err)
	}
}

// Destructive commands get a yes at the prompt: only the resolver may stop them.
func TestDestructiveCommandsActOnlyOnAUniqueExactMatch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cmd     *cobra.Command
		args    []string
		wantErr string
		want    []string
	}{
		{"delete, several", deleteCmd, []string{"GitHub"}, twinSecondID, nil},
		{"delete, substring", deleteCmd, []string{"Git"}, "no entry named exactly", nil},
		{"delete, other case", deleteCmd, []string{"bank"}, "no entry named exactly", nil},
		{"delete, unique", deleteCmd, []string{"Bank"}, "", []string{bankID}},
		{"delete, by id", deleteCmd, []string{twinSecondID}, "", []string{twinSecondID}},
		{"totp-delete, several", totpDeleteCmd, []string{"Twin"}, totpTwinID, nil},
		{"totp-delete, substring", totpDeleteCmd, []string{"Tw"}, "no entry named exactly", nil},
		{"totp-delete, not a totp entry", totpDeleteCmd, []string{"Bank"}, "not a TOTP entry", nil},
		{"totp-delete, the totp one of two", totpDeleteCmd, []string{"Mixed"}, "", []string{totpID}},
		{"totp-delete, by id", totpDeleteCmd, []string{totpTwinID}, "", []string{totpTwinID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := resolverVault(t)
			withStdin(t, "y\n")
			err := tc.cmd.RunE(tc.cmd, tc.args)
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Errorf("err = %v, want it to mention %q", err, tc.wantErr)
			}
			if tc.wantErr == "" && err != nil {
				t.Errorf("err = %v", err)
			}
			if !slices.Equal(s.deleted, tc.want) {
				t.Errorf("deleted %v, want %v", s.deleted, tc.want)
			}
		})
	}
}

func TestDeleteCanBeDeclined(t *testing.T) {
	s := resolverVault(t)
	withStdin(t, "n\n")
	if err := deleteCmd.RunE(deleteCmd, []string{"Bank"}); err != nil {
		t.Fatal(err)
	}
	if len(s.deleted) != 0 {
		t.Fatalf("deleted %v", s.deleted)
	}
}

func TestGetGoesThroughTheResolver(t *testing.T) {
	resolverVault(t)

	out := captureStdout(t, func() {
		if err := getCmd.RunE(getCmd, []string{twinSecondID}); err != nil {
			t.Errorf("get by id: %v", err)
		}
	})
	if !strings.Contains(out, "pw-second") {
		t.Errorf("get by id printed:\n%s", out)
	}

	out = captureStdout(t, func() {
		if err := getCmd.RunE(getCmd, []string{"GitHub"}); err == nil {
			t.Error("get with several matches should fail")
		}
		if err := getCmd.RunE(getCmd, []string{"Git"}); err == nil {
			t.Error("get with a substring should fail")
		}
	})
	if strings.Contains(out, "pw-first") || strings.Contains(out, "pw-second") {
		t.Errorf("a guess reached the terminal:\n%s", out)
	}
}

func resetFlags(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	t.Cleanup(func() {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		})
	})
}

func TestUpdateGoesThroughTheResolver(t *testing.T) {
	resetFlags(t, updateCmd)
	if err := updateCmd.Flags().Set("notes", "new notes"); err != nil {
		t.Fatal(err)
	}

	s := resolverVault(t)
	if err := updateCmd.RunE(updateCmd, []string{"GitHub"}); err == nil || !strings.Contains(err.Error(), twinSecondID) {
		t.Errorf("err = %v", err)
	}
	if err := updateCmd.RunE(updateCmd, []string{"Git"}); err == nil {
		t.Error("a substring should not match")
	}
	if len(s.puts) != 0 {
		t.Fatalf("updated %v", s.putIDs)
	}

	if err := updateCmd.RunE(updateCmd, []string{twinSecondID}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.putIDs, []string{twinSecondID}) {
		t.Fatalf("updated %v", s.putIDs)
	}
}

func TestTOTPCodeGoesThroughTheResolver(t *testing.T) {
	resolverVault(t)
	out := captureStdout(t, func() {
		if err := totpCodeCmd.RunE(totpCodeCmd, []string{"Mixed"}); err != nil {
			t.Errorf("the one TOTP entry named Mixed should resolve: %v", err)
		}
	})
	if !strings.Contains(out, "Code:") {
		t.Errorf("printed:\n%s", out)
	}
	if err := totpCodeCmd.RunE(totpCodeCmd, []string{"Twin"}); err == nil {
		t.Error("two TOTP entries should be an error")
	}
	if err := totpCodeCmd.RunE(totpCodeCmd, []string{"Bank"}); err == nil || !strings.Contains(err.Error(), "not a TOTP entry") {
		t.Errorf("err = %v", err)
	}
}

// The importers keep a login with 2FA a password entry, and it must not lose its
// code over that.
func TestTOTPCodeWorksForALoginWithASecret(t *testing.T) {
	s := resolverVault(t)
	s.addEntry(t, apiclient.EntryResponse{ID: "e8", Name: "Login", Type: "password"}, `{"password":"pw","totp_secret":"JBSWY3DPEHPK3PXP"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e9", Name: "Both", Type: "password"}, `{"password":"pw","totp_secret":"JBSWY3DPEHPK3PXP"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e10", Name: "Both", Type: "password"}, `{"password":"pw","totp_secret":"GEZDGNBVGY3TQOJQ"}`)

	for _, arg := range []string{"Login", "e8"} {
		out := captureStdout(t, func() {
			if err := totpCodeCmd.RunE(totpCodeCmd, []string{arg}); err != nil {
				t.Errorf("%s: %v", arg, err)
			}
		})
		if !strings.Contains(out, "Code:") {
			t.Errorf("%s printed:\n%s", arg, out)
		}
	}

	// Still no guessing, and a login without a secret is still no TOTP entry.
	if err := totpCodeCmd.RunE(totpCodeCmd, []string{"Both"}); err == nil || !strings.Contains(err.Error(), "e10") {
		t.Errorf("err = %v", err)
	}
	if err := totpCodeCmd.RunE(totpCodeCmd, []string{"Bank"}); err == nil || !strings.Contains(err.Error(), "not a TOTP entry") {
		t.Errorf("err = %v", err)
	}
}

// Nothing was wrong here: this pins it, because the TUI got it wrong.
func TestGetPrintsThePasswordOfATOTPEntry(t *testing.T) {
	s := newCommandVault(t, `{"password":"pw"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "AWS", Type: "totp"}, `{"password":"hunter2","totp_secret":"JBSWY3DPEHPK3PXP"}`)

	out := captureStdout(t, func() {
		if err := getCmd.RunE(getCmd, []string{"AWS"}); err != nil {
			t.Error(err)
		}
	})
	if !strings.Contains(out, "hunter2") || !strings.Contains(out, "JBSWY3DPEHPK3PXP") {
		t.Errorf("printed:\n%s", out)
	}
}

func TestFavoriteAndHistoryGoThroughTheResolver(t *testing.T) {
	resetFlags(t, favoriteCmd)
	resetFlags(t, historyCmd)
	s := resolverVault(t)

	if err := favoriteCmd.RunE(favoriteCmd, []string{"GitHub"}); err == nil {
		t.Error("favorite with several matches should fail")
	}
	if err := favoriteCmd.RunE(favoriteCmd, []string{"Git"}); err == nil {
		t.Error("favorite with a substring should fail")
	}
	if len(s.favorites) != 0 {
		t.Fatalf("favorites %v", s.favorites)
	}
	if err := favoriteCmd.RunE(favoriteCmd, []string{twinSecondID}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.favorites, []string{twinSecondID}) {
		t.Fatalf("favorites %v", s.favorites)
	}

	if err := historyCmd.Flags().Set("restore", "v1"); err != nil {
		t.Fatal(err)
	}
	withStdin(t, "y\n")
	if err := historyCmd.RunE(historyCmd, []string{"GitHub"}); err == nil {
		t.Error("history --restore with several matches should fail")
	}
	if len(s.versions) != 0 {
		t.Fatalf("restored %v", s.versions)
	}
	if err := historyCmd.RunE(historyCmd, []string{"Bank"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.versions, []string{bankID + "/versions/v1/restore"}) {
		t.Fatalf("restored %v", s.versions)
	}
}

func TestTrashPurgeActsOnlyOnAUniqueExactMatch(t *testing.T) {
	s := newCommandVault(t, `{"password":"pw"}`)
	s.trash = []apiclient.EntryResponse{
		{ID: "t1", Name: "Old", URL: "https://old.example"},
		{ID: "t2", Name: "Old", URL: "https://older.example"},
		{ID: "t3", Name: "Gone"},
	}
	for _, tc := range []struct {
		arg, wantErr string
		want         []string
	}{
		{"Old", "older.example", nil},
		{"gone", "no trashed entry named exactly", nil},
		{"Go", "no trashed entry named exactly", nil},
		{"t1", "", []string{"t1"}},
		{"Gone", "", []string{"t1", "t3"}},
	} {
		withStdin(t, "y\n")
		err := trashPurgeCmd.RunE(trashPurgeCmd, []string{tc.arg})
		if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
			t.Errorf("%s: err = %v", tc.arg, err)
		}
		if tc.wantErr == "" && err != nil {
			t.Errorf("%s: err = %v", tc.arg, err)
		}
		if tc.want != nil && !slices.Equal(s.purged, tc.want) {
			t.Errorf("%s: purged %v, want %v", tc.arg, s.purged, tc.want)
		}
	}
	if len(s.purged) != 2 {
		t.Errorf("purged %v", s.purged)
	}
}

// --- stray TOTP secret in the commands ---

func TestGetPrintsNoLineForAStrayTOTPSecret(t *testing.T) {
	s := newCommandVault(t, `{"username":"octo","password":"hunter2","totp_secret":"hunter2"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "Real", Type: "password"}, `{"password":"hunter2","totp_secret":"JBSWY3DPEHPK3PXP"}`)

	out := captureStdout(t, func() {
		if err := getCmd.RunE(getCmd, []string{"GitHub"}); err != nil {
			t.Error(err)
		}
	})
	if strings.Contains(out, "TOTP Secret") {
		t.Errorf("a stray secret was printed:\n%s", out)
	}
	if !strings.Contains(out, "hunter2") {
		t.Errorf("the password should still print:\n%s", out)
	}

	out = captureStdout(t, func() {
		if err := getCmd.RunE(getCmd, []string{"Real"}); err != nil {
			t.Error(err)
		}
	})
	if !strings.Contains(out, "TOTP Secret") || !strings.Contains(out, "JBSWY3DPEHPK3PXP") {
		t.Errorf("a real secret must print:\n%s", out)
	}
}

func TestUpdateDropsAStraySecretAndKeepsARealOne(t *testing.T) {
	resetFlags(t, updateCmd)
	if err := updateCmd.Flags().Set("notes", "new notes"); err != nil {
		t.Fatal(err)
	}

	s := newCommandVault(t, `{"password":"hunter2","totp_secret":"hunter2"}`)
	if err := updateCmd.RunE(updateCmd, []string{commandEntryID}); err != nil {
		t.Fatal(err)
	}
	if got := s.saved(t); got["totp_secret"] != nil || got["password"] != "hunter2" || got["notes"] != "new notes" {
		t.Fatalf("saved %v", got)
	}

	s = newCommandVault(t, `{"password":"hunter2","totp_secret":"JBSWY3DPEHPK3PXP"}`)
	if err := updateCmd.RunE(updateCmd, []string{commandEntryID}); err != nil {
		t.Fatal(err)
	}
	if got := s.saved(t); got["totp_secret"] != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("a real secret must stay: %v", got)
	}
}

func TestTOTPCodeIgnoresAStraySecret(t *testing.T) {
	s := newCommandVault(t, `{"password":"pw"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "Stray", Type: "totp"}, `{"password":"JBSWY3DPEHPK3PXP","totp_secret":"JBSWY3DPEHPK3PXP"}`)
	if err := totpCodeCmd.RunE(totpCodeCmd, []string{"Stray"}); err == nil || !strings.Contains(err.Error(), "no TOTP secret") {
		t.Fatalf("err = %v", err)
	}
}

// --- backup ---

func TestBackupSavesTheRestAndFailsNamingWhatItSkipped(t *testing.T) {
	resetFlags(t, backupCmd)
	s := newCommandVault(t, `{"username":"octo","password":"pw"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "Fine", Type: "password"}, `{"password":"pw-fine"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e3", Name: "Broken", Type: "password"}, `{"password":"pw-broken"}`)
	s.entries[2].EncryptedData = "bm90IGEgY2lwaGVydGV4dCBvZiB0aGlzIGtleQ=="

	path := filepath.Join(t.TempDir(), "backup.json")
	if err := backupCmd.Flags().Set("output", path); err != nil {
		t.Fatal(err)
	}
	err := backupCmd.RunE(backupCmd, nil)
	if err == nil {
		t.Fatal("an incomplete backup must exit non-zero")
	}
	for _, want := range []string{"Broken", "e3", "NOT in it"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the rest should have been saved: %v", err)
	}
	var bk backup.File
	if err := json.Unmarshal(raw, &bk); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range bk.Entries {
		names = append(names, e.Name)
	}
	if !slices.Equal(names, []string{"GitHub", "Fine"}) {
		t.Fatalf("backed up %v", names)
	}
}

// The commands and the TUI share one implementation, so this is the whole way
// through the commands: what the TUI restores is what `pwmgr backup` wrote.
func TestBackupAndRestoreCommandsKeepTheEntry(t *testing.T) {
	resetFlags(t, backupCmd)
	resetFlags(t, restoreCmd)
	const stored = `{"username":"octo","password":"hunter2","totp_secret":"JBSWY3DPEHPK3PXP","iban":"AT00",
		"tags":["work"],"future_flag":{"nested":true},
		"custom_fields":[{"label":"PIN","value":"1234","type":"password","future_cf":1}]}`
	s := newCommandVault(t, `{"password":"pw"}`)
	s.addEntry(t, apiclient.EntryResponse{ID: "e2", Name: "AWS", Type: "totp", URL: "https://aws.example"}, stored)

	dir := t.TempDir()
	if err := backupCmd.Flags().Set("dir", dir); err != nil {
		t.Fatal(err)
	}
	if err := backupCmd.RunE(backupCmd, nil); err != nil {
		t.Fatal(err)
	}
	backups, err := backup.List(dir)
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups %v, err %v", backups, err)
	}
	if info, err := os.Stat(backups[0].Path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("file = %v, %v", info, err)
	}
	out := captureStdout(t, func() {
		if err := listBackupsCmd.Flags().Set("dir", dir); err != nil {
			t.Error(err)
		}
		if err := listBackupsCmd.RunE(listBackupsCmd, nil); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(func() { _ = listBackupsCmd.Flags().Set("dir", "") })
	if !strings.Contains(out, backups[0].Name) {
		t.Errorf("list-backups printed:\n%s", out)
	}

	if err := restoreCmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	if err := restoreCmd.RunE(restoreCmd, []string{backups[0].Path}); err != nil {
		t.Fatal(err)
	}
	if len(s.posts) != 2 {
		t.Fatalf("restored %d entries, want 2", len(s.posts))
	}
	post := s.posts[1]
	if post.Name != "AWS" || post.Type != "totp" || post.URL != "https://aws.example" {
		t.Errorf("metadata = %+v", post)
	}
	// Hand the upload back to the vault: that also proves it is readable.
	s.entries = append(s.entries, apiclient.EntryResponse{
		ID: "restored", Name: post.Name, Type: post.Type, EncryptedData: post.EncryptedData, EntryKey: &post.EntryKeys[0],
	})
	restored, err := v.GetEntry("restored")
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := json.Marshal(restored.Data)
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(plaintext, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(stored), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("restored\n got: %v\nwant: %v", got, want)
	}
}

func TestBackupOfEverythingSucceeds(t *testing.T) {
	resetFlags(t, backupCmd)
	newCommandVault(t, `{"username":"octo","password":"pw"}`)
	if err := backupCmd.Flags().Set("output", filepath.Join(t.TempDir(), "backup.json")); err != nil {
		t.Fatal(err)
	}
	if err := backupCmd.RunE(backupCmd, nil); err != nil {
		t.Fatal(err)
	}
}

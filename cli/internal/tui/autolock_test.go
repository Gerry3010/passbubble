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
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
	vaultpkg "github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/Gerry3010/passbubble/cli/pkg/backup"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	testMasterPassword = "correct horse"
	testFormSecret     = "form-secret-hunter2"
)

// unlockedVault returns a vault with real keys in memory, talking to serverURL.
func unlockedVault(t *testing.T, serverURL string) *vaultpkg.Vault {
	t.Helper()
	privX, pubX, err := crypto.GenerateX25519()
	if err != nil {
		t.Fatal(err)
	}
	privM, pubM, err := crypto.GenerateMLKEM768()
	if err != nil {
		t.Fatal(err)
	}
	params := &crypto.KDFParams{Salt: make([]byte, 16), Time: 1, Memory: 8 * 1024}
	masterKey := crypto.DeriveKey(testMasterPassword, params)
	encX, err := crypto.Encrypt(masterKey, privX)
	if err != nil {
		t.Fatal(err)
	}
	encM, err := crypto.Encrypt(masterKey, privM)
	if err != nil {
		t.Fatal(err)
	}
	v := vaultpkg.New(&config.Config{
		ServerURL:       serverURL,
		UserID:          "u1",
		RefreshToken:    "refresh",
		PubX25519:       crypto.B64Enc(pubX),
		PubMLKEM768:     crypto.B64Enc(pubM),
		EncPrivX25519:   crypto.B64Enc(encX),
		EncPrivMLKEM768: crypto.B64Enc(encM),
		KDFSalt:         crypto.B64Enc(params.Salt),
		KDFTime:         int(params.Time),
		KDFMemory:       int(params.Memory),
	}, filepath.Join(t.TempDir(), "vault.yaml"))
	if err := v.Unlock(testMasterPassword); err != nil {
		t.Fatal(err)
	}
	return v
}

// testForms lists one form of every FormType.
func testForms() map[string]FormModel {
	pw := Entry{ID: "e1", Service: "GitHub", Type: "password"}
	otp := Entry{ID: "e2", Service: "AWS", Type: "totp"}
	return map[string]FormModel{
		"add password":       CreateAddPasswordForm(),
		"add totp":           CreateAddTOTPForm(),
		"add totp to entry":  CreateAddTOTPToEntryForm(pw),
		"edit password":      CreateEditEntryForm(pw, &vaultpkg.EntryData{Username: "octo"}),
		"edit totp":          CreateEditEntryForm(otp, &vaultpkg.EntryData{Issuer: "AWS"}),
		"confirm delete":     CreateConfirmDeleteForm(pw),
		"create backup":      CreateCreateBackupForm(),
		"save password":      CreateSavePasswordForm(testFormSecret),
		"new folder":         CreateNewFolderForm(nil),
		"rename folder":      CreateRenameFolderForm("f1", "Work", nil),
		"delete folder":      CreateConfirmDeleteFolderForm("f1", "Work"),
		"generator options":  CreateGeneratorOptionsForm(config.DefaultGeneratorSettings()),
		"generator defaults": CreateGeneratorDefaultsForm(config.DefaultGeneratorSettings()),
		"confirm restore":    CreateConfirmRestoreForm(backup.Info{Name: "backup-x.json", Path: "/tmp/backup-x.json"}),
	}
}

// modelWithForm returns an unlocked model showing form, its last field holding
// a secret, and a user who has just been active.
func modelWithForm(t *testing.T, form FormModel) Model {
	t.Helper()
	m, _ := newTestModel(t, &config.Config{})
	m.vault = unlockedVault(t, "http://127.0.0.1:9")
	m.idleTimeout = 10 * time.Minute
	m.lastActivity = time.Now()
	if n := len(form.Fields); n > 0 {
		form.Fields = append([]FormField(nil), form.Fields...)
		form.Fields[n-1].Value = testFormSecret
	}
	m.form = form
	m.showingForm = true
	return m
}

func tick(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(TickMsg{Time: time.Now()})
	return next.(Model), cmd
}

func TestTickChainSurvivesEveryForm(t *testing.T) {
	for name, form := range testForms() {
		t.Run(name, func(t *testing.T) {
			m, cmd := tick(t, modelWithForm(t, form))
			if cmd == nil {
				t.Fatal("no next tick scheduled: the auto-lock timer stops here")
			}
			if !m.showingForm || m.screen != MainScreen {
				t.Fatal("an active user must keep the form")
			}
		})
	}
}

func TestTickCommandYieldsNextTickWhileFormIsOpen(t *testing.T) {
	_, cmd := tick(t, modelWithForm(t, CreateAddPasswordForm()))
	if cmd == nil {
		t.Fatal("no next tick scheduled")
	}
	if _, ok := cmd().(TickMsg); !ok {
		t.Fatal("the scheduled command should deliver the next TickMsg")
	}
}

func TestAutoLockFiresInEveryFormAndDiscardsSecrets(t *testing.T) {
	for name, form := range testForms() {
		t.Run(name, func(t *testing.T) {
			m := modelWithForm(t, form)
			m.lastActivity = time.Now().Add(-time.Hour)

			m, cmd := tick(t, m)
			if m.vault.IsUnlocked() {
				t.Fatal("idle timeout should have locked the vault")
			}
			if cmd == nil {
				t.Error("the timer should keep running after the lock")
			}
			if m.screen != UnlockScreen {
				t.Errorf("screen = %v, want UnlockScreen", m.screen)
			}
			if m.showingForm || len(m.form.Fields) != 0 || m.form.Entry != nil {
				t.Errorf("the form's contents survived the lock: %+v", m.form)
			}
			out := m.View()
			if strings.Contains(out, testFormSecret) {
				t.Error("the secret is still on screen after the lock")
			}
			if !strings.Contains(out, "unlock vault") {
				t.Errorf("the unlock screen should be showing:\n%s", out)
			}
		})
	}
}

func TestAutoLockDiscardsConfirmStepUnderOptionsForm(t *testing.T) {
	m := modelWithForm(t, CreateAddPasswordForm())
	m.form.Fields[0].Value = "GitHub"
	m.form.Fields[2].Value = ""
	m.form.CurrentField = 2
	m = send(t, m, "enter")
	if !m.showGenConfirm {
		t.Fatalf("expected the confirm step, form error %q", m.form.Error)
	}
	m = send(t, m, "o")
	if !m.showingForm || m.form.Type != GeneratorOptionsForm {
		t.Fatal("o should open the options form")
	}

	m.lastActivity = time.Now().Add(-time.Hour)
	m, _ = tick(t, m)
	if m.showingForm || m.showGenConfirm || m.genConfirmPW != "" || m.genFields != nil ||
		len(m.genReturnForm.Fields) != 0 {
		t.Fatal("the lock must drop the options form, the pending password and the add form below it")
	}
}

func TestAutoLockFiresAfterFormWasClosed(t *testing.T) {
	m, cmd := tick(t, modelWithForm(t, CreateAddPasswordForm()))
	if cmd == nil {
		t.Fatal("no next tick scheduled while the form was open")
	}
	m = send(t, m, "esc")
	if m.showingForm {
		t.Fatal("esc should close the form")
	}

	m.lastActivity = time.Now().Add(-time.Hour)
	m, cmd = tick(t, m)
	if m.vault.IsUnlocked() || m.screen != UnlockScreen {
		t.Fatal("the lock should fire after the form was closed")
	}
	if cmd == nil {
		t.Error("the timer should keep running after the lock")
	}
}

func TestLockedFormStaysGoneAfterUnlock(t *testing.T) {
	m := modelWithForm(t, CreateAddPasswordForm())
	m.lastActivity = time.Now().Add(-time.Hour)
	m, _ = tick(t, m)

	next, _ := m.Update(AuthResultMsg{vault: m.vault})
	m = next.(Model)
	if m.screen != MainScreen || m.showingForm {
		t.Fatalf("unlock should land on the list without the old form (screen %v, form %v)", m.screen, m.showingForm)
	}
}

func TestTypingInFormCountsAsActivity(t *testing.T) {
	m := modelWithForm(t, CreateAddPasswordForm())
	m.lastActivity = time.Now().Add(-time.Hour)

	m = send(t, m, "x")
	if got := m.form.Fields[0].Value; got != "x" {
		t.Fatalf("the key should reach the form, field = %q", got)
	}
	m, _ = tick(t, m)
	if !m.vault.IsUnlocked() || !m.showingForm {
		t.Fatal("somebody typing in a form is not idle")
	}
}

func TestClickWhileFormIsOpenStaysInTheForm(t *testing.T) {
	m := modelWithForm(t, CreateAddPasswordForm())
	m.allEntries = []Entry{{ID: "e1", Service: "GitHub", Type: "note"}}
	m.lastActivity = time.Now().Add(-time.Hour)

	click := tea.MouseMsg{X: 5, Y: 4, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	next, _ := m.Update(click)
	m = next.(Model)
	if !m.showingForm || m.screen != MainScreen {
		t.Fatal("a click must not reach the list below the form")
	}
	if time.Since(m.lastActivity) > time.Minute {
		t.Error("a click counts as activity")
	}
}

// Messages the form has no use for belong to the model: a swallowed
// CopiedSecretMsg would leave a secret on the clipboard for good.
func TestBackgroundResultsReachModelWhileFormIsOpen(t *testing.T) {
	m := modelWithForm(t, CreateAddPasswordForm())

	next, cmd := m.Update(CopiedSecretMsg{value: "s3cret", label: "Password"})
	m = next.(Model)
	if cmd == nil {
		t.Error("the clipboard wipe must be scheduled with a form open")
	}

	if _, cmd = m.Update(ClipboardClearMsg{value: "s3cret"}); cmd == nil {
		t.Error("the clipboard wipe must run with a form open")
	}

	next, _ = m.Update(LoadEntriesMsg{Entries: []Entry{{ID: "e1", Service: "GitHub"}}})
	m = next.(Model)
	if len(m.allEntries) != 1 {
		t.Error("loaded entries should arrive with a form open")
	}

	next, _ = m.Update(tea.WindowSizeMsg{Width: 77, Height: 33})
	m = next.(Model)
	if m.width != 77 || m.height != 33 || m.form.width != 77 {
		t.Errorf("resize lost: model %dx%d, form width %d", m.width, m.height, m.form.width)
	}

	if !m.showingForm {
		t.Error("none of these may close the form")
	}
}

func TestLockDiscardsDetailAndOverlaySecrets(t *testing.T) {
	m := modelWithForm(t, CreateAddPasswordForm())
	m.showingForm = false
	m.form = FormModel{}
	m.detailEntry = Entry{ID: "e1", Service: "GitHub", Type: "password"}
	m.detailTags = []string{"work"}
	m.password = "detail-password"
	m.maskPasswords = false
	m.totpCode = "123 456"
	m.totpRemaining = 12
	m.showShareQR = true
	m.shareURL = "https://example.test/s/1#share-key"
	m.shareQR = "QR"
	m.showShareMenu = true
	m.showMoveMenu = true
	m.showSortMenu = true
	m.showHelp = true
	m.showSearch = true
	m.filter = "git"

	m = m.lockVault()
	if m.password != "" || m.totpCode != "" || m.totpRemaining != 0 || m.detailEntry.ID != "" || m.detailTags != nil {
		t.Error("decrypted detail data survived the lock")
	}
	if !m.maskPasswords {
		t.Error("passwords should be masked again after a lock")
	}
	if m.shareURL != "" || m.shareQR != "" {
		t.Error("the share link survived the lock")
	}
	if m.showShareQR || m.showShareMenu || m.showMoveMenu || m.showSortMenu || m.showHelp || m.showSearch || m.filter != "" {
		t.Error("an overlay survived the lock")
	}
	out := m.View()
	if strings.Contains(out, "share-key") || !strings.Contains(out, "unlock vault") {
		t.Errorf("the unlock screen should be showing:\n%s", out)
	}
}

func TestLateResultsAreDroppedAfterLock(t *testing.T) {
	m := modelWithForm(t, CreateAddPasswordForm())
	m = m.lockVault()

	for _, msg := range []tea.Msg{
		SecretLoadMsg{Password: "late-password"},
		TOTPUpdateMsg{Code: "123 456", Remaining: 20},
		LoadEntriesMsg{Entries: []Entry{{ID: "e1", Service: "GitHub"}}},
	} {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	if m.password != "" || m.totpCode != "" || len(m.allEntries) != 0 {
		t.Fatal("a command started before the lock refilled the locked model")
	}
	// Checked directly: its handler would write to the real clipboard.
	if !lateVaultResult(ShareLinkCreatedMsg{url: "https://example.test/s/1#key"}) {
		t.Error("a share link arriving after the lock must be dropped")
	}

	// The wipe of a copied secret still has to happen on the unlock screen.
	if _, cmd := m.Update(CopiedSecretMsg{value: "s3cret", label: "Password"}); cmd == nil {
		t.Error("the clipboard wipe must be scheduled on the unlock screen too")
	}
}

// lockWatcher reports through locked when the wrapped model reaches the unlock
// screen, so the test below needs neither a fixed sleep nor a racy look at the
// vault from outside the event loop.
type lockWatcher struct {
	Model
	locked chan struct{}
	once   *sync.Once
}

func (w lockWatcher) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := w.Model.Update(msg)
	w.Model = next.(Model)
	if w.screen == UnlockScreen {
		w.once.Do(func() { close(w.locked) })
	}
	return w, cmd
}

// The real runtime: the tick chain has to carry the lock through the event loop.
func TestAutoLockFiresInRunningProgram(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{})
	}))
	defer srv.Close()

	m, _ := newTestModel(t, &config.Config{})
	v := unlockedVault(t, srv.URL)
	m.vault = v
	m.idleTimeout = time.Second
	m.lastActivity = time.Now()

	watcher := lockWatcher{Model: m, locked: make(chan struct{}), once: &sync.Once{}}
	p := tea.NewProgram(watcher, tea.WithInput(nil), tea.WithOutput(io.Discard),
		tea.WithoutRenderer(), tea.WithoutSignalHandler())
	type result struct {
		model tea.Model
		err   error
	}
	done := make(chan result, 1)
	go func() {
		final, err := p.Run()
		done <- result{final, err}
	}()

	p.Send(key(m.keymap[actAddPassword]))
	for _, r := range testFormSecret {
		p.Send(key(string(r)))
	}

	fired := false
	select {
	case <-watcher.locked:
		fired = true
	case <-time.After(5 * time.Second): // 1s timeout + at most 1s to the next tick
	}
	p.Quit()

	res := <-done
	if res.err != nil {
		t.Fatal(res.err)
	}
	if !fired {
		t.Fatal("the idle timeout never fired")
	}
	final := res.model.(lockWatcher).Model
	if v.IsUnlocked() || final.screen != UnlockScreen {
		t.Fatalf("not locked (screen %v)", final.screen)
	}
	if final.showingForm || len(final.form.Fields) != 0 {
		t.Fatal("the form survived the lock")
	}
}

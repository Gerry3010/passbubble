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

package vault

import (
	"crypto/rand"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
)

// newPINTestVault builds a logged-in (but locked) vault backed by a temp config
// file, with known private keys encrypted under masterPassword.
func newPINTestVault(t *testing.T, masterPassword string) (*Vault, []byte, []byte) {
	t.Helper()

	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	// Light KDF params keep the test fast; the PIN KDF is fixed in pin.go.
	kdf := &crypto.KDFParams{Salt: salt, Time: 1, Memory: 8192}
	masterKey := crypto.DeriveKey(masterPassword, kdf)

	privX, _, err := crypto.GenerateX25519()
	if err != nil {
		t.Fatal(err)
	}
	privM, _, err := crypto.GenerateMLKEM768()
	if err != nil {
		t.Fatal(err)
	}
	encX, err := crypto.Encrypt(masterKey, privX)
	if err != nil {
		t.Fatal(err)
	}
	encM, err := crypto.Encrypt(masterKey, privM)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		ServerURL:       "http://localhost",
		UserID:          "u1",
		RefreshToken:    "r1",
		KDFSalt:         crypto.B64Enc(salt),
		KDFTime:         1,
		KDFMemory:       8192,
		EncPrivX25519:   crypto.B64Enc(encX),
		EncPrivMLKEM768: crypto.B64Enc(encM),
	}
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	v := New(cfg, cfgPath)
	return v, privX, privM
}

func TestEnableAndUnlockWithPIN(t *testing.T) {
	v, privX, privM := newPINTestVault(t, "master-pw")

	if err := v.EnablePIN("master-pw", "123456", 14, 5); err != nil {
		t.Fatalf("EnablePIN: %v", err)
	}
	if !v.PINEnabled() {
		t.Fatal("PINEnabled should be true after EnablePIN")
	}

	// Fresh vault loaded from the persisted config (simulates a new process).
	cfg, err := config.Load(v.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	v2 := New(cfg, v.cfgPath)

	if err := v2.UnlockWithPIN("123456"); err != nil {
		t.Fatalf("UnlockWithPIN: %v", err)
	}
	if !v2.IsUnlocked() {
		t.Fatal("vault should be unlocked after correct PIN")
	}
	if string(v2.privX25519) != string(privX) || string(v2.privMLKEM) != string(privM) {
		t.Fatal("recovered private keys do not match originals")
	}
}

func TestEnablePINWrongMasterPassword(t *testing.T) {
	v, _, _ := newPINTestVault(t, "master-pw")
	if err := v.EnablePIN("WRONG", "123456", 14, 5); err == nil {
		t.Fatal("EnablePIN with wrong master password should fail")
	}
	if v.PINEnabled() {
		t.Fatal("PIN must not be enabled after a failed EnablePIN")
	}
}

func TestUnlockWithPINWrongThenLockout(t *testing.T) {
	v, _, _ := newPINTestVault(t, "master-pw")
	if err := v.EnablePIN("master-pw", "123456", 14, 3); err != nil {
		t.Fatal(err)
	}

	// First two wrong attempts: ErrWrongPIN, counter decrements.
	for i := 0; i < 2; i++ {
		if err := v.UnlockWithPIN("000000"); !errors.Is(err, ErrWrongPIN) {
			t.Fatalf("attempt %d: want ErrWrongPIN, got %v", i, err)
		}
	}
	if got := v.PINTriesRemaining(); got != 1 {
		t.Fatalf("tries remaining = %d, want 1", got)
	}

	// Third wrong attempt: lockout + PIN wiped.
	if err := v.UnlockWithPIN("000000"); !errors.Is(err, ErrPINLockedOut) {
		t.Fatalf("want ErrPINLockedOut, got %v", err)
	}
	if v.PINEnabled() {
		t.Fatal("PIN must be wiped after lockout")
	}
	// The wipe must be persisted.
	cfg, _ := config.Load(v.cfgPath)
	if cfg.PINEnabled {
		t.Fatal("persisted config must have PIN disabled after lockout")
	}
}

func TestCorrectPINResetsFailCount(t *testing.T) {
	v, _, _ := newPINTestVault(t, "master-pw")
	if err := v.EnablePIN("master-pw", "123456", 14, 5); err != nil {
		t.Fatal(err)
	}
	if err := v.UnlockWithPIN("000000"); !errors.Is(err, ErrWrongPIN) {
		t.Fatalf("want ErrWrongPIN, got %v", err)
	}
	if err := v.UnlockWithPIN("123456"); err != nil {
		t.Fatalf("correct PIN: %v", err)
	}
	if v.cfg.PINFailCount != 0 {
		t.Fatalf("fail count = %d, want 0 after success", v.cfg.PINFailCount)
	}
}

func TestUnlockWithPINExpired(t *testing.T) {
	v, _, _ := newPINTestVault(t, "master-pw")
	if err := v.EnablePIN("master-pw", "123456", 14, 5); err != nil {
		t.Fatal(err)
	}
	// Backdate the last master unlock beyond the interval.
	v.cfg.PINLastMasterUnlock = time.Now().Add(-15 * 24 * time.Hour).Unix()

	if err := v.UnlockWithPIN("123456"); !errors.Is(err, ErrPINExpired) {
		t.Fatalf("want ErrPINExpired, got %v", err)
	}
	// PIN stays configured; a master-password unlock restarts the interval.
	if !v.PINEnabled() {
		t.Fatal("PIN must remain enabled after expiry")
	}
	if err := v.Unlock("master-pw"); err != nil {
		t.Fatalf("master Unlock: %v", err)
	}
	if v.PINPwExpired() {
		t.Fatal("interval should be restarted after a master-password unlock")
	}
}

func TestClampPINIntervalDays(t *testing.T) {
	cases := map[int]int{0: 1, -5: 1, 1: 1, 14: 14, 60: 60, 61: 60, 1000: 60}
	for in, want := range cases {
		if got := ClampPINIntervalDays(in); got != want {
			t.Errorf("ClampPINIntervalDays(%d) = %d, want %d", in, got, want)
		}
	}
}

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

// PIN quick-unlock for the CLI. The PIN is never stored; it derives (Argon2id,
// own salt) a wrap-key that AES-GCM-encrypts a copy of the master key, persisted
// in the config file. A wrong PIN simply fails the GCM auth tag.
//
// SECURITY: the config file is plaintext on disk, so the wrapped key + salt can
// be copied and a 6-digit PIN brute-forced offline. The in-app failure counter
// does NOT stop that. The TUI therefore warns the user when enabling a PIN.

import (
	"crypto/rand"
	"errors"
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/crypto"
)

const (
	pinKDFTime             = 3
	pinKDFMemory           = 64 * 1024
	pinSaltLen             = 16
	DefaultPINMaxTries     = 5
	DefaultPINIntervalDays = 14
	PINIntervalMinDays     = 1
	PINIntervalMaxDays     = 60 // 2 months
)

// ErrPINNotEnabled is returned when a PIN unlock is attempted without a PIN set.
var ErrPINNotEnabled = errors.New("PIN quick-unlock is not enabled")

// ErrPINExpired means the configured re-auth interval elapsed; the master
// password must be entered (the PIN stays configured).
var ErrPINExpired = errors.New("PIN expired — master password required")

// ErrWrongPIN is returned for an incorrect PIN that has not yet exhausted tries.
var ErrWrongPIN = errors.New("incorrect PIN")

// ErrPINLockedOut is returned once max tries are exhausted; the PIN has been
// wiped and the master password is required.
var ErrPINLockedOut = errors.New("too many incorrect PIN attempts — PIN removed, master password required")

// ClampPINIntervalDays clamps a requested re-auth interval to [1, 60] days.
func ClampPINIntervalDays(days int) int {
	if days < PINIntervalMinDays {
		return PINIntervalMinDays
	}
	if days > PINIntervalMaxDays {
		return PINIntervalMaxDays
	}
	return days
}

// PINEnabled reports whether a PIN is configured.
func (v *Vault) PINEnabled() bool { return v.cfg.PINEnabled }

func (v *Vault) pinMaxTries() int {
	if v.cfg.PINMaxTries > 0 {
		return v.cfg.PINMaxTries
	}
	return DefaultPINMaxTries
}

func (v *Vault) pinIntervalDays() int {
	if v.cfg.PINPwIntervalDays > 0 {
		return ClampPINIntervalDays(v.cfg.PINPwIntervalDays)
	}
	return DefaultPINIntervalDays
}

// PINPwExpired reports whether the re-auth interval has elapsed since the last
// master-password unlock.
func (v *Vault) PINPwExpired() bool {
	if v.cfg.PINLastMasterUnlock == 0 {
		return true
	}
	deadline := time.Unix(v.cfg.PINLastMasterUnlock, 0).
		Add(time.Duration(v.pinIntervalDays()) * 24 * time.Hour)
	return time.Now().After(deadline)
}

// PINTriesRemaining returns how many incorrect attempts remain before wipe.
func (v *Vault) PINTriesRemaining() int {
	r := v.pinMaxTries() - v.cfg.PINFailCount
	if r < 0 {
		return 0
	}
	return r
}

// onMasterUnlock restarts the PIN re-auth interval and clears the failure
// counter after any successful master-password unlock. No-op without a PIN.
func (v *Vault) onMasterUnlock() {
	if !v.cfg.PINEnabled {
		return
	}
	v.cfg.PINLastMasterUnlock = time.Now().Unix()
	v.cfg.PINFailCount = 0
	_ = v.cfg.Save(v.cfgPath)
}

// EnablePIN wraps the master key under the PIN and persists the PIN config.
// It requires the master password (re-derives the master key and authorizes the
// change). intervalDays is clamped to [1, 60]; maxTries <= 0 uses the default.
func (v *Vault) EnablePIN(masterPassword, pin string, intervalDays, maxTries int) error {
	masterKey, err := v.deriveMasterKey(masterPassword)
	if err != nil {
		return err
	}
	// Verify the master password before trusting the derived key.
	encPrivX, err := crypto.B64Dec(v.cfg.EncPrivX25519)
	if err != nil {
		return err
	}
	if _, err := crypto.Decrypt(masterKey, encPrivX); err != nil {
		return errors.New("wrong master password")
	}

	pinSalt := make([]byte, pinSaltLen)
	if _, err := rand.Read(pinSalt); err != nil {
		return err
	}
	pinKey := crypto.DeriveKey(pin, &crypto.KDFParams{Salt: pinSalt, Time: pinKDFTime, Memory: pinKDFMemory})
	wrapped, err := crypto.Encrypt(pinKey, masterKey)
	if err != nil {
		return err
	}

	if maxTries <= 0 {
		maxTries = DefaultPINMaxTries
	}
	v.cfg.PINEnabled = true
	v.cfg.PINSalt = crypto.B64Enc(pinSalt)
	v.cfg.PINWrappedMasterKey = crypto.B64Enc(wrapped)
	v.cfg.PINKDFTime = pinKDFTime
	v.cfg.PINKDFMemory = pinKDFMemory
	v.cfg.PINMaxTries = maxTries
	v.cfg.PINFailCount = 0
	v.cfg.PINPwIntervalDays = ClampPINIntervalDays(intervalDays)
	v.cfg.PINLastMasterUnlock = time.Now().Unix()
	return v.cfg.Save(v.cfgPath)
}

// DisablePIN removes all PIN quick-unlock state.
func (v *Vault) DisablePIN() error {
	v.cfg.ClearPIN()
	return v.cfg.Save(v.cfgPath)
}

// UnlockWithPIN unlocks the vault using the PIN. On success the private keys are
// loaded into memory (same as Unlock). On the Nth wrong attempt the PIN is wiped
// (returns ErrPINLockedOut). If the re-auth interval elapsed it returns
// ErrPINExpired without consuming an attempt.
func (v *Vault) UnlockWithPIN(pin string) error {
	if !v.cfg.PINEnabled {
		return ErrPINNotEnabled
	}
	if v.PINPwExpired() {
		return ErrPINExpired
	}

	// Persist the incremented counter BEFORE attempting, so killing the process
	// mid-attempt cannot reset the count and bypass the lockout.
	v.cfg.PINFailCount++
	_ = v.cfg.Save(v.cfgPath)

	pinSalt, err := crypto.B64Dec(v.cfg.PINSalt)
	if err != nil {
		return err
	}
	wrapped, err := crypto.B64Dec(v.cfg.PINWrappedMasterKey)
	if err != nil {
		return err
	}
	pinKey := crypto.DeriveKey(pin, &crypto.KDFParams{
		Salt:   pinSalt,
		Time:   uint32(v.cfg.PINKDFTime),
		Memory: uint32(v.cfg.PINKDFMemory),
	})
	masterKey, err := crypto.Decrypt(pinKey, wrapped)
	if err != nil {
		// Wrong PIN. Wipe once tries are exhausted.
		if v.cfg.PINFailCount >= v.pinMaxTries() {
			_ = v.DisablePIN()
			return ErrPINLockedOut
		}
		_ = v.cfg.Save(v.cfgPath)
		return ErrWrongPIN
	}

	if err := v.decryptPrivKeys(masterKey); err != nil {
		return err
	}

	// Success: reset the failure counter (the interval is NOT reset — only a
	// real master-password unlock restarts it).
	v.cfg.PINFailCount = 0
	_ = v.cfg.Save(v.cfgPath)
	return nil
}

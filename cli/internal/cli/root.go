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
	"fmt"
	"os"

	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
)

var cfg *config.Config
var v *vault.Vault

var rootCmd = &cobra.Command{
	Use:     "pwmgr",
	Short:   "Passbubble CLI — E2E encrypted password manager",
	Version: "2.0.0",
	Long: `Passbubble CLI — End-to-End Encrypted Password Manager

Connects to a self-hosted Passbubble server. Run 'pwmgr login' first.

Usage:
  pwmgr           Launch interactive TUI (default)
  pwmgr login     Authenticate against your server
  pwmgr add       Add a new password
  pwmgr get       Retrieve a password
  pwmgr list      List all entries
  pwmgr generate  Generate a secure password`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Launch the TUI directly; it handles login / unlock on its own.
		return runTUI(v)
	},
}

// Execute is the CLI entry point.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/pwmgr/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}

func initConfig() {
	path := config.ConfigPath(cfgFile)
	var err error
	cfg, err = config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load config: %v\n", err)
		cfg = &config.Config{}
	}
	if cfg.ServerURL != "" {
		v = vault.New(cfg, path)
	}
}

// ensureAuthenticated refreshes the access token.
// Called by commands that need an active session.
func ensureAuthenticated() error {
	if v == nil {
		return fmt.Errorf("not logged in")
	}
	return v.Authenticate()
}

// ensureUnlocked authenticates and decrypts private keys.
// Commands that need E2E crypto (read/write entries) call this.
func ensureUnlocked() error {
	if err := ensureAuthenticated(); err != nil {
		return err
	}
	if !v.IsUnlocked() {
		password, err := promptPassword("Master password: ")
		if err != nil {
			return err
		}
		if err := v.Unlock(password); err != nil {
			return err
		}
	}
	return nil
}

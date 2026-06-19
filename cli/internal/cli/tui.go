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
	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/tui"
	vaultpkg "github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/Gerry3010/passbubble/cli/pkg/keyring"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(tuiCmd)
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch interactive TUI",
	RunE: func(cmd *cobra.Command, args []string) error {
		// The TUI handles login / unlock itself, so no ensureUnlocked() here.
		return runTUI(v)
	},
}

// runTUI wires the vault adapter into the keyring shim (if logged in) and starts the TUI.
// The vault may be nil (not logged in) — the TUI shows its login screen in that case.
func runTUI(vlt *vaultpkg.Vault) error {
	if vlt != nil {
		// Register the vault adapter so the TUI's keyring.New() calls work.
		keyring.SetGlobal(vaultpkg.NewKeyringAdapter(vlt))
	}
	return tui.StartTUI(vlt, cfg, config.ConfigPath(cfgFile))
}

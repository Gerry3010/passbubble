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
	"strings"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
	"github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.Flags().StringP("server", "s", "http://localhost:8765", "Server URL")
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "First-run setup: register the initial admin account",
	Long: `Interactive first-run setup wizard.

Registers the first admin account on a fresh Passbubble server.
No invitation token is required — the first account is automatically
promoted to admin (bootstrap mode).

Run this once after 'docker compose up -d'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL, _ := cmd.Flags().GetString("server")
		serverURL = strings.TrimRight(serverURL, "/")

		fmt.Println()
		fmt.Println("  Welcome to Passbubble!")
		fmt.Println("  ─────────────────────────────────────────────")
		fmt.Println("  This wizard registers your initial admin account.")
		fmt.Println("  The first account on a fresh server is automatically admin.")
		fmt.Println()
		fmt.Printf("  Server: %s\n", serverURL)
		fmt.Println()

		// Confirm server is reachable
		client := apiclient.New(serverURL)
		if err := client.Health(); err != nil {
			return fmt.Errorf("cannot reach server at %s: %w\nIs 'docker compose up -d' running?", serverURL, err)
		}
		fmt.Println("  ✓ Server reachable")
		fmt.Println()

		// Collect user details
		name, err := promptInput("  Display name:       ")
		if err != nil {
			return err
		}
		if name == "" {
			return fmt.Errorf("display name is required")
		}

		email, err := promptInput("  Email:              ")
		if err != nil {
			return err
		}
		if email == "" {
			return fmt.Errorf("email is required")
		}

		fmt.Println()
		fmt.Println("  Choose a strong master password. This password encrypts your")
		fmt.Println("  private keys — it is never sent to the server.")
		fmt.Println()

		password, err := promptPassword("  Master password:    ")
		if err != nil {
			return err
		}
		if len(password) < 12 {
			return fmt.Errorf("master password must be at least 12 characters")
		}
		password2, err := promptPassword("  Confirm password:   ")
		if err != nil {
			return err
		}
		if password != password2 {
			return fmt.Errorf("passwords do not match")
		}

		fmt.Println()
		fmt.Print("  Generating cryptographic keys... ")

		privX25519, pubX25519, err := crypto.GenerateX25519()
		if err != nil {
			return fmt.Errorf("generate x25519: %w", err)
		}
		privMLKEM, pubMLKEM, err := crypto.GenerateMLKEM768()
		if err != nil {
			return fmt.Errorf("generate mlkem768: %w", err)
		}

		kdfParams, err := crypto.NewKDFParams()
		if err != nil {
			return err
		}
		masterKey := crypto.DeriveKey(password, kdfParams)

		encPrivX25519, err := crypto.Encrypt(masterKey, privX25519)
		if err != nil {
			return err
		}
		encPrivMLKEM, err := crypto.Encrypt(masterKey, privMLKEM)
		if err != nil {
			return err
		}
		fmt.Println("done")

		fmt.Print("  Registering admin account...     ")

		resp, err := client.Register(apiclient.RegisterRequest{
			Email:           email,
			Name:            name,
			Password:        password,
			InvitationToken: "", // empty = bootstrap (first user)
			PubX25519:       crypto.B64Enc(pubX25519),
			PubMLKEM768:     crypto.B64Enc(pubMLKEM),
			EncPrivX25519:   crypto.B64Enc(encPrivX25519),
			EncPrivMLKEM768: crypto.B64Enc(encPrivMLKEM),
			KDFSalt:         crypto.B64Enc(kdfParams.Salt),
		})
		if err != nil {
			return fmt.Errorf("registration failed: %w", err)
		}
		fmt.Println("done")

		// Save config
		cfgPath := config.ConfigPath(cfgFile)
		newCfg := &config.Config{
			ServerURL:       serverURL,
			UserID:          resp.UserID,
			Email:           resp.Email,
			RefreshToken:    resp.RefreshToken,
			PubX25519:       crypto.B64Enc(pubX25519),
			PubMLKEM768:     crypto.B64Enc(pubMLKEM),
			EncPrivX25519:   crypto.B64Enc(encPrivX25519),
			EncPrivMLKEM768: crypto.B64Enc(encPrivMLKEM),
			KDFSalt:         crypto.B64Enc(kdfParams.Salt),
			KDFTime:         int(kdfParams.Time),
			KDFMemory:       int(kdfParams.Memory),
		}
		if err := newCfg.Save(cfgPath); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		cfg = newCfg
		v = vault.New(cfg, cfgPath)

		fmt.Println()
		fmt.Println("  ─────────────────────────────────────────────")
		fmt.Printf("  ✓ Admin account created!\n")
		fmt.Println("  ─────────────────────────────────────────────")
		fmt.Println()
		fmt.Printf("  Name:   %s\n", name)
		fmt.Printf("  Email:  %s\n", email)
		fmt.Printf("  Role:   %s\n", resp.Role)
		fmt.Printf("  Server: %s\n", serverURL)
		fmt.Println()
		fmt.Println("  Next steps:")
		fmt.Println("    pwmgr add 'My Password'       — add your first entry")
		fmt.Println("    pwmgr tui                     — open the interactive TUI")
		fmt.Println("    pwmgr register --server ...   — invite more users")
		fmt.Println()

		return nil
	},
}

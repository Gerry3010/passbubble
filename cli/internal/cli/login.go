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
	"strings"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/config"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
	"github.com/Gerry3010/passbubble/cli/internal/vault"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(whoamiCmd)
	rootCmd.AddCommand(registerCmd)

	registerCmd.Flags().StringP("token", "t", "", "Invitation token")
	registerCmd.Flags().StringP("name", "n", "", "Display name")
	loginCmd.Flags().StringP("server", "s", "", "Server URL (e.g. https://pass.example.com)")
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to your Passbubble server",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL, _ := cmd.Flags().GetString("server")
		if serverURL == "" && cfg != nil && cfg.ServerURL != "" {
			serverURL = cfg.ServerURL
		}
		if serverURL == "" {
			var err error
			serverURL, err = promptInput("Server URL: ")
			if err != nil {
				return err
			}
		}
		serverURL = strings.TrimRight(serverURL, "/")

		email, err := promptInput("Email: ")
		if err != nil {
			return err
		}
		password, err := promptPassword("Master password: ")
		if err != nil {
			return err
		}

		client := apiclient.New(serverURL)
		resp, err := client.Login(apiclient.LoginRequest{Email: email, Password: password})
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		cfgPath := config.ConfigPath(cfgFile)
		newCfg := &config.Config{
			ServerURL:       serverURL,
			UserID:          resp.UserID,
			Email:           resp.Email,
			RefreshToken:    resp.RefreshToken,
			PubX25519:       resp.PubX25519,
			PubMLKEM768:     resp.PubMLKEM768,
			EncPrivX25519:   resp.EncPrivX25519,
			EncPrivMLKEM768: resp.EncPrivMLKEM768,
			KDFSalt:         resp.KDFSalt,
			KDFTime:         resp.KDFTime,
			KDFMemory:       resp.KDFMemory,
		}
		if err := newCfg.Save(cfgPath); err != nil {
			return fmt.Errorf("save config: %w", err)
		}

		// Update in-process state
		cfg = newCfg
		v = vault.New(cfg, cfgPath)

		fmt.Printf("Logged in as %s (%s)\n", resp.Name, resp.Email)
		fmt.Printf("Role: %s\n", resp.Role)
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out of your Passbubble server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg == nil || !cfg.IsLoggedIn() {
			fmt.Println("Not logged in.")
			return nil
		}
		if err := ensureAuthenticated(); err == nil {
			_ = v.Client().Logout(cfg.RefreshToken)
		}
		cfg.Clear()
		cfgPath := config.ConfigPath(cfgFile)
		if err := cfg.Save(cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not clear config: %v\n", err)
		}
		fmt.Println("Logged out.")
		return nil
	},
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current user info",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAuthenticated(); err != nil {
			return err
		}
		me, err := v.Client().Me()
		if err != nil {
			return err
		}
		fmt.Printf("User:   %s\n", me.Name)
		fmt.Printf("Email:  %s\n", me.Email)
		fmt.Printf("Role:   %s\n", me.Role)
		fmt.Printf("Server: %s\n", cfg.ServerURL)
		return nil
	},
}

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a new account (requires invitation token)",
	RunE: func(cmd *cobra.Command, args []string) error {
		serverURL, _ := cmd.Flags().GetString("server")
		if serverURL == "" {
			var err error
			serverURL, err = promptInput("Server URL: ")
			if err != nil {
				return err
			}
		}
		serverURL = strings.TrimRight(serverURL, "/")

		invToken, _ := cmd.Flags().GetString("token")
		if invToken == "" {
			var err error
			invToken, err = promptInput("Invitation token: ")
			if err != nil {
				return err
			}
		}
		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			var err error
			name, err = promptInput("Display name: ")
			if err != nil {
				return err
			}
		}
		email, err := promptInput("Email: ")
		if err != nil {
			return err
		}
		password, err := promptPassword("Master password: ")
		if err != nil {
			return err
		}
		password2, err := promptPassword("Confirm master password: ")
		if err != nil {
			return err
		}
		if password != password2 {
			return fmt.Errorf("passwords do not match")
		}

		fmt.Print("Generating keys... ")
		// Generate key material
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

		client := apiclient.New(serverURL)
		resp, err := client.Register(apiclient.RegisterRequest{
			Email:           email,
			Name:            name,
			Password:        password,
			InvitationToken: invToken,
			PubX25519:       crypto.B64Enc(pubX25519),
			PubMLKEM768:     crypto.B64Enc(pubMLKEM),
			EncPrivX25519:   crypto.B64Enc(encPrivX25519),
			EncPrivMLKEM768: crypto.B64Enc(encPrivMLKEM),
			KDFSalt:         crypto.B64Enc(kdfParams.Salt),
		})
		if err != nil {
			return fmt.Errorf("registration failed: %w", err)
		}

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
		fmt.Printf("Registered and logged in as %s (%s)\n", name, email)
		return nil
	},
}

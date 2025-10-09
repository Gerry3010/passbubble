package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/gerry/password-manager/pkg/generator"
	"github.com/gerry/password-manager/pkg/keyring"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var passwordCmds = []*cobra.Command{addCmd, getCmd, listCmd, updateCmd, deleteCmd}

func init() {
	// Add all password commands to root
	for _, cmd := range passwordCmds {
		rootCmd.AddCommand(cmd)
	}
}

// addCmd adds a new password
var addCmd = &cobra.Command{
	Use:   "add <service> [username]",
	Short: "Add a new password entry",
	Long: `Add a new password entry to the keyring.

If no password is provided, a secure password will be generated automatically.
The service name is required, while username is optional.

Examples:
  pwmgr add gmail john.doe@gmail.com
  pwmgr add github
  pwmgr add banking myusername`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]
		var username string
		if len(args) > 1 {
			username = args[1]
		}

		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		// Check if password already exists
		if _, err := kr.Get(service, username); err == nil {
			displayName := service
			if username != "" {
				displayName = fmt.Sprintf("%s (%s)", service, username)
			}
			return fmt.Errorf("password for %s already exists. Use 'update' command to modify it", displayName)
		}

		// Get password from user
		fmt.Printf("Enter password for %s", service)
		if username != "" {
			fmt.Printf(" (%s)", username)
		}
		fmt.Print(" (leave empty to generate): ")

		password, err := readPassword()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}

		if strings.TrimSpace(password) == "" {
			// Generate password
			gen := generator.New(generator.DefaultOptions())
			passwords, err := gen.Generate()
			if err != nil {
				return fmt.Errorf("failed to generate password: %w", err)
			}
			password = passwords[0]
			fmt.Printf("Generated password: %s\n", password)
		}

		// Store password
		if err := kr.Store(service, username, password); err != nil {
			return fmt.Errorf("failed to store password: %w", err)
		}

		displayName := service
		if username != "" {
			displayName = fmt.Sprintf("%s (%s)", service, username)
		}
		fmt.Printf("✓ Password stored for %s\n", displayName)
		return nil
	},
}

// getCmd retrieves a password
var getCmd = &cobra.Command{
	Use:   "get <service> [username]",
	Short: "Retrieve a password",
	Long: `Retrieve a password from the keyring.

The service name is required, while username is optional.

Examples:
  pwmgr get gmail
  pwmgr get gmail john.doe@gmail.com`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]
		var username string
		if len(args) > 1 {
			username = args[1]
		}

		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		password, err := kr.Get(service, username)
		if err != nil {
			displayName := service
			if username != "" {
				displayName = fmt.Sprintf("%s (%s)", service, username)
			}
			return fmt.Errorf("no password found for %s", displayName)
		}

		fmt.Println(password)
		return nil
	},
}

// listCmd lists all passwords
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stored services",
	Long: `List all password entries stored in the keyring.

Shows service names and associated usernames if available.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		entries, err := kr.List()
		if err != nil {
			return fmt.Errorf("failed to list passwords: %w", err)
		}

		if len(entries) == 0 {
			fmt.Println("No passwords stored yet. Use 'add' command to store your first password.")
			return nil
		}

		fmt.Println("Stored password entries:")
		fmt.Println()
		for _, entry := range entries {
			if entry.Username != "" {
				fmt.Printf("  %s (%s)\n", entry.Service, entry.Username)
			} else {
				fmt.Printf("  %s\n", entry.Service)
			}
		}
		return nil
	},
}

// updateCmd updates an existing password
var updateCmd = &cobra.Command{
	Use:   "update <service> [username]",
	Short: "Update an existing password",
	Long: `Update an existing password entry in the keyring.

If no password is provided, a secure password will be generated automatically.
The service name is required, while username is optional.

Examples:
  pwmgr update gmail john.doe@gmail.com
  pwmgr update github`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]
		var username string
		if len(args) > 1 {
			username = args[1]
		}

		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		// Check if password exists
		if _, err := kr.Get(service, username); err != nil {
			displayName := service
			if username != "" {
				displayName = fmt.Sprintf("%s (%s)", service, username)
			}
			return fmt.Errorf("no password found for %s. Use 'add' command to create it", displayName)
		}

		// Get new password from user
		fmt.Printf("Enter new password for %s", service)
		if username != "" {
			fmt.Printf(" (%s)", username)
		}
		fmt.Print(" (leave empty to generate): ")

		password, err := readPassword()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}

		if strings.TrimSpace(password) == "" {
			// Generate password
			gen := generator.New(generator.DefaultOptions())
			passwords, err := gen.Generate()
			if err != nil {
				return fmt.Errorf("failed to generate password: %w", err)
			}
			password = passwords[0]
			fmt.Printf("Generated password: %s\n", password)
		}

		// Store updated password
		if err := kr.Store(service, username, password); err != nil {
			return fmt.Errorf("failed to update password: %w", err)
		}

		displayName := service
		if username != "" {
			displayName = fmt.Sprintf("%s (%s)", service, username)
		}
		fmt.Printf("✓ Password updated for %s\n", displayName)
		return nil
	},
}

// deleteCmd deletes a password
var deleteCmd = &cobra.Command{
	Use:   "delete <service> [username]",
	Short: "Delete a password entry",
	Long: `Delete a password entry from the keyring.

This action requires confirmation and cannot be undone.
The service name is required, while username is optional.

Examples:
  pwmgr delete gmail john.doe@gmail.com
  pwmgr delete github`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]
		var username string
		if len(args) > 1 {
			username = args[1]
		}

		kr := keyring.New()
		if !kr.IsAvailable() {
			return fmt.Errorf("secret-tool is not available. Please install libsecret-tools package")
		}

		// Check if password exists
		if _, err := kr.Get(service, username); err != nil {
			displayName := service
			if username != "" {
				displayName = fmt.Sprintf("%s (%s)", service, username)
			}
			return fmt.Errorf("no password found for %s", displayName)
		}

		// Confirm deletion
		displayName := service
		if username != "" {
			displayName = fmt.Sprintf("%s (%s)", service, username)
		}
		fmt.Printf("Are you sure you want to delete the password for %s? [y/N]: ", displayName)

		reader := bufio.NewReader(os.Stdin)
		confirmation, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}

		confirmation = strings.TrimSpace(strings.ToLower(confirmation))
		if confirmation != "y" && confirmation != "yes" {
			fmt.Println("Operation cancelled")
			return nil
		}

		// Delete password
		if err := kr.Delete(service, username); err != nil {
			return fmt.Errorf("failed to delete password: %w", err)
		}

		fmt.Printf("✓ Password deleted for %s\n", displayName)
		return nil
	},
}

// readPassword reads a password from stdin without echoing
func readPassword() (string, error) {
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	fmt.Println() // Add newline after password input
	return string(bytePassword), nil
}

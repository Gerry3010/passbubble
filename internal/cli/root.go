package cli

import (
	"fmt"
	"os"

	"github.com/Gerry3010/passbubble/internal/tui"
	"github.com/Gerry3010/passbubble/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	verbose bool
	quiet   bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "pwmgr",
	Short:   "A secure password manager with interactive TUI",
	Long: `Password Manager - Go Edition

A comprehensive password manager with both CLI and TUI interfaces.
Built using GNOME Keyring (Linux), Keychain (macOS), or Credential Manager (Windows).

Features:
- 🔐 Interactive Bubble Tea TUI interface (default)
- 🔑 Secure password storage using system keyring
- 🔓 TOTP 2FA support with live code generation
- 🎲 Advanced password generation with multiple types
- 💾 Encrypted backup and restore functionality
- 🔍 Search and organization capabilities
- 🌐 Cross-platform compatibility

Usage:
  pwmgr           Launch interactive TUI (default)
  pwmgr [command] Use specific CLI command
  pwmgr help      Show all available commands`,
	Version: version.GetInfo().Short(),
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: launch TUI
		if err := tui.StartTUI(); err != nil {
			cmd.PrintErrf("Error launching TUI: %v\n", err)
			cmd.PrintErrln("Use 'pwmgr help' to see available CLI commands.")
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/pwmgr/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "quiet mode (minimal output)")

	// Bind flags to viper
	if err := viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to bind verbose flag: %v\n", err)
	}
	if err := viper.BindPFlag("quiet", rootCmd.PersistentFlags().Lookup("quiet")); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to bind quiet flag: %v\n", err)
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".pwmgr" (without extension).
		configDir := fmt.Sprintf("%s/.config/pwmgr", home)
		viper.AddConfigPath(configDir)
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil && verbose {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

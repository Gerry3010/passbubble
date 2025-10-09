package cli

import (
	"github.com/Gerry3010/passbubble/internal/tui"
	"github.com/spf13/cobra"
)

// tuiCmd represents the interactive TUI command
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch interactive terminal user interface",
	Long: `Launch the interactive Bubble Tea TUI for managing passwords and TOTP codes.

The TUI provides a visual interface for:
- Browsing all stored entries (passwords, TOTP secrets, etc.)
- Viewing entry details with live TOTP code generation
- Managing backups with visual selection
- Navigating with keyboard shortcuts

Navigation:
  ↑/↓ or j/k    Navigate lists
  Enter         View entry details
  s             Show/hide secrets in detail view
  a             Add new entry (placeholder)
  e             Edit entry (placeholder)
  d             Delete entry (placeholder)
  c             Create backup (placeholder)
  b             View backup management screen
  r/f           Refresh current view
  esc/q         Go back or quit

Security Note:
For security reasons, actual passwords are not displayed in the TUI.
Use the CLI commands to safely copy passwords to clipboard.`,
	RunE: runTUI,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

func runTUI(cmd *cobra.Command, args []string) error {
	return tui.StartTUI()
}
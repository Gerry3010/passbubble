package cli

import (
	"encoding/json"
	"fmt"

	"github.com/gerry/password-manager/internal/version"
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long: `Display detailed version information including:
- Version number
- Build time
- Commit hash
- Go version used
- Target platform`,
	Run: runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output version information in JSON format")
}

func runVersion(cmd *cobra.Command, args []string) {
	info := version.GetInfo()

	if jsonOutput {
		jsonData, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			fmt.Printf("Error marshaling version info: %v\n", err)
			return
		}
		fmt.Println(string(jsonData))
	} else {
		fmt.Printf("Password Manager %s\n\n", info.Version)
		fmt.Printf("Build Information:\n")
		fmt.Printf("  Version:     %s\n", info.Version)
		fmt.Printf("  Build Time:  %s\n", info.BuildTime)
		fmt.Printf("  Commit Hash: %s\n", info.CommitHash)
		fmt.Printf("  Go Version:  %s\n", info.GoVersion)
		fmt.Printf("  Platform:    %s\n", info.Platform)
	}
}

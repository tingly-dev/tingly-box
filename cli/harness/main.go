// Package main provides the CLI harness for protocol validation testing.
//
// The harness provides three testing modes:
//   - matrix: Virtual provider e2e tests (protocol transformations)
//   - profile: Real profile-based e2e tests (config, agent interaction) - Phase 2
//   - provider: Real provider API e2e tests (live API compatibility) - Phase 3
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Build information variables (set via ldflags)
var (
	version   = "dev"
	gitCommit = "unknown"
	buildTime = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "harness",
	Short: "Tingly-Box Protocol Validation Harness",
	Long: `Test harness for Tingly-Box protocol validation.

Provides three testing modes:
  matrix    - Virtual provider e2e tests (protocol transformations)
  profile   - Real profile-based e2e tests (config, agent interaction)
  provider  - Real provider API e2e tests (live API compatibility)

Run 'harness <command> --help' for command-specific usage.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default: show help
		return cmd.Help()
	},
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, gitCommit, buildTime),
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		// Exit with code 1 for any error
		os.Exit(1)
	}
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(newMatrixCommand())
	rootCmd.AddCommand(newProfileCommand())
	rootCmd.AddCommand(newProviderCommand())
}

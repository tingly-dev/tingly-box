package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/pkg/fs"
)

var rootCmd = &cobra.Command{
	Use:   "tingly-box",
	Short: "Tingly Box - Provider-agnostic Desktop AI Model Proxy and Key Manager",
	Long: `Tingly Box is a provider-agnostic desktop AI model proxy and key manager.
It provides a unified OpenAI-compatible endpoint that routes requests to multiple
AI providers, with flexible configuration and secure credential management.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to start command when no subcommand is provided
		startCmd := command.StartCommand(appManager)
		startCmd.SetArgs([]string{})
		if err := startCmd.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")
		// Apply priority: CLI flag > Config > Default
		if !cmd.Flags().Changed("verbose") && appConfig != nil {
			verbose = appConfig.GetVerbose()
		}
		if verbose {
			logrus.SetLevel(logrus.TraceLevel)
		}

		return nil
	},
}

// Build information variables
var (
	// Set by compiler via -ldflags
	version   = "dev"
	gitCommit = "unknown"
	buildTime = "unknown"
	goVersion = "unknown"
	platform  = "unknown"

	// Global configuration directory flag
	configDir string

	// Global config and app manager instances
	appConfig  *config.AppConfig
	appManager *command.AppManager
)

func init() {
	// Add global flags FIRST before parsing
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", "", "configuration directory (default: ~/.tingly-box)")

	// Parse flags early to get config-dir before adding subcommands
	if err := rootCmd.ParseFlags(os.Args[1:]); err != nil {
		// Flags will be parsed again later, so ignore errors here
	}

	// Initialize config based on parsed flags
	var err error
	if configDir != "" {
		expandedDir, expandErr := fs.ExpandConfigDir(configDir)
		if expandErr == nil {
			appConfig, err = config.NewAppConfig(config.WithConfigDir(expandedDir))
		} else {
			err = expandErr
		}
	}
	if appConfig == nil && err == nil {
		appConfig, err = config.NewAppConfig()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize config: %v\n", err)
		os.Exit(1)
	}
	if appConfig != nil {
		appConfig.SetVersion(version)
		// Create AppManager from the AppConfig
		appManager = command.NewAppManagerWithConfig(appConfig)
	}

	// Add version command (doesn't need config)
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Tingly Box CLI\n")
			fmt.Printf("Version:    %s\n", version)
			fmt.Printf("Git Commit: %s\n", gitCommit)
			fmt.Printf("Build Time: %s\n", buildTime)
			fmt.Printf("Go Version: %s\n", goVersion)
			fmt.Printf("Platform:   %s\n", platform)
		},
	}
	rootCmd.AddCommand(versionCmd)

	// Add subcommands with initialized appManager
	rootCmd.AddCommand(command.AddCommand(appManager))
	rootCmd.AddCommand(command.ListCommand(appManager))
	rootCmd.AddCommand(command.DeleteCommand(appManager))
	rootCmd.AddCommand(command.ExportCommand(appManager))
	rootCmd.AddCommand(command.ImportCommand(appManager))
	rootCmd.AddCommand(command.StartCommand(appManager))
	rootCmd.AddCommand(command.StopCommand(appManager))
	rootCmd.AddCommand(command.RestartCommand(appManager))
	rootCmd.AddCommand(command.StatusCommand(appManager))
	rootCmd.AddCommand(command.RemoteCoderCommand(appManager))
	rootCmd.AddCommand(command.QuickstartCommand(appManager))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		// Only print usage for Cobra CLI errors (flag/argument errors), not runtime errors
		if isCobraFlagError(err) {
			rootCmd.PrintErrln(rootCmd.UsageString())
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// isCobraFlagError checks if the error is a Cobra CLI parsing error
func isCobraFlagError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Cobra flag/argument errors contain these patterns
	return strings.Contains(errStr, "unknown flag") ||
		strings.Contains(errStr, "unknown shorthand flag") ||
		strings.Contains(errStr, "flag needs an argument") ||
		strings.Contains(errStr, "bad flag syntax") ||
		strings.Contains(errStr, "unknown command") ||
		strings.Contains(errStr, "too many arguments") ||
		strings.Contains(errStr, "required flag(s)")
}

package main

import (
	"fmt"
	"os"
	"tingly-box/pkg/fs"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"tingly-box/internal/command"
	"tingly-box/internal/config"
)

var rootCmd = &cobra.Command{
	Use:   "tingly-box",
	Short: "Tingly Box - Provider-agnostic Desktop AI Model Proxy and Key Manager",
	Long: `Tingly Box is a provider-agnostic desktop AI model proxy and key manager.
It provides a unified OpenAI-compatible endpoint that routes requests to multiple
AI providers, with flexible configuration and secure credential management.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to start command when no subcommand is provided
		startCmd := command.StartCommand(appConfig)
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

	// Global config instance
	appConfig *config.AppConfig
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

	// Add subcommands with initialized config
	rootCmd.AddCommand(command.AddCommand(appConfig))
	rootCmd.AddCommand(command.ListCommand(appConfig))
	rootCmd.AddCommand(command.DeleteCommand(appConfig))
	rootCmd.AddCommand(command.ImportCommand(appConfig))
	rootCmd.AddCommand(command.StartCommand(appConfig))
	rootCmd.AddCommand(command.StopCommand(appConfig))
	rootCmd.AddCommand(command.RestartCommand(appConfig))
	rootCmd.AddCommand(command.StatusCommand(appConfig))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

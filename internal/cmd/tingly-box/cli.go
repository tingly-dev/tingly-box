package main

import (
	"fmt"
	"os"
	"tingly-box/internal/cli"
	"tingly-box/internal/config"
	"tingly-box/internal/util"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "tingly-box",
	Short: "Tingly Box - Provider-agnostic Desktop AI Model Proxy and Key Manager",
	Long: `Tingly Box is a provider-agnostic desktop AI model proxy and key manager.
It provides a unified OpenAI-compatible endpoint that routes requests to multiple
AI providers, with flexible configuration and secure credential management.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to start command when no subcommand is provided
		startCmd := cli.StartCommand(appConfig)
		startCmd.SetArgs([]string{})
		if err := startCmd.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Skip PersistentPreRun for root command's default Run
		// Root command has no parent, so check if Parent() is nil
		if cmd.Parent() == nil {
			return
		}
		verbose, _ := cmd.Flags().GetBool("verbose")
		// Apply priority: CLI flag > Config > Default
		if !cmd.Flags().Changed("verbose") && appConfig != nil {
			verbose = appConfig.GetVerbose()
		}
		if verbose {
			logrus.SetLevel(logrus.TraceLevel)
		}
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
	// Add global flags
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", "", "configuration directory (default: ~/.tingly-box)")

	// Add version command
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

	// Initialize app config with optional custom config directory
	var err error

	if configDir != "" {
		// Expand and use custom config directory
		expandedDir, err := util.ExpandConfigDir(configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error expanding config directory path: %v\n", err)
			os.Exit(1)
		}
		appConfig, err = config.NewAppConfig(config.WithConfigDir(expandedDir))
	} else {
		// Use default config directory
		appConfig, err = config.NewAppConfig()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing config: %v\n", err)
		os.Exit(1)
	}

	appConfig.SetVersion(version)

	// Add subcommands
	rootCmd.AddCommand(cli.AddCommand(appConfig))
	rootCmd.AddCommand(cli.ListCommand(appConfig))
	rootCmd.AddCommand(cli.DeleteCommand(appConfig))
	rootCmd.AddCommand(cli.StartCommand(appConfig))
	rootCmd.AddCommand(cli.StopCommand(appConfig))
	rootCmd.AddCommand(cli.RestartCommand(appConfig))
	rootCmd.AddCommand(cli.StatusCommand(appConfig))
	//rootCmd.AddCommand(cli.TokenCommand(appConfig))
	//rootCmd.AddCommand(cli.ShellCommand(appConfig))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

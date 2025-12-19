package main

import (
	"fmt"
	"os"
	"tingly-box/internal/cli"
	"tingly-box/internal/config"
	"tingly-box/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "tingly-box",
	Short: "Tingly Box - AI Service API Key Management and Proxy CLI",
	Long: `Tingly Box is a CLI tool for managing AI service API keys and acting as a proxy.
It provides a unified OpenAI-compatible endpoint that routes requests to configured
AI providers with dynamic configuration management.`,
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

	gin.SetMode(gin.ReleaseMode)

	// Initialize app config with optional custom config directory
	var appConfig *config.AppConfig
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

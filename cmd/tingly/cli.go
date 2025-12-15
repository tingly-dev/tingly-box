package main

import (
	"fmt"
	"os"

	"tingly-box/internal/cli"
	"tingly-box/internal/config"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "tingly",
	Short: "Tingly Box - AI Provider Management CLI",
	Long: `Tingly Box is a CLI tool and server for managing multiple AI model providers.
It provides a unified OpenAI-compatible endpoint that routes requests to configured
AI providers with dynamic configuration management.`,
}

func init() {
	// Add global flags
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")

	// Initialize app config
	appConfig, err := config.NewAppConfig()
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
	rootCmd.AddCommand(cli.TokenCommand(appConfig))
	rootCmd.AddCommand(cli.ShellCommand(appConfig))
	rootCmd.AddCommand(cli.UICommand(appConfig))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

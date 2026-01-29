package main

import (
	"log"
	"os"

	appcommand "github.com/tingly-dev/tingly-box/gui/wails3/command"
	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/pkg/fs"
)

func init() {
	// Register a custom event whose associated data type is string.
	// This is not required, but the binding generator will pick up registered events
	// and provide a strongly typed JS/TS API for them.
	// application.RegisterEvent[string]("time")
}

// main function serves as the application's entry point. It uses cobra commands
// to handle CLI arguments and launches the appropriate GUI mode.
func main() {
	// Create AppManager
	home, err := fs.GetUserPath()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
	}
	configDir := home + "/.tingly-box"
	appManager, err := command.NewAppManager(configDir)
	if err != nil {
		log.Fatalf("Failed to create app manager: %v", err)
	}

	// Create root command with app launcher
	launcher := NewAppLauncher()
	rootCmd := appcommand.RootCommand(appManager, launcher)

	// Default to "gui" subcommand if no args provided
	if len(os.Args) == 1 {
		rootCmd.SetArgs([]string{"gui"})
	}

	// Execute the command
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

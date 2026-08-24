package main

import (
	"log"
	"os"
	"runtime"
	"strings"

	appcommand "github.com/tingly-dev/tingly-box/gui/wails3/command"
	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/pkg/fs"
)

// Build metadata injected via -ldflags "-X main.version=..." (see
// VERSION_LDFLAGS in Taskfile.wails.yml). Same -X targets as cli/tingly-box.
var (
	version   = "dev"
	gitCommit = "unknown"
	buildTime = "unknown"
)

// main function serves as the application's entry point. It uses cobra commands
// to handle CLI arguments and launches the appropriate GUI mode.
func main() {
	command.BuildVersion = version
	command.BuildGitCommit = gitCommit
	command.BuildBuildTime = buildTime
	command.BuildGoVersion = strings.TrimPrefix(runtime.Version(), "go")
	command.BuildPlatform = runtime.GOOS + "/" + runtime.GOARCH

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

	// Default to "tray" subcommand if no args provided
	if len(os.Args) == 1 {
		rootCmd.SetArgs([]string{"tray"})
	}

	// Execute the command
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

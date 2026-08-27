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

// resolveConfigDir picks the config directory for this process, mirroring
// the CLI's `--config-dir` (cli/tingly-box/main.go, Kong-based). Cobra's
// rootCmd flags aren't parsed until rootCmd.Execute(), but AppManager needs
// the dir up front — so this scans os.Args directly, same trick the CLI's
// Kong parser effectively gets for free by parsing before dispatch.
//
// Lets a second GUI instance run alongside the default one for debugging,
// e.g.:
//
//	tingly-box-gui gui --config-dir ./.tingly-box-debug --port 12333
//
// Each config dir has its own file lock (see gui/wails3/run.go's
// acquireSingleInstanceLock), so two instances pointed at different dirs
// don't trip each other's single-instance check.
func resolveConfigDir() (string, error) {
	for i, arg := range os.Args {
		if arg == "--config-dir" && i+1 < len(os.Args) {
			return fs.ExpandConfigDir(os.Args[i+1])
		}
		if rest, ok := strings.CutPrefix(arg, "--config-dir="); ok {
			return fs.ExpandConfigDir(rest)
		}
	}
	if envDir := os.Getenv("TINGLY_CONFIG_DIR"); envDir != "" {
		return fs.ExpandConfigDir(envDir)
	}

	home, err := fs.GetUserPath()
	if err != nil {
		return "", err
	}
	return home + "/.tingly-box", nil
}

// main function serves as the application's entry point. It uses cobra commands
// to handle CLI arguments and launches the appropriate GUI mode.
func main() {
	command.BuildVersion = version
	command.BuildGitCommit = gitCommit
	command.BuildBuildTime = buildTime
	command.BuildGoVersion = strings.TrimPrefix(runtime.Version(), "go")
	command.BuildPlatform = runtime.GOOS + "/" + runtime.GOARCH

	// Create AppManager
	configDir, err := resolveConfigDir()
	if err != nil {
		log.Fatalf("Failed to resolve config directory: %v", err)
	}
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

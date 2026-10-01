// Tingly Box GUI - Kong-based implementation (mirrors cli/tingly-box/main.go)

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/alecthomas/kong"

	commandgui "github.com/tingly-dev/tingly-box/gui/wails3/command"
	"github.com/tingly-dev/tingly-box/internal/app"
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

// CLI is the Kong root structure for the GUI binary. There are no
// subcommands: the GUI has a single unified mode (server + tray with hub
// panel + main window), so the server flags live directly on the root and
// bare `tingly-box-gui` (or `tingly-box-gui --config-dir X --port Y`)
// launches straight into it. The server flags are the CLI's own
// (command.ServerFlagsKong), so the two entry points cannot drift apart.
type CLI struct {
	ConfigDir string `kong:"flag,name='config-dir',help='Configuration directory'"`

	command.ServerFlagsKong

	// Accepted and ignored, so a launch line written for the CLI's `start`
	// still opens the GUI: the window is the UI (no browser) and a desktop
	// app is never daemonized.
	EnableOpenBrowser bool   `kong:"flag,name='browser',hidden,help='Unused in GUI mode'"`
	Daemon            bool   `kong:"flag,name='daemon',hidden,help='Unused in GUI mode'"`
	LogFile           string `kong:"flag,name='log-file',hidden,help='Unused in GUI mode'"`
}

// Run launches the unified GUI mode. Kong invokes this on the root node
// since the grammar has no subcommands.
func (c *CLI) Run(appManager *app.AppManager, launcher commandgui.AppLauncher) error {
	return launcher.Start(appManager, c.ServerFlagsKong)
}

// main function serves as the application's entry point. It uses Kong to
// parse CLI arguments and launches the unified GUI.
func main() {
	command.BuildVersion = version
	command.BuildGitCommit = gitCommit
	command.BuildBuildTime = buildTime
	command.BuildGoVersion = strings.TrimPrefix(runtime.Version(), "go")
	command.BuildPlatform = runtime.GOOS + "/" + runtime.GOARCH

	launcher := NewAppLauncher()

	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("tingly-box-gui"),
		kong.Description("Tingly Box GUI mode provides a desktop application interface for managing the AI model proxy server."),
		// CLI.Run takes AppLauncher, an interface — Kong's ctx.Run(v)
		// binds by v's concrete type, so a plain positional bind here
		// would register *appLauncher and never match the interface
		// parameter. BindTo pins it to the interface.
		kong.BindTo(launcher, (*commandgui.AppLauncher)(nil)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize parser: %v\n", err)
		os.Exit(1)
	}

	ctx, parseErr := parser.Parse(os.Args[1:])
	if parseErr != nil {
		parser.Errorf("%v", parseErr)
		os.Exit(1)
	}

	// Lazy, same as cli/tingly-box/main.go: AppConfig — and with it
	// tingly.db and every store's migration — is built only once the
	// launcher holds the single-instance lock (see run.go), so a second
	// launch that just focuses the running instance never becomes a second
	// SQLite writer next to it (#1912).
	configDir := cli.ConfigDir
	if configDir != "" {
		configDir, err = fs.ExpandConfigDir(configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to initialize config: %v\n", err)
			os.Exit(1)
		}
	}
	appManager := app.NewLazyAppManager(configDir, func(err error) {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize config: %v\n", err)
		os.Exit(1)
	})
	appManager.SetVersion(version)

	if err := ctx.Run(appManager); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

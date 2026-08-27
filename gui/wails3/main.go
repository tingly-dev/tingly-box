// Tingly Box GUI - Kong-based implementation (mirrors cli/tingly-box/main.go)

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/alecthomas/kong"

	commandgui "github.com/tingly-dev/tingly-box/gui/wails3/command"
	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/pkg/fs"
)

// Build metadata injected via -ldflags "-X main.version=..." (see
// VERSION_LDFLAGS in Taskfile.wails.yml). Same -X targets as cli/tingly-box.
var (
	version   = "dev"
	gitCommit = "unknown"
	buildTime = "unknown"
)

// CLI is the Kong root structure for the GUI binary. --config-dir is parsed
// here, before any subcommand's Run — AppConfig is built from it up front,
// same as cli/tingly-box/main.go's CLI struct. tray is tagged the default
// subcommand ("withargs": selected even with flags present, e.g.
// `tingly-box-gui --config-dir X --port Y`) so bare `tingly-box-gui` keeps
// launching straight into tray mode like it did pre-Kong.
type CLI struct {
	ConfigDir string `kong:"flag,name='config-dir',help='Configuration directory'"`

	GUI  commandgui.GUICmdKong  `kong:"cmd,help='Start Tingly Box in full GUI mode (window + systray)'"`
	Slim commandgui.SlimCmdKong `kong:"cmd,help='Start Tingly Box in slim GUI mode (systray only)'"`
	Tray commandgui.TrayCmdKong `kong:"cmd,default='withargs',help='Start Tingly Box in tray GUI mode (systray only)'"`
}

// main function serves as the application's entry point. It uses Kong to
// parse CLI arguments and launches the appropriate GUI mode.
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
		// GUICmdKong/SlimCmdKong/TrayCmdKong's Run() takes AppLauncher, an
		// interface — Kong's ctx.Run(v) binds by v's concrete type, so a
		// plain positional bind here would register *appLauncher and never
		// match the interface parameter. BindTo pins it to the interface.
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

	var appConfig *config.AppConfig
	if cli.ConfigDir != "" {
		expandedDir, expandErr := fs.ExpandConfigDir(cli.ConfigDir)
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

	appManager := command.NewAppManagerWithConfig(appConfig)

	if err := ctx.Run(appManager); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

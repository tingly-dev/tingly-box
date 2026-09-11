// Tingly Box CLI - Kong-based implementation

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	_ "time/tzdata" // Embed timezone data for static builds

	"github.com/alecthomas/kong"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/command"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/pkg/fs"
)

var (
	version   = "dev"
	gitCommit = "unknown"
	buildTime = "unknown"
	goVersion = "unknown"
	platform  = "unknown"
)

// CLI is the main Kong CLI structure
type CLI struct {
	ConfigDir string `kong:"flag,name='config-dir',help='Configuration directory'"`
	Verbose   bool   `kong:"flag,name='verbose',short='v',help='Verbose output'"`
	PProf     bool   `kong:"flag,name='pprof',help='Run with pprof in :6060'"`
	Source    string `kong:"flag,name='source',help='How tingly-box was launched (binary, npx, npm, npx-bundle, npm-bundle); used to match the shortcut it creates/refreshes to the install method'"`

	// Server commands
	Start   command.StartCmdKong   `kong:"cmd,help='Start the server'"`
	Stop    command.StopCmdKong    `kong:"cmd,help='Stop the server'"`
	Status  command.StatusCmdKong  `kong:"cmd,help='Show status'"`
	Restart command.RestartCmdKong `kong:"cmd,help='Restart the server'"`
	Open    command.OpenCmdKong    `kong:"cmd,help='Open web UI'"`

	// Create a double-click shortcut (desktop / start menu) to launch Tingly Box
	Shortcut command.ShortcutCmdKong `kong:"cmd,help='Create a desktop/start-menu shortcut to launch Tingly Box'"`

	// Provider / rule management: pure CI surface, one operation per
	// invocation, never interactive — see provider_cmd.go / rule_cmd.go.
	Provider command.ProviderCmdKong `kong:"cmd,help='Manage providers (list/add/get/update/delete)'"`
	Rule     command.RuleCmdKong     `kong:"cmd,help='Manage routing rules (list/add/update/delete/export/import)'"`

	// Agent commands
	Agent command.AgentCmdKong `kong:"cmd,help='Agent configuration'"`

	// Headless one-shot setup for CI / fully-managed environments
	CI command.CICmdKong `kong:"cmd,name='ci',help='Headless one-shot agent setup (provider + model + agent)'"`

	// OAuth
	OAuth command.OAuthCmdKong `kong:"cmd,name='oauth',help='OAuth authentication'"`

	// Tingly-box token management (auth + model, view / refresh)
	Token command.TokenCmdKong `kong:"cmd,name='token',help='View or refresh tingly-box auth/model tokens'"`

	// Claude Code
	CC      command.CCmdKong       `kong:"cmd,help='Launch Claude Code'"`
	Profile command.ProfileCmdKong `kong:"cmd,help='Manage and use Claude Code profiles'"`

	// Other commands
	Swagger    command.SwaggerCmdKong    `kong:"cmd,hidden,help='Generate OpenAPI schema'"`
	Quota      command.QuotaCmdKong      `kong:"cmd,help='Quota information'"`
	Remote     command.RemoteCmdKong     `kong:"cmd,help='Remote control'"`
	TUI        command.TUICmdKong        `kong:"cmd,name='tui',help='Interactive console (QuickStart / Provider / Rule / Agent)'"`
	Quickstart command.QuickstartCmdKong `kong:"cmd,name='quickstart',hidden,help='Jump straight to the QuickStart wizard'"`

	// System log streaming/inspection
	Log command.LogCmdKong `kong:"cmd,name='log',help='View system logs (real-time follow by default, use --once for one-shot)'"`

	// MCP builtin (hidden)
	MCPBuiltin command.MCPBuiltinCmdKong `kong:"cmd,name='mcp-builtin',hidden,help='Start the builtin MCP server (internal use)'"`

	// Version
	Version command.VersionCmdKong `kong:"cmd,help='Show version'"`
}

func main() {
	command.BuildVersion = version
	command.BuildGitCommit = gitCommit
	command.BuildBuildTime = buildTime
	command.BuildGoVersion = goVersion
	command.BuildPlatform = platform

	var cli CLI

	// Parse CLI. NoExpandSubcommands keeps `--help` showing only the next
	// level of subcommands rather than walking every leaf — so
	// `tingly-box --help` lists `provider` / `rule` (not `provider add` etc.),
	// and `tingly-box provider --help` lists list/add/get/update/delete
	// rather than expanding each one's own flags.
	parser, err := kong.New(&cli,
		kong.Vars{
			"version":   version,
			"gitCommit": gitCommit,
			"buildTime": buildTime,
			"goVersion": goVersion,
			"platform":  platform,
		},
		kong.ConfigureHelp(kong.HelpOptions{NoExpandSubcommands: true}),
		kong.Help(func(options kong.HelpOptions, ctx *kong.Context) error {
			// Print default Kong help
			if err := kong.DefaultHelpPrinter(options, ctx); err != nil {
				return err
			}
			return nil
		}),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize parser: %v\n", err)
		os.Exit(1)
	}

	// A bare `tingly-box` shows help instead of a parse error: the CLI is a
	// toolbox, and starting/restarting the server stays an explicit action
	// (`tingly-box start`). Only the npx shim, where invocation itself
	// expresses "run it now", injects a default command.
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"--help"}
	}

	ctx, parseErr := parser.Parse(args)
	if parseErr != nil {
		parser.Errorf("%v", parseErr)
		os.Exit(1)
	}

	if cli.PProf {
		go func() {
			_ = http.ListenAndServe("127.0.0.1:6060", nil)
		}()
	}

	// Setup verbose logging
	if cli.Verbose {
		logrus.SetLevel(logrus.TraceLevel)
	}

	var appConfig *config.AppConfig

	configDir := cli.ConfigDir
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

	appManager := command.NewAppManagerWithConfig(appConfig)

	// Run the selected command. command.LaunchSource carries how this process
	// was invoked (binary/npx/npx-bundle, from --source) so `shortcut`, `start`,
	// and `restart` can generate/refresh a launcher matching the install method.
	if err := ctx.Run(appManager, command.LaunchSource(cli.Source)); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

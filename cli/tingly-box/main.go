// Tingly Box CLI - Kong-based implementation

package main

import (
	"fmt"
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

	// Server commands
	Start   command.StartCmdKong   `kong:"cmd,help='Start the server'"`
	Stop    command.StopCmdKong    `kong:"cmd,help='Stop the server'"`
	Status  command.StatusCmdKong  `kong:"cmd,help='Show status'"`
	Restart command.RestartCmdKong `kong:"cmd,help='Restart the server'"`
	Open    command.OpenCmdKong    `kong:"cmd,help='Open web UI'"`

	// Configuration management (unified). Subcommands: provider, rule.
	Config command.ConfigCmdKong `kong:"cmd,help='Manage configuration (providers, rules)'"`

	// Agent commands
	Agent command.AgentCmdKong `kong:"cmd,help='Agent configuration'"`

	// OAuth
	OAuth command.OAuthCmdKong `kong:"cmd,name='oauth',help='OAuth authentication'"`

	// Tingly-box token management (auth + model, view / refresh)
	Token command.TokenCmdKong `kong:"cmd,name='token',help='View or refresh tingly-box auth/model tokens'"`

	// Claude Code
	CC command.CCmdKong `kong:"cmd,help='Launch Claude Code',passthrough"`

	// Other commands
	Swagger    command.SwaggerCmdKong    `kong:"cmd,hidden,help='Generate OpenAPI schema'"`
	Quota      command.QuotaCmdKong      `kong:"cmd,help='Quota information'"`
	Remote     command.RemoteCmdKong     `kong:"cmd,help='Remote control'"`
	TUI        command.TUICmdKong        `kong:"cmd,name='tui',help='Interactive setup wizard'"`
	Quickstart command.QuickstartCmdKong `kong:"cmd,name='quickstart',hidden,help='Alias for tui'"`

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
	// `tingly-box --help` lists `config` (not `config provider add` etc.),
	// and `tingly-box config --help` lists `provider` / `rule` rather than
	// every finer-grained operation.
	ctx := kong.Parse(&cli,
		kong.Vars{
			"version":   version,
			"gitCommit": gitCommit,
			"buildTime": buildTime,
			"goVersion": goVersion,
			"platform":  platform,
		},
		kong.ConfigureHelp(kong.HelpOptions{NoExpandSubcommands: true}),
	)

	// Setup verbose logging
	if cli.Verbose {
		logrus.SetLevel(logrus.TraceLevel)
	}

	// Initialize config
	var appConfig *config.AppConfig
	var err error

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

	// Run the selected command
	if err := ctx.Run(appManager); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

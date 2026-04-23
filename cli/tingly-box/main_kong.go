//go:build kong

// Kong version of main - for testing migration
// Build with: go build -tags kong ./cli/tingly-box

package main

import (
	"fmt"
	"os"

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

	// Provider commands
	Provider command.ProviderCmdKong `kong:"cmd,help='Manage providers'"`

	// Agent commands
	Agent command.AgentCmdKong `kong:"cmd,help='Agent configuration'"`

	// OAuth
	OAuth command.OAuthCmdKong `kong:"cmd,help='OAuth authentication'"`

	// Import/Export
	Export command.ExportCmdKong `kong:"cmd,help='Export configuration'"`
	Import command.ImportCmdKong `kong:"cmd,help='Import configuration'"`

	// Claude Code
	CC command.CCmdKong `kong:"cmd,help='Launch Claude Code',passthrough"`

	// Other commands
	Swagger    command.SwaggerCmdKong    `kong:"cmd,help='Generate OpenAPI schema'"`
	Quota      command.QuotaCmdKong      `kong:"cmd,help='Quota information'"`
	Remote     command.RemoteCmdKong     `kong:"cmd,help='Remote control'"`
	Quickstart command.QuickstartCmdKong `kong:"cmd,help='Guided setup'"`
	MCP        command.MCPCmdKong        `kong:"cmd,help='MCP builtin server'"`

	// Version
	Version command.VersionCmdKong `kong:"cmd,help='Show version'"`
}

func main() {
	var cli CLI

	// Parse CLI
	ctx := kong.Parse(&cli, kong.Vars{
		"version":   version,
		"gitCommit": gitCommit,
		"buildTime": buildTime,
		"goVersion": goVersion,
		"platform":  platform,
	})

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

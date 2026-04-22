//go:build kong

package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

var (
	version   = "dev"
	gitCommit = "unknown"
	buildTime = "unknown"
)

// CLI is the main Kong CLI structure for harness
type CLI struct {
	Version VersionKong `kong:"cmd,help='Show version'"`

	// Subcommands
	Matrix     MatrixKong     `kong:"cmd,help='Run protocol validation matrix tests'"`
	Agent      AgentKong      `kong:"cmd,help='Run agent e2e tests'"`
	Provider   ProviderKong   `kong:"cmd,help='Run real provider API tests'"`
	InitConfig InitConfigKong `kong:"cmd,help='Create config file template'"`
}

func main() {
	var cli CLI

	ctx := kong.Parse(&cli, kong.Vars{
		"version":   version,
		"gitCommit": gitCommit,
		"buildTime": buildTime,
	})

	// Run the selected command
	if err := ctx.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// Package main provides the CLI harness for protocol validation testing.
//
// The harness provides several testing modes:
//   - matrix: Virtual provider e2e tests (protocol transformations)
//   - replay: Fixture replay through the in-process gateway
//   - agent: Real agent CLI runs against mock or real upstreams
//   - lb: Load-balancing scenario simulator (tier/failover/breaker/affinity)
//   - provider: Real provider API e2e tests (live API compatibility) - Phase 3
package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

// Build information variables (set via ldflags).
var (
	version   = "dev"
	gitCommit = "unknown"
	buildTime = "unknown"
)

// CLI is the top-level Kong CLI structure for the harness.
type CLI struct {
	Version    VersionCmd    `kong:"cmd,help='Show version'"`
	Matrix     MatrixCmd     `kong:"cmd,help='Run protocol validation matrix tests'"`
	Agent      AgentCmd      `kong:"cmd,help='Run agent e2e tests (use --mock or --config <file>)'"`
	Replay     ReplayCmd     `kong:"cmd,help='Replay a captured agent request fixture through the gateway'"`
	Lb         LbCmd         `kong:"cmd,help='Simulate load-balancing (tier/failover/breaker/affinity) over a request sequence'"`
	Provider   ProviderCmd   `kong:"cmd,help='Real provider API tests (Phase 3 - not yet implemented)'"`
	InitConfig InitConfigCmd `kong:"cmd,name='init-config',help='Create a providers config file template for agent --config'"`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("harness"),
		kong.Description("Tingly-Box Protocol Validation Harness"),
		kong.UsageOnError(),
		kong.Vars{
			"version":   version,
			"gitCommit": gitCommit,
			"buildTime": buildTime,
		},
	)
	if err := ctx.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// VersionCmd prints build version information.
type VersionCmd struct{}

func (*VersionCmd) Run() error {
	fmt.Printf("Tingly-Box Protocol Validation Harness\n")
	fmt.Printf("Version:   %s\n", version)
	fmt.Printf("Commit:    %s\n", gitCommit)
	fmt.Printf("Built:     %s\n", buildTime)
	return nil
}

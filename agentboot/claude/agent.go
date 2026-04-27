package claude

import (
	"context"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Agent implements the agentboot.Agent interface for Claude Code.
// Internally it uses Driver (process setup) + Transport (protocol) + agentboot.Runner (execution).
type Agent struct {
	runner    *agentboot.Runner
	driver    *Driver
	transport *Transport
}

// NewAgent creates a new Claude agent.
func NewAgent(config agentboot.Config) *Agent {
	claudeConfig := Config{
		EnableStreamJSON:        config.EnableStreamJSON,
		StreamBufferSize:        config.StreamBufferSize,
		DefaultExecutionTimeout: config.DefaultExecutionTimeout,
	}
	return NewAgentWithConfig(claudeConfig)
}

// NewAgentWithConfig creates a Claude agent with full Claude-specific config.
func NewAgentWithConfig(config Config) *Agent {
	driver := NewDriver(config)
	transport := NewTransport()
	runner := agentboot.NewRunner(driver, transport)
	return &Agent{
		runner:    runner,
		driver:    driver,
		transport: transport,
	}
}

// Execute runs the Claude agent.
func (a *Agent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (*agentboot.Result, error) {
	// Inject per-execution routing context into the transport before running.
	a.transport.SetExecutionContext(opts.SessionID, opts.ChatID, opts.Platform, opts.BotUUID)
	return a.runner.Execute(ctx, prompt, opts)
}

// IsAvailable checks if Claude Code is available.
func (a *Agent) IsAvailable() bool { return a.driver.IsAvailable() }

// Type returns the agent type.
func (a *Agent) Type() agentboot.AgentType { return agentboot.AgentTypeClaude }

// SetDefaultFormat sets the default output format.
func (a *Agent) SetDefaultFormat(format agentboot.OutputFormat) {
	a.runner.SetDefaultFormat(format)
}

// GetDefaultFormat returns the current default format.
func (a *Agent) GetDefaultFormat() agentboot.OutputFormat {
	return a.runner.GetDefaultFormat()
}

// SetSkipPermissions enables or disables skip permissions mode.
func (a *Agent) SetSkipPermissions(enabled bool) {
	a.driver.SetSkipPermissions(enabled)
}

// SetCLIPath sets an explicit CLI path.
func (a *Agent) SetCLIPath(path string) {
	a.driver.SetCLIPath(path)
}

package claude

import (
	"context"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

// Agent implements the agentboot.Agent interface for Claude Code.
// Internally it uses Driver (process setup) + Transport (protocol) + agentboot.Runner (execution).
type Agent struct {
	runner *agentboot.Runner
	driver *Driver
}

// NewAgent creates a new Claude agent.
func NewAgent(config agentboot.Config) *Agent {
	claudeConfig := Config{
		EnableStreamJSON:        config.EnableStreamJSON,
		StreamBufferSize:        config.StreamBufferSize,
		DefaultExecutionTimeout: config.DefaultExecutionTimeout,
	}
	agent := NewAgentWithConfig(claudeConfig)
	if config.DefaultFormat != "" {
		agent.SetDefaultFormat(config.DefaultFormat)
	}
	return agent
}

// NewAgentWithConfig creates a Claude agent with full Claude-specific config.
func NewAgentWithConfig(config Config) *Agent {
	driver := NewDriver(config)
	runner := agentboot.NewRunnerWithConfig(
		driver,
		func() agentboot.AgentTransport { return NewTransport() },
		runnerConfig(config),
	)
	return &Agent{
		runner: runner,
		driver: driver,
	}
}

// NewAgentWithFactory creates a Claude agent that uses the supplied
// [process.Factory] instead of the OS exec factory. Tests inject a factory
// from agentboot/claude/fixture to substitute the claude binary with a
// scripted in-memory process while keeping the real driver and transport
// on the code path.
//
// The driver's binary-availability check is forced to true since tests
// don't want to spawn the real claude binary.
func NewAgentWithFactory(config Config, factory process.Factory) *Agent {
	driver := NewDriver(config)
	driver.SetForceAvailable(true)
	driver.SetCLIPath("claude-fixture-binary") // sentinel; resolveBinary returns it as-is
	runner := agentboot.NewRunnerWithFactoryAndConfig(
		driver,
		func() agentboot.AgentTransport { return NewTransport() },
		factory,
		runnerConfig(config),
	)
	return &Agent{
		runner: runner,
		driver: driver,
	}
}

func runnerConfig(config Config) agentboot.RunnerConfig {
	return agentboot.RunnerConfig{
		DefaultFormat:           agentboot.OutputFormatStreamJSON,
		EventBufferSize:         config.StreamBufferSize,
		DefaultExecutionTimeout: config.DefaultExecutionTimeout,
	}
}

// Execute runs the Claude agent and returns an [agentboot.ExecutionHandle].
// The runner injects the per-execution routing context into the transport
// internally; callers consume handle.Events() for streaming output and
// respond to control events via handle.Respond.
func (a *Agent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
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

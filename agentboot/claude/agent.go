package claude

import (
	"context"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/permission"
)

// Agent implements the agentboot.Agent interface for Claude Code
type Agent struct {
	launcher *Launcher
}

// NewAgent creates a new Claude agent
func NewAgent(config agentboot.Config) *Agent {
	launcherConfig := Config{
		EnableStreamJSON: config.EnableStreamJSON,
		StreamBufferSize: config.StreamBufferSize,
	}

	return &Agent{
		launcher: NewLauncher(launcherConfig),
	}
}

// NewAgentWithPermissionHandler creates a new Claude agent with permission handler
func NewAgentWithPermissionHandler(config agentboot.Config, handler permission.Handler) *Agent {
	agent := NewAgent(config)
	agent.launcher.SetPermissionHandler(handler)
	return agent
}

// Execute runs the Claude agent
func (a *Agent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (*agentboot.Result, error) {
	return a.launcher.Execute(ctx, prompt, opts)
}

// IsAvailable checks if Claude Code is available
func (a *Agent) IsAvailable() bool {
	return a.launcher.IsAvailable()
}

// Type returns the agent type
func (a *Agent) Type() agentboot.AgentType {
	return agentboot.AgentTypeClaude
}

// SetDefaultFormat sets the default output format
func (a *Agent) SetDefaultFormat(format agentboot.OutputFormat) {
	a.launcher.SetDefaultFormat(format)
}

// GetDefaultFormat returns the current default format
func (a *Agent) GetDefaultFormat() agentboot.OutputFormat {
	return a.launcher.GetDefaultFormat()
}

// SetPermissionHandler sets the permission handler
func (a *Agent) SetPermissionHandler(handler permission.Handler) {
	a.launcher.SetPermissionHandler(handler)
}

// GetPermissionHandler returns the current permission handler
func (a *Agent) GetPermissionHandler() permission.Handler {
	return a.launcher.GetPermissionHandler()
}

// SetSkipPermissions enables or disables skip permissions mode
func (a *Agent) SetSkipPermissions(enabled bool) {
	a.launcher.SetSkipPermissions(enabled)
}

// SetCLIPath sets an explicit CLI path
func (a *Agent) SetCLIPath(path string) {
	a.launcher.SetCLIPath(path)
}

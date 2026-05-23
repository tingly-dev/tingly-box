package claude

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Launcher is the legacy entry point for Claude Code CLI execution.
// Internally it now delegates to Driver + Transport + agentboot.Runner.
//
// Prefer using NewAgentWithConfig for new code. Launcher is retained for
// backward compatibility with existing callers (examples, integration tests).
type Launcher struct {
	mu      sync.RWMutex
	driver  *Driver
	runner  *agentboot.Runner
	transport *Transport
}

// NewLauncher creates a new Claude launcher.
func NewLauncher(config Config) *Launcher {
	driver := NewDriver(config)
	transport := NewTransport()
	runner := agentboot.NewRunner(driver, transport)
	return &Launcher{
		driver:    driver,
		transport: transport,
		runner:    runner,
	}
}

// GetControlManager returns the ControlManager (for Interrupt / SendPermissionRequest).
func (l *Launcher) GetControlManager() *ControlManager {
	return NewControlManager()
}

// GetDiscovery returns the CLI discovery instance.
func (l *Launcher) GetDiscovery() *CLIDiscovery {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.driver.discovery
}

// Execute runs Claude Code and returns an [agentboot.ExecutionHandle].
func (l *Launcher) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	return l.runner.Execute(ctx, prompt, opts)
}

// ExecuteWithTimeout runs Claude Code with a specific timeout and returns an
// [agentboot.ExecutionHandle].
func (l *Launcher) ExecuteWithTimeout(
	ctx context.Context,
	prompt string,
	timeout time.Duration,
	opts agentboot.ExecutionOptions,
) (agentboot.ExecutionHandle, error) {
	opts.Timeout = timeout
	return l.runner.Execute(ctx, prompt, opts)
}

// IsAvailable checks if Claude Code CLI is available.
func (l *Launcher) IsAvailable() bool {
	return l.driver.IsAvailable()
}

// Type returns the agent type.
func (l *Launcher) Type() agentboot.AgentType {
	return agentboot.AgentTypeClaude
}

// SetDefaultFormat sets the default output format.
func (l *Launcher) SetDefaultFormat(format agentboot.OutputFormat) {
	l.runner.SetDefaultFormat(format)
}

// GetDefaultFormat returns the current default format.
func (l *Launcher) GetDefaultFormat() agentboot.OutputFormat {
	return l.runner.GetDefaultFormat()
}

// SetSkipPermissions enables or disables the dangerously-skip-permissions flag.
func (l *Launcher) SetSkipPermissions(enabled bool) {
	l.driver.SetSkipPermissions(enabled)
}

// SetCLIPath sets an explicit path to the Claude CLI binary.
func (l *Launcher) SetCLIPath(path string) {
	l.driver.SetCLIPath(path)
}

// Interrupt sends an interrupt/cancel request to a running Claude process.
func (l *Launcher) Interrupt(ctx context.Context, stdin io.WriteCloser, reason string) error {
	controlMgr := NewControlManager()
	builder := NewCancelRequestBuilder().WithCancel("execution").WithReason(reason)
	return controlMgr.SendRequestAsync(builder.Build(), stdin)
}


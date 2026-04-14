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

// Execute runs Claude Code with the given prompt.
// If opts.Handler is set the result is delivered via the handler and nil is returned.
func (l *Launcher) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (*agentboot.Result, error) {
	l.transport.SetExecutionContext(opts.SessionID, opts.ChatID, opts.Platform, opts.BotUUID)
	return l.runner.Execute(ctx, prompt, opts)
}

// ExecuteWithTimeout runs Claude Code with a specific timeout.
func (l *Launcher) ExecuteWithTimeout(
	ctx context.Context,
	prompt string,
	timeout time.Duration,
	opts agentboot.ExecutionOptions,
) (*agentboot.Result, error) {
	opts.Timeout = timeout
	l.transport.SetExecutionContext(opts.SessionID, opts.ChatID, opts.Platform, opts.BotUUID)
	return l.runner.Execute(ctx, prompt, opts)
}

// ExecuteWithHandler runs Claude Code and streams events to handler.
// Returns nil result; the handler receives all messages.
func (l *Launcher) ExecuteWithHandler(
	ctx context.Context,
	prompt string,
	timeout time.Duration,
	opts agentboot.ExecutionOptions,
	handler agentboot.MessageHandler,
) error {
	opts.Timeout = timeout
	opts.Handler = handler
	l.transport.SetExecutionContext(opts.SessionID, opts.ChatID, opts.Platform, opts.BotUUID)
	_, err := l.runner.Execute(ctx, prompt, opts)
	return err
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

// SendPermissionRequest sends a permission request and waits for the response.
func (l *Launcher) SendPermissionRequest(ctx context.Context, req agentboot.PermissionRequest, stdin io.WriteCloser) (agentboot.PermissionResult, error) {
	controlMgr := NewControlManager()
	builder := NewPermissionRequestBuilder().
		WithRequestID(req.RequestID).
		WithTool(req.ToolName, req.Input)

	resp, err := controlMgr.SendRequest(ctx, builder.Build(), stdin)
	if err != nil {
		return agentboot.PermissionResult{Approved: false}, err
	}

	result := agentboot.PermissionResult{Approved: true}
	if resp.Response != nil {
		if subtype, _ := resp.Response["subtype"].(string); subtype == ResultSubtypeError {
			result.Approved = false
			result.Reason, _ = resp.Response["error"].(string)
		}
	}
	return result, nil
}

package agentboot

import (
	"context"
	"os/exec"
)

// LaunchSpec describes how to start an agent process.
// It is pure data: no business logic.
type LaunchSpec struct {
	// Command is the binary and arguments (e.g. ["claude", "--output-format", "stream-json", ...])
	Command []string

	// Env is the process environment. nil inherits the current process environment.
	Env []string

	// WorkDir is the working directory for the agent process. Empty means current directory.
	WorkDir string

	// InitialInput is an optional channel that feeds initial messages into the agent's stdin.
	// Close the channel to signal EOF. May be nil.
	InitialInput <-chan any
}

// BuildCmd converts a LaunchSpec into an *exec.Cmd with the given context.
// WorkDir and Env are applied; the caller is responsible for piping stdin/stdout.
func (s *LaunchSpec) BuildCmd(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(ctx, s.Command[0], s.Command[1:]...)
	if s.WorkDir != "" {
		cmd.Dir = s.WorkDir
	}
	if s.Env != nil {
		cmd.Env = s.Env
	}
	return cmd
}

// AgentDriver knows how to prepare the launch of an agent process.
// Each agent type (Claude, Codex, opencode, …) provides its own Driver.
//
// A Driver is responsible for:
//   - Binary discovery
//   - CLI argument construction
//   - Environment setup
//   - Initial prompt injection (stdin bootstrap)
//
// A Driver does NOT manage the running process or the communication protocol.
type AgentDriver interface {
	// Prepare returns a LaunchSpec describing how to start the agent.
	// The spec is consumed by a Runner; the Driver itself does not start anything.
	Prepare(ctx context.Context, prompt string, opts ExecutionOptions) (*LaunchSpec, error)

	// IsAvailable reports whether the agent binary is present and usable.
	IsAvailable() bool

	// Type returns the agent type this driver handles.
	Type() AgentType
}

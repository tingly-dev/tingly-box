// Package process provides the seam between agentboot and the OS process that
// implements an agent (e.g. the `claude` CLI binary).
//
// Production code uses [OSExecFactory], which wraps os/exec. Tests use
// [FakeFactory] to substitute the binary with a scripted in-memory process,
// keeping the rest of the agent stack (driver, decoder, runner) on the real
// code path.
package process

import (
	"context"
	"io"
	"os/exec"
)

// LaunchSpec describes how to start an agent process.
//
// It is pure launch data shared directly by agent drivers and process
// factories, so the Runner does not need an intermediate conversion.
type LaunchSpec struct {
	// Command is the binary followed by arguments.
	Command []string

	// Env is the process environment. nil inherits the current environment.
	Env []string

	// WorkDir is the child working directory. Empty means the current directory.
	WorkDir string

	// InitialInput optionally feeds bootstrap messages to stdin after start.
	// It is consumed by the Runner, not the process Factory.
	InitialInput <-chan any
}

// BuildCmd converts the specification into an exec.Cmd.
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

// Handle is a running process with attached stdin/stdout pipes.
//
// Lifecycle invariants:
//   - Stdin and Stdout are valid until Wait returns.
//   - Done is closed after the process has exited; Wait then returns
//     immediately with the exit error.
//   - Kill is idempotent and safe to call concurrently with Wait.
type Handle interface {
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	Wait() error
	Kill() error
	Done() <-chan struct{}
}

// Factory starts agent processes.
type Factory interface {
	Start(ctx context.Context, spec LaunchSpec) (Handle, error)
}

package agentboot

import (
	"context"

	"github.com/tingly-dev/tingly-box/agentboot/process"
)

// LaunchSpec is the process launch contract shared by drivers and factories.
// It aliases [process.LaunchSpec] for source compatibility.
type LaunchSpec = process.LaunchSpec

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

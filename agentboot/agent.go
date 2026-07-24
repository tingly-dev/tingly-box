package agentboot

import "context"

// AgentType defines the supported agent types.
type AgentType string

const (
	AgentTypeClaude AgentType = "claude"
	// AgentTypeCodex  AgentType = "codex"  // Future
	// AgentTypeGemini AgentType = "gemini" // Future
	// AgentTypeCursor AgentType = "cursor" // Future
)

// String returns the string representation of AgentType.
func (t AgentType) String() string {
	return string(t)
}

// Agent is the interface for all agent types.
//
// Execute returns an [ExecutionHandle]; the caller iterates handle.Events()
// to consume the totally-ordered event stream, calls handle.Respond(...) to
// answer Approval/Ask requests, and calls handle.Wait() to obtain the
// aggregated [Result]. See the ExecutionHandle docs for lifecycle details.
type Agent interface {
	Execute(ctx context.Context, prompt string, opts ExecutionOptions) (ExecutionHandle, error)
	IsAvailable() bool
	Type() AgentType
	SetDefaultFormat(format OutputFormat)
	GetDefaultFormat() OutputFormat
}

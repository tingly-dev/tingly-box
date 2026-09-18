package agentboot

import "context"

// AgentType defines the supported agent types.
type AgentType string

// AgentTypeClaude is the only production agent type. New backends define
// their own constant next to their AgentDriver/AgentTransport implementation
// and register it via AgentService.RegisterAgent.
const (
	AgentTypeClaude AgentType = "claude"
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
}

// PersistentAgent is an optional capability: an [Agent] that also supports
// long-lived, multi-turn sessions via Open, alongside the one-shot Execute
// every Agent provides. Not every Agent implementation supports this —
// [AgentService.Open] type-asserts against it and reports an error for
// agents that don't. claude.Agent is the only implementation as of
// .design/claude-code.md's P1/P2.
type PersistentAgent interface {
	Agent
	Open(ctx context.Context, prompt string, opts ExecutionOptions) (PersistentSession, error)
}

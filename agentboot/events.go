package agentboot

import "github.com/tingly-dev/tingly-box/agentboot/common"

// Event represents a generic agent event.
// Alias of common.Event — the two types are identical and interchangeable.
type Event = common.Event

// StreamEvent is the sum type of events flowing on [ExecutionHandle.Events].
// Callers type-switch to specific event types.
//
// The interface is sealed (its sentinel method is unexported) so that
// agentboot owns the closed set of event types that runners may emit.
type StreamEvent interface {
	isStreamEvent()
}

// MessageEvent wraps a streamable agent message after the per-agent
// accumulator has consumed the raw common.Event. The concrete type of Raw
// is agent-specific (e.g. *claude.AssistantMessage, *claude.ToolUseMessage);
// consumers type-switch.
//
// In addition to emitting MessageEvents, the runner appends the raw
// underlying common.Event values to [Result.Events] for callers that
// prefer the aggregated form returned from [ExecutionHandle.Wait].
type MessageEvent struct {
	Raw any
}

func (MessageEvent) isStreamEvent() {}

// ApprovalRequestEvent is emitted when the agent requests permission to use
// a tool. Callers must call [ExecutionHandle.Respond] with [ApprovalResponse]
// to unblock the agent.
type ApprovalRequestEvent struct {
	ID        string
	AgentType AgentType
	ToolName  string
	Input     map[string]any
	Reason    string

	SessionID string
	ChatID    string
	Platform  string
	BotUUID   string
}

func (ApprovalRequestEvent) isStreamEvent() {}

// AskRequestEvent is emitted for AskUserQuestion-style interactive prompts.
// Callers respond via [ExecutionHandle.Respond] with [AskResponse].
type AskRequestEvent struct {
	ID        string
	AgentType AgentType
	Type      string
	ToolName  string
	Input     map[string]any
	CallID    string
	Message   string
	Reason    string

	SessionID string
	ChatID    string
	Platform  string
	BotUUID   string
}

func (AskRequestEvent) isStreamEvent() {}

// ErrorEvent reports an error noticed while consuming the execution stream.
// It may be recoverable, or it may be a tail notification of the fatal error
// that [ExecutionHandle.Wait] returns. Wait is the authoritative outcome.
type ErrorEvent struct {
	Err error
}

func (ErrorEvent) isStreamEvent() {}

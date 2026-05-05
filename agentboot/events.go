package agentboot

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
// is agent-specific (e.g. *claude.AssistantMessage, *claude.ToolUseMessage,
// or agentboot.AgentMessage); consumers type-switch.
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

// ErrorEvent reports a non-fatal error noticed during execution. The
// runner continues processing after emitting an ErrorEvent. Fatal errors
// are surfaced via [ExecutionHandle.Wait]'s returned error instead.
type ErrorEvent struct {
	Err error
}

func (ErrorEvent) isStreamEvent() {}

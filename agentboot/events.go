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
// accumulator has consumed the raw event. The concrete type of Raw
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

// SessionState is the lifecycle state of a [PersistentSession].
type SessionState string

const (
	// SessionStateIdle: no turn is in flight; Send is ready to accept one.
	SessionStateIdle SessionState = "idle"
	// SessionStateRunning: a turn is in flight (Open's first prompt, or a
	// call to Send).
	SessionStateRunning SessionState = "running"
	// SessionStateClosing: Close has been called; the underlying process is
	// shutting down.
	SessionStateClosing SessionState = "closing"
	// SessionStateTerminated: the process has exited, whether from a
	// graceful Close or an unexpected crash. Events() has closed.
	SessionStateTerminated SessionState = "terminated"
)

// TurnCompleteEvent marks the end of one turn on a [PersistentSession].
// Unlike [ExecutionHandle], whose Events channel closes when its single turn
// ends, a PersistentSession's channel stays open across turns —
// TurnCompleteEvent is the per-turn boundary signal that ExecutionHandle
// gets for free from the channel closing. Result carries that turn's
// events/duration/error, scoped to just that turn (not the whole session).
type TurnCompleteEvent struct {
	Result *Result
}

func (TurnCompleteEvent) isStreamEvent() {}

// SessionStateEvent reports a [PersistentSession] lifecycle transition to
// Terminated — a graceful Close or an unexpected process exit/protocol
// error. Reason is human-readable context. It is not emitted for the
// routine Running<->Idle transitions between turns; TurnCompleteEvent
// already conveys those.
type SessionStateEvent struct {
	State  SessionState
	Reason string
}

func (SessionStateEvent) isStreamEvent() {}

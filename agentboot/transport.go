package agentboot

import (
	"errors"

	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// AgentTransportFactory creates the protocol state for one execution.
//
// A transport may contain mutable, execution-scoped state such as message
// accumulators and routing metadata. Runner therefore calls the factory once
// per Execute instead of sharing a transport across concurrent executions.
type AgentTransportFactory func() AgentTransport

// EventKind is the classification result returned by [AgentTransport.Classify].
type EventKind int

const (
	// EventKindIgnore: the event is fully consumed by the transport (e.g.
	// internal-only system pings); the runner does not emit anything.
	EventKindIgnore EventKind = iota

	// EventKindMessage: a streamable agent message. The runner calls
	// [AgentTransport.AccumulateMessage] and emits a [MessageEvent] for each
	// rich message returned.
	EventKindMessage

	// EventKindControl: an interactive control request (permission/ask).
	// The corresponding parsed [StreamEvent] is returned alongside this
	// kind by Classify; the runner emits it on the handle and waits for a
	// response via [ExecutionHandle.Respond].
	EventKindControl

	// EventKindTerminalSuccess: the agent emitted a successful terminal
	// event. The runner records success and stops processing further events.
	EventKindTerminalSuccess

	// EventKindTerminalError: the agent emitted a failed terminal event.
	// The runner records failure and stops processing further events.
	EventKindTerminalError
)

// AgentTransport is the per-agent protocol parser. It is pure: it consumes
// [protocol.Event] values and produces classifications and encoded responses,
// but performs no IO and owns no goroutines.
//
// Each agent type (Claude, Codex, …) provides its own AgentTransport.
type AgentTransport interface {
	// Classify reports the kind of the event. For control events it also
	// returns the parsed StreamEvent (ApprovalRequestEvent or
	// AskRequestEvent) ready to emit on the handle.
	//
	// Execution context metadata can be stamped onto the StreamEvent during
	// Classify.
	Classify(ev protocol.Event) (kind EventKind, parsed StreamEvent)

	// AccumulateMessage feeds the event to the per-agent message accumulator
	// and returns 0+ rich message values to emit as [MessageEvent.Raw]. The
	// concrete type of each value is agent-specific (e.g.
	// *claude.AssistantMessage). The runner does not introspect them.
	AccumulateMessage(ev protocol.Event) []any

	// EncodeControlResponse converts a [ControlResponse] into the wire value
	// sent to the agent process's stdin via [protocol.Encoder].
	//
	// originalInput is the Input field of the corresponding
	// ApprovalRequestEvent / AskRequestEvent; some agents (e.g. claude) use
	// it when constructing the "allow" reply if the response did not supply
	// an UpdatedInput.
	EncodeControlResponse(reqID string, resp ControlResponse, originalInput map[string]any) any

	// EncodeUserMessage converts a plain-text user turn into the wire value
	// sent to the agent process's stdin via [protocol.Encoder]. Used by
	// [PersistentSession.Send] for every turn after the first — the first
	// turn is still delivered via the driver's InitialInput channel, exactly
	// as for a one-shot Execute.
	EncodeUserMessage(prompt string) any

	// SetExecutionContext injects provider-neutral per-execution metadata.
	SetExecutionContext(context ExecutionContext)
}

// ControlRequest is a host-initiated request to a running agent process —
// the reverse direction of ApprovalRequestEvent/AskRequestEvent.
type ControlRequest interface{ isControlRequest() }

// InterruptRequest stops the in-flight turn. The process stays alive and
// takes the next turn normally; background work it started keeps running.
type InterruptRequest struct{}

// StopTaskRequest stops one background task (a backgrounded shell command
// or subagent) by the id the agent reported for it.
type StopTaskRequest struct{ TaskID string }

func (InterruptRequest) isControlRequest() {}
func (StopTaskRequest) isControlRequest()  {}

// ErrControlUnsupported is returned for a [ControlRequest] the session's
// agent transport can't encode.
var ErrControlUnsupported = errors.New("agentboot: agent does not support this control request")

// ControlRequestEncoder is implemented by transports whose agent accepts
// host-initiated control requests on stdin. Optional: sessions whose
// transport lacks it return ErrControlUnsupported.
type ControlRequestEncoder interface {
	// EncodeControlRequest returns the wire value for req, or nil if the
	// agent has no such request.
	EncodeControlRequest(reqID string, req ControlRequest) any
}

// TurnStartDetector is implemented by transports that can tell which event
// opens a turn. A persistent agent can start a turn of its own — Claude Code
// does when a background task finishes after the turn that started it — and
// the session only recognizes such a turn through this. Optional.
type TurnStartDetector interface {
	IsTurnStart(ev protocol.Event) bool
}

package agentboot

import "github.com/tingly-dev/tingly-box/agentboot/common"

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
// [common.Event] values and produces classifications and encoded responses,
// but performs no IO and owns no goroutines.
//
// Each agent type (Claude, Codex, …) provides its own AgentTransport.
type AgentTransport interface {
	// Classify reports the kind of the event. For control events it also
	// returns the parsed StreamEvent (ApprovalRequestEvent or
	// AskRequestEvent) ready to emit on the handle.
	//
	// The execution-context fields (sessionID, chatID, platform, botUUID)
	// previously set via SetExecutionContext are stamped onto the StreamEvent
	// during Classify.
	Classify(ev common.Event) (kind EventKind, parsed StreamEvent)

	// AccumulateMessage feeds the event to the per-agent message accumulator
	// and returns 0+ rich message values to emit as [MessageEvent.Raw]. The
	// concrete type of each value is agent-specific (e.g.
	// *claude.AssistantMessage). The runner does not introspect them.
	AccumulateMessage(ev common.Event) []any

	// EncodeControlResponse converts a [ControlResponse] into the wire value
	// sent to the agent process's stdin via [protocol.Encoder].
	//
	// originalInput is the Input field of the corresponding
	// ApprovalRequestEvent / AskRequestEvent; some agents (e.g. claude) use
	// it when constructing the "allow" reply if the response did not supply
	// an UpdatedInput.
	EncodeControlResponse(reqID string, resp ControlResponse, originalInput map[string]any) any

	// SetExecutionContext injects per-execution routing metadata that is
	// stamped onto Approval/Ask events during Classify.
	SetExecutionContext(sessionID, chatID, platform, botUUID string)
}

package agentboot

import "context"

// AgentEvent is an agent-agnostic parsed event produced by a transport's Decode call.
// It wraps the raw Event and tags which categories of handling it requires.
type AgentEvent struct {
	// Raw is the original parsed event.
	Raw Event

	// IsControl reports whether this event requires a bidirectional control response
	// (e.g. permission prompt, AskUserQuestion).
	IsControl bool

	// IsTerminal reports whether this event signals the end of the agent run
	// (e.g. Claude's "result" event).
	IsTerminal bool
}

// AgentTransport knows how to communicate with a running agent process.
// Each agent type (Claude, Codex, opencode, …) provides its own Transport.
//
// A Transport is responsible for:
//   - Decoding raw stdout values into AgentEvents
//   - Handling interactive control events (permission requests, user questions)
//     and writing the response payload back to the agent's stdin
//   - Accumulating raw events into typed messages to forward to MessageHandler.OnMessage
//
// A Transport does NOT manage the process lifecycle.
type AgentTransport interface {
	// Decode parses one JSON-decoded output value (as delivered by the mitm.Runner
	// in CodecJSON mode) into an AgentEvent.
	Decode(raw any) (AgentEvent, error)

	// HandleControl processes a control event and writes the response to the agent's
	// stdin using write. Called only when AgentEvent.IsControl == true.
	HandleControl(ctx context.Context, event AgentEvent, handler MessageHandler, write func(any) error) error

	// AccumulateAndForward accumulates the event and returns:
	//   - msgs: typed messages to forward to MessageHandler.OnMessage (nil for control/terminal events)
	//   - isTerminal: whether the event signals execution end
	//   - success: whether the terminal event indicates success
	AccumulateAndForward(ae AgentEvent) (msgs []interface{}, isTerminal bool, success bool)
}

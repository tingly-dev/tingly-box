package codex

import (
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/common"
)

const (
	eventThreadStarted = "thread.started"
	eventTurnCompleted = "turn.completed"
	eventTurnFailed    = "turn.failed"
	eventError         = "error"
)

// Message is the typed wrapper emitted for Codex JSONL progress events.
// SessionID allows higher layers to checkpoint a thread as soon as the
// thread.started event arrives.
type Message struct {
	Event     common.Event
	SessionID string
}

func (m Message) GetSessionID() string { return m.SessionID }

type Transport struct{}

func NewTransport() *Transport { return &Transport{} }

func (*Transport) SetExecutionContext(_, _, _, _ string) {}

func (*Transport) Classify(event common.Event) (agentboot.EventKind, agentboot.StreamEvent) {
	switch event.Type {
	case eventTurnCompleted:
		return agentboot.EventKindTerminalSuccess, nil
	case eventTurnFailed, eventError:
		return agentboot.EventKindTerminalError, nil
	default:
		return agentboot.EventKindMessage, nil
	}
}

func (*Transport) AccumulateMessage(event common.Event) []any {
	sessionID := ""
	if event.Type == eventThreadStarted {
		sessionID, _ = event.Data["thread_id"].(string)
	}
	return []any{Message{Event: event, SessionID: sessionID}}
}

// Codex exec is deliberately run with approval_policy=never, so it never has
// an interactive stdin control protocol to encode.
func (*Transport) EncodeControlResponse(string, agentboot.ControlResponse, map[string]any) any {
	return nil
}

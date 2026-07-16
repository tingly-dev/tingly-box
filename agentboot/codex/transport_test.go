package codex

import (
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/common"
)

func TestTransport_ClassifyAndSession(t *testing.T) {
	transport := NewTransport()
	started := common.NewEventFromMap(map[string]any{
		"type":      "thread.started",
		"thread_id": "thread-1",
	})
	kind, _ := transport.Classify(started)
	if kind != agentboot.EventKindMessage {
		t.Fatalf("kind = %v", kind)
	}
	messages := transport.AccumulateMessage(started)
	message, ok := messages[0].(Message)
	if !ok || message.GetSessionID() != "thread-1" {
		t.Fatalf("message = %#v", messages[0])
	}

	completed := common.NewEventFromMap(map[string]any{"type": "turn.completed"})
	if kind, _ := transport.Classify(completed); kind != agentboot.EventKindTerminalSuccess {
		t.Fatalf("completed kind = %v", kind)
	}
	failed := common.NewEventFromMap(map[string]any{"type": "turn.failed"})
	if kind, _ := transport.Classify(failed); kind != agentboot.EventKindTerminalError {
		t.Fatalf("failed kind = %v", kind)
	}
}

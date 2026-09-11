package claude

import (
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// The CLI's stream-json answers a tool_use with a `user` message whose
// content is a tool_result block. That must surface as a ToolResultMessage.
func TestAccumulator_UserMessageCarriesToolResults(t *testing.T) {
	a := NewMessageAccumulator()
	raw := `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_1","type":"tool_result","content":"probe-marker","is_error":false}]},"parent_tool_use_id":null,"session_id":"s1"}`
	msgs, _, _ := a.AddEvent(protocol.Event{Type: SDKUserMessage, Raw: raw})
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d: %+v", len(msgs), msgs)
	}
	tr, ok := msgs[0].(*ToolResultMessage)
	if !ok || tr.ToolUseID != "toolu_1" || tr.Output != "probe-marker" || tr.IsError {
		t.Fatalf("unexpected message %#v", msgs[0])
	}

	// Structured content and an error flag.
	raw = `{"type":"user","message":{"role":"user","content":[{"tool_use_id":"toolu_2","type":"tool_result","content":[{"type":"text","text":"boom"}],"is_error":true}]}}`
	msgs, _, _ = a.AddEvent(protocol.Event{Type: SDKUserMessage, Raw: raw})
	tr, ok = msgs[0].(*ToolResultMessage)
	if !ok || !tr.IsError || len(tr.Content) != 1 {
		t.Fatalf("unexpected message %#v", msgs[0])
	}

	// Plain text still comes through as a UserMessage, in both shapes.
	for _, raw := range []string{
		`{"type":"user","message":"hello"}`,
		`{"type":"user","message":{"role":"user","content":"hello"}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`,
	} {
		msgs, _, _ = a.AddEvent(protocol.Event{Type: SDKUserMessage, Raw: raw})
		um, ok := msgs[0].(*UserMessage)
		if len(msgs) != 1 || !ok || um.Message != "hello" {
			t.Fatalf("%s: unexpected %#v", raw, msgs)
		}
	}
}

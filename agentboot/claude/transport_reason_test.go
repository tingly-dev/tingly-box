package claude

import (
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// A permission prompt carries why Claude Code asked, so a thread that prompts
// in a mode that should not can say whether a rule, the auto-mode classifier
// or the mode itself is the reason.
func TestTransport_ApprovalCarriesWhyClaudeAsked(t *testing.T) {
	request := func(extra map[string]any) protocol.Event {
		req := map[string]any{"subtype": "can_use_tool", "tool_name": "Bash", "input": map[string]any{"command": "ls"}}
		for k, v := range extra {
			req[k] = v
		}
		return protocol.Event{Type: SDKControlPrefix + "request", Data: map[string]any{"request_id": "r1", "request": req}}
	}
	for _, tc := range []struct {
		name  string
		extra map[string]any
		want  string
	}{
		{"classifier gave up", map[string]any{"decision_reason": "3 consecutive actions were blocked. Please review the transcript."}, "3 consecutive actions were blocked. Please review the transcript."},
		{"an ask rule", map[string]any{"decision_reason_type": "rule"}, "matched an ask rule in the permission settings"},
		{"other kind", map[string]any{"decision_reason_type": "hook"}, "hook"},
		{"the mode itself", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ev := NewTransport().Classify(request(tc.extra))
			got, ok := ev.(agentboot.ApprovalRequestEvent)
			if !ok {
				t.Fatalf("event = %T", ev)
			}
			if got.Reason != tc.want {
				t.Fatalf("Reason = %q, want %q", got.Reason, tc.want)
			}
		})
	}
}

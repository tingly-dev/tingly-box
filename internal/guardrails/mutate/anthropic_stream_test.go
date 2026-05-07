package mutate

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestHandleAnthropicToolUseBuffer_RewritesBlockedMessageDeltaStopReason(t *testing.T) {
	streamState := &protocol.GuardrailsStreamState{
		RewroteBlockedToolUse: true,
	}

	decision := HandleAnthropicToolUseBuffer(
		nil,
		streamState,
		anthropicEventTypeMessageDelta,
		0,
		nil,
		map[string]interface{}{
			"type": anthropicEventTypeMessageDelta,
			"delta": map[string]interface{}{
				"stop_reason": "tool_use",
			},
		},
	)

	if decision.Kind != AnthropicToolUseDecisionPassthrough {
		t.Fatalf("decision.Kind = %q, want %q", decision.Kind, AnthropicToolUseDecisionPassthrough)
	}
	if streamState.RewroteBlockedToolUse {
		t.Fatalf("streamState.RewroteBlockedToolUse = true, want false")
	}
	if len(decision.Passthrough) != 1 {
		t.Fatalf("len(decision.Passthrough) = %d, want 1", len(decision.Passthrough))
	}
	delta, _ := decision.Passthrough[0].Payload["delta"].(map[string]interface{})
	if got, _ := delta["stop_reason"].(string); got != "end_turn" {
		t.Fatalf("delta.stop_reason = %q, want %q", got, "end_turn")
	}
}

package mutate

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestRewriteAnthropicToolUseEventDecisionDistinguishesAllowedPassthrough(t *testing.T) {
	state := &protocol.GuardrailsStreamState{
		PendingBlockMessages: make(map[string]string), PendingBlockedIndex: make(map[int]string),
		AnthropicToolEvents: make(map[int][]protocol.GuardrailsBufferedEvent), AnthropicToolIDs: make(map[int]string),
	}
	events := []string{
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool-1","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"safe\"}"}}`,
		`{"type":"content_block_stop","index":0}`,
	}
	want := []AnthropicToolUseDecisionKind{AnthropicToolUseDecisionBuffer, AnthropicToolUseDecisionBuffer, AnthropicToolUseDecisionPassthrough}
	for i, raw := range events {
		var event anthropic.BetaRawMessageStreamEventUnion
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			t.Fatalf("decode event %d: %v", i, err)
		}
		kind, _, err := RewriteAnthropicToolUseEventDecision(nil, state, &event)
		if err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
		if kind != want[i] {
			t.Fatalf("event %d decision = %q, want %q", i, kind, want[i])
		}
	}
}

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

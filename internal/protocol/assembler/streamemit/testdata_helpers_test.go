package streamemit

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// v1Event JSON-unmarshals a raw Anthropic v1 stream event into a typed
// MessageStreamEventUnion. Failing here indicates a malformed fixture.
func v1Event(t *testing.T, raw string) *anthropic.MessageStreamEventUnion {
	t.Helper()
	var evt anthropic.MessageStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &evt); err != nil {
		t.Fatalf("v1Event unmarshal failed: %v\nraw: %s", err, raw)
	}
	return &evt
}

// betaEvent JSON-unmarshals a raw Anthropic v1beta stream event into a
// typed BetaRawMessageStreamEventUnion.
func betaEvent(t *testing.T, raw string) *anthropic.BetaRawMessageStreamEventUnion {
	t.Helper()
	var evt anthropic.BetaRawMessageStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &evt); err != nil {
		t.Fatalf("betaEvent unmarshal failed: %v\nraw: %s", err, raw)
	}
	return &evt
}

// Fixture builders. Each returns a JSON string that can be passed to
// v1Event or betaEvent.

func fxMessageStart(id string) string {
	return fmt.Sprintf(`{
		"type": "message_start",
		"message": {
			"id": %q,
			"type": "message",
			"role": "assistant",
			"content": [],
			"model": "claude-test",
			"stop_reason": null,
			"stop_sequence": null,
			"usage": {"input_tokens": 1, "output_tokens": 0}
		}
	}`, id)
}

func fxMessageDelta(stopReason string, outTokens int) string {
	return fmt.Sprintf(`{
		"type": "message_delta",
		"delta": {"stop_reason": %q, "stop_sequence": null},
		"usage": {"input_tokens": 0, "output_tokens": %d}
	}`, stopReason, outTokens)
}

func fxMessageStop() string {
	return `{"type": "message_stop"}`
}

func fxTextBlockStart(index int) string {
	return fmt.Sprintf(`{
		"type": "content_block_start",
		"index": %d,
		"content_block": {"type": "text", "text": ""}
	}`, index)
}

func fxThinkingBlockStart(index int) string {
	return fmt.Sprintf(`{
		"type": "content_block_start",
		"index": %d,
		"content_block": {"type": "thinking", "thinking": "", "signature": ""}
	}`, index)
}

func fxToolUseBlockStart(index int, id, name string) string {
	return fmt.Sprintf(`{
		"type": "content_block_start",
		"index": %d,
		"content_block": {"type": "tool_use", "id": %q, "name": %q, "input": {}}
	}`, index, id, name)
}

func fxTextDelta(index int, text string) string {
	textJSON, _ := json.Marshal(text)
	return fmt.Sprintf(`{
		"type": "content_block_delta",
		"index": %d,
		"delta": {"type": "text_delta", "text": %s}
	}`, index, textJSON)
}

func fxInputJSONDelta(index int, partial string) string {
	pj, _ := json.Marshal(partial)
	return fmt.Sprintf(`{
		"type": "content_block_delta",
		"index": %d,
		"delta": {"type": "input_json_delta", "partial_json": %s}
	}`, index, pj)
}

func fxBlockStop(index int) string {
	return fmt.Sprintf(`{"type": "content_block_stop", "index": %d}`, index)
}

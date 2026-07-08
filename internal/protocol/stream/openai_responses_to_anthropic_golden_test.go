package stream

import (
	"context"
	"encoding/json"
	"testing"

	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResponsesToAnthropicConverter_GoldenSequence is a hermetic regression oracle
// for the responsesToAnthropicConverter. It feeds a realistic Responses API stream
// (text deltas + a tool call assembled across two delta events, then
// response.completed with usage) through the full Next() iterator and asserts the
// exact ordered sequence of emitted Anthropic SSE events plus key payload fields.
func TestResponsesToAnthropicConverter_GoldenSequence(t *testing.T) {
	events := []map[string]any{
		// 1: no-op — message_start is emitted before the first upstream read
		{"type": "response.created", "response": map[string]any{"id": "resp_golden"}},
		// 2: opens text block (textBlockIndex==-1) + emits delta "Hello"
		{"type": "response.output_text.delta", "item_id": "item_1", "output_index": 0, "delta": "Hello"},
		// 3: text delta ", World!"
		{"type": "response.output_text.delta", "item_id": "item_1", "output_index": 0, "delta": ", World!"},
		// 4: closes text block (content_block_stop)
		{"type": "response.output_text.done", "item_id": "item_1", "output_index": 0, "text": "Hello, World!"},
		// 5: opens tool call block (content_block_start tool_use)
		{"type": "response.output_item.added", "output_index": 1, "item": map[string]any{
			"id": "fc_1", "type": "function_call", "call_id": "call_1",
			"name": "get_weather", "status": "in_progress",
		}},
		// 6: first args delta (content_block_delta input_json_delta)
		{"type": "response.function_call_arguments.delta", "item_id": "fc_1", "output_index": 1, "delta": `{"city":`},
		// 7: second args delta
		{"type": "response.function_call_arguments.delta", "item_id": "fc_1", "output_index": 1, "delta": `"Paris"}`},
		// 8: closes tool call block (content_block_stop via function_call_arguments.done)
		{"type": "response.function_call_arguments.done", "item_id": "fc_1", "output_index": 1, "arguments": `{"city":"Paris"}`},
		// 9: terminal → message_delta (stop_reason=tool_use) + message_stop
		{"type": "response.completed", "response": map[string]any{
			"id": "resp_golden", "status": "completed",
			"output": []any{
				map[string]any{
					"id": "msg_1", "type": "message", "role": "assistant", "status": "completed",
					"content": []any{map[string]any{"type": "output_text", "text": "Hello, World!"}},
				},
				map[string]any{
					"id": "fc_1", "type": "function_call", "call_id": "call_1",
					"name": "get_weather", "arguments": `{"city":"Paris"}`,
				},
			},
			"usage": map[string]any{
				"input_tokens": 10, "output_tokens": 5, "total_tokens": 15,
				"input_tokens_details":  map[string]any{"cached_tokens": 0},
				"output_tokens_details": map[string]any{"reasoning_tokens": 0},
			},
		}},
	}

	stream := openaistream.NewStream[responses.ResponseStreamEventUnion](
		newFakeResponsesDecoder(eventsToStrings(events)), nil,
	)
	conv := newResponsesToAnthropicConverter(context.Background(), stream, "gpt-4o-mini")

	var got []anthropicStreamEvent
	for {
		evt, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		ae, ok := evt.(anthropicStreamEvent)
		require.Truef(t, ok, "emitted event %T is not anthropicStreamEvent", evt)
		got = append(got, ae)
	}

	// 1. Exact ordered event type sequence.
	want := []string{
		"message_start",
		"content_block_start", // text, block 0
		"content_block_delta", // text "Hello"
		"content_block_delta", // text ", World!"
		"content_block_stop",  // block 0
		"content_block_start", // tool_use, block 1
		"content_block_delta", // input_json {"city":
		"content_block_delta", // input_json "Paris"}
		"content_block_stop",  // block 1
		"message_delta",
		"message_stop",
	}
	gotTypes := make([]string, len(got))
	for i, e := range got {
		gotTypes[i] = e.eventType
	}
	require.Equal(t, want, gotTypes, "ordered Anthropic event sequence")

	// 2. Spot-check key payload fields on the wire shapes. Events are typed
	// structs (or maps for message_delta); normalize through JSON so the
	// assertions check exactly what a client would receive.

	// message_start carries role="assistant"
	msgData := eventDataAsMap(t, got[0].data)["message"].(map[string]interface{})
	assert.Equal(t, "assistant", msgData["role"])

	// text content_block_start
	textBlockStart := eventDataAsMap(t, got[1].data)
	assert.Equal(t, float64(0), textBlockStart["index"])
	assert.Equal(t, "text", textBlockStart["content_block"].(map[string]interface{})["type"])

	// text deltas
	delta1 := eventDataAsMap(t, got[2].data)["delta"].(map[string]interface{})
	assert.Equal(t, "text_delta", delta1["type"])
	assert.Equal(t, "Hello", delta1["text"])

	delta2 := eventDataAsMap(t, got[3].data)["delta"].(map[string]interface{})
	assert.Equal(t, ", World!", delta2["text"])

	// tool_use content_block_start
	toolBlockStart := eventDataAsMap(t, got[5].data)
	toolBlock := toolBlockStart["content_block"].(map[string]interface{})
	assert.Equal(t, "tool_use", toolBlock["type"])
	assert.Equal(t, "get_weather", toolBlock["name"])

	// args delta
	argsDelta := eventDataAsMap(t, got[6].data)["delta"].(map[string]interface{})
	assert.Equal(t, "input_json_delta", argsDelta["type"])
	assert.Equal(t, `{"city":`, argsDelta["partial_json"])

	// message_delta carries stop_reason=tool_use
	msgDelta := eventDataAsMap(t, got[9].data)["delta"].(map[string]interface{})
	assert.Equal(t, "tool_use", msgDelta["stop_reason"])

	// 3. Usage from response.completed.
	usage := conv.Usage()
	assert.Equal(t, 10, usage.InputTokens)
	assert.Equal(t, 5, usage.OutputTokens)
}

// eventDataAsMap normalizes an event payload (typed wire struct or map) to a
// generic map through JSON, mirroring what reaches the client.
func eventDataAsMap(t *testing.T, data any) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// eventsToStrings marshals a slice of event maps to JSON strings.
func eventsToStrings(events []map[string]any) []string {
	strs := make([]string, len(events))
	for i, e := range events {
		b, _ := json.Marshal(e)
		strs[i] = string(b)
	}
	return strs
}

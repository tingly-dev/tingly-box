package stream

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// seqOfFull extracts the SequenceNumber from any Responses API event type emitted
// by the anthropicBetaToResponsesConverter, including ContentPart events that
// seqOf in the ChatToResponses golden test does not cover.
func seqOfFull(t *testing.T, e wire.ResponsesEvent) int64 {
	t.Helper()
	switch v := e.(type) {
	case wire.ResponsesCreatedEvent:
		return v.SequenceNumber
	case wire.ResponsesInProgressEvent:
		return v.SequenceNumber
	case wire.ResponsesOutputItemAddedEvent:
		return v.SequenceNumber
	case wire.ResponsesContentPartAddedEvent:
		return v.SequenceNumber
	case wire.ResponsesOutputTextDeltaEvent:
		return v.SequenceNumber
	case wire.ResponsesFunctionCallArgumentsDeltaEvent:
		return v.SequenceNumber
	case wire.ResponsesOutputTextDoneEvent:
		return v.SequenceNumber
	case wire.ResponsesContentPartDoneEvent:
		return v.SequenceNumber
	case wire.ResponsesOutputItemDoneEvent:
		return v.SequenceNumber
	case wire.ResponsesFunctionCallArgumentsDoneEvent:
		return v.SequenceNumber
	case wire.ResponsesCompletedEvent:
		return v.SequenceNumber
	default:
		t.Fatalf("unexpected event type %T", e)
		return 0
	}
}

// TestAnthropicBetaToResponsesConverter_GoldenSequence is a hermetic regression
// oracle for the anthropicBetaToResponsesConverter. It feeds a realistic Anthropic
// Beta stream (text block + tool call assembled across two delta chunks, then
// message_stop) through the full Next() iterator and asserts the exact ordered
// sequence of emitted Responses API events plus key payload fields and final usage.
func TestAnthropicBetaToResponsesConverter_GoldenSequence(t *testing.T) {
	marshal := func(v map[string]any) string {
		b, _ := json.Marshal(v)
		return string(b)
	}

	events := []string{
		// 1: message_start → response.created + response.in_progress
		marshal(map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg_golden", "type": "message", "role": "assistant",
				"content": []any{}, "model": "claude-3-5-sonnet-20241022",
				"stop_reason": nil, "stop_sequence": nil,
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 0},
			},
		}),
		// 2: content_block_start text → output_item.added + content_part.added
		marshal(map[string]any{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": ""},
		}),
		// 3: text delta "Hello" → output_text.delta
		marshal(map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": "Hello"},
		}),
		// 4: text delta ", World!" → output_text.delta
		marshal(map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": ", World!"},
		}),
		// 5: content_block_stop text → output_text.done + content_part.done + output_item.done
		marshal(map[string]any{"type": "content_block_stop", "index": 0}),
		// 6: content_block_start tool_use → output_item.added (function_call)
		marshal(map[string]any{
			"type": "content_block_start", "index": 1,
			"content_block": map[string]any{
				"type": "tool_use", "id": "tool_1", "name": "get_weather",
				"input": map[string]any{},
			},
		}),
		// 7: first args fragment → function_call_arguments.delta
		marshal(map[string]any{
			"type": "content_block_delta", "index": 1,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": `{"city":`},
		}),
		// 8: second args fragment → function_call_arguments.delta
		marshal(map[string]any{
			"type": "content_block_delta", "index": 1,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": `"Paris"}`},
		}),
		// 9: content_block_stop tool → function_call_arguments.done + output_item.done
		marshal(map[string]any{"type": "content_block_stop", "index": 1}),
		// 10: message_delta (output_tokens accumulation only, no Responses events)
		marshal(map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": "tool_use", "stop_sequence": nil},
			"usage": map[string]any{"output_tokens": 5},
		}),
		// 11: message_stop → response.completed
		marshal(map[string]any{"type": "message_stop"}),
	}

	stream := anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](
		newFakeAnthropicDecoder(events), nil,
	)
	conv := newAnthropicBetaToResponsesConverter(stream, "claude-3-5-sonnet-20241022")

	var got []wire.ResponsesEvent
	for {
		evt, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		re, ok := evt.(wire.ResponsesEvent)
		require.Truef(t, ok, "emitted event %T does not implement wire.ResponsesEvent", evt)
		got = append(got, re)
	}

	// 1. Exact ordered event sequence — the heart of the oracle.
	want := []string{
		"response.created",
		"response.in_progress",
		"response.output_item.added", // message item
		"response.content_part.added",
		"response.output_text.delta", // "Hello"
		"response.output_text.delta", // ", World!"
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",              // message item done
		"response.output_item.added",             // function_call item
		"response.function_call_arguments.delta", // {"city":
		"response.function_call_arguments.delta", // "Paris"}
		"response.function_call_arguments.done",
		"response.output_item.done", // function_call item done
		"response.completed",
	}
	gotTypes := make([]string, len(got))
	for i, e := range got {
		gotTypes[i] = e.EventType()
	}
	require.Equal(t, want, gotTypes, "ordered Responses event sequence")

	// 2. Sequence numbers start at 0 and increment by 1 (no gaps/dupes).
	for i, e := range got {
		assert.Equal(t, int64(i), seqOfFull(t, e), "sequence number at position %d", i)
	}

	// 3. Spot-check key payloads.
	assert.Equal(t, "Hello", got[4].(wire.ResponsesOutputTextDeltaEvent).Delta)
	assert.Equal(t, ", World!", got[5].(wire.ResponsesOutputTextDeltaEvent).Delta)

	textDone := got[6].(wire.ResponsesOutputTextDoneEvent)
	assert.Equal(t, "Hello, World!", textDone.Text)

	argsDone := got[12].(wire.ResponsesFunctionCallArgumentsDoneEvent)
	assert.Equal(t, `{"city":"Paris"}`, argsDone.Arguments)

	completed := got[14].(wire.ResponsesCompletedEvent)
	assert.Equal(t, "completed", completed.Response.Status)

	// 4. Usage reflects upstream message_start (input) + message_delta (output).
	usage := conv.Usage()
	assert.Equal(t, 10, usage.InputTokens)
	assert.Equal(t, 5, usage.OutputTokens)
}

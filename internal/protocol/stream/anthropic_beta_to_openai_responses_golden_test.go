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
	conv := newAnthropicBetaToResponsesConverter(stream, "claude-3-5-sonnet-20241022", nil)

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
		assert.Equal(t, int64(i), seqOf(t, e), "sequence number at position %d", i)
	}

	// 3. Spot-check key payloads.
	assert.Equal(t, "Hello", got[4].(wire.ResponsesOutputTextDeltaEvent).Delta)
	assert.Equal(t, ", World!", got[5].(wire.ResponsesOutputTextDeltaEvent).Delta)

	textDone := got[6].(wire.ResponsesOutputTextDoneEvent)
	assert.Equal(t, "Hello, World!", textDone.Text)
	assert.Equal(t, 0, textDone.OutputIndex)
	assert.Equal(t, 0, got[2].(wire.ResponsesOutputItemAddedEvent).OutputIndex)
	assert.Equal(t, 0, got[3].(wire.ResponsesContentPartAddedEvent).OutputIndex)

	argsDone := got[12].(wire.ResponsesFunctionCallArgumentsDoneEvent)
	assert.Equal(t, `{"city":"Paris"}`, argsDone.Arguments)
	assert.Equal(t, 1, got[9].(wire.ResponsesOutputItemAddedEvent).OutputIndex)
	assert.Equal(t, 1, got[10].(wire.ResponsesFunctionCallArgumentsDeltaEvent).OutputIndex)
	assert.Equal(t, 1, got[11].(wire.ResponsesFunctionCallArgumentsDeltaEvent).OutputIndex)
	assert.Equal(t, 1, argsDone.OutputIndex)
	assert.Equal(t, 1, got[13].(wire.ResponsesOutputItemDoneEvent).OutputIndex)

	completed := got[14].(wire.ResponsesCompletedEvent)
	assert.Equal(t, "completed", completed.Response.Status)
	assert.Equal(t, "claude-3-5-sonnet-20241022", completed.Response.Model)
	require.Len(t, completed.Response.Output, 2)
	assert.Equal(t, "message", completed.Response.Output[0].Type)
	assert.Equal(t, "function_call", completed.Response.Output[1].Type)

	// 4. Usage reflects upstream message_start (input) + message_delta (output).
	usage := conv.Usage()
	assert.Equal(t, 10, usage.InputTokens)
	assert.Equal(t, 5, usage.OutputTokens)
}

func TestAnthropicBetaToResponsesConverter_PreservesInterleavedTextBlocks(t *testing.T) {
	marshal := func(v map[string]any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return string(b)
	}

	events := []string{
		marshal(map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg_interleaved", "type": "message", "role": "assistant",
				"content": []any{}, "model": "claude-test",
				"stop_reason": nil, "stop_sequence": nil,
				"usage": map[string]any{"input_tokens": 3, "output_tokens": 0},
			},
		}),
		marshal(map[string]any{
			"type": "content_block_start", "index": 0,
			"content_block": map[string]any{"type": "text", "text": ""},
		}),
		marshal(map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": "before"},
		}),
		marshal(map[string]any{"type": "content_block_stop", "index": 0}),
		marshal(map[string]any{
			"type": "content_block_start", "index": 1,
			"content_block": map[string]any{
				"type": "tool_use", "id": "tool_interleaved", "name": "lookup",
				"input": map[string]any{},
			},
		}),
		marshal(map[string]any{
			"type": "content_block_delta", "index": 1,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": `{}`},
		}),
		marshal(map[string]any{"type": "content_block_stop", "index": 1}),
		marshal(map[string]any{
			"type": "content_block_start", "index": 2,
			"content_block": map[string]any{"type": "text", "text": ""},
		}),
		marshal(map[string]any{
			"type": "content_block_delta", "index": 2,
			"delta": map[string]any{"type": "text_delta", "text": "after"},
		}),
		marshal(map[string]any{"type": "content_block_stop", "index": 2}),
		marshal(map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": "tool_use", "stop_sequence": nil},
			"usage": map[string]any{"output_tokens": 4},
		}),
		marshal(map[string]any{"type": "message_stop"}),
	}

	stream := anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](
		newFakeAnthropicDecoder(events), nil,
	)
	conv := newAnthropicBetaToResponsesConverter(stream, "claude-test", nil)

	var added []wire.ResponsesOutputItemAddedEvent
	var done []wire.ResponsesOutputItemDoneEvent
	var completed wire.ResponsesCompletedEvent
	for {
		event, finished, err := conv.Next()
		require.NoError(t, err)
		if finished {
			break
		}
		switch typed := event.(type) {
		case wire.ResponsesOutputItemAddedEvent:
			added = append(added, typed)
		case wire.ResponsesOutputItemDoneEvent:
			done = append(done, typed)
		case wire.ResponsesCompletedEvent:
			completed = typed
		}
	}

	require.Len(t, added, 3)
	require.Len(t, done, 3)
	for index := range added {
		assert.Equal(t, index, added[index].OutputIndex)
		assert.Equal(t, index, done[index].OutputIndex)
		assert.Equal(t, added[index].Item.ID, done[index].Item.ID)
	}
	assert.NotEqual(t, added[0].Item.ID, added[2].Item.ID)

	require.Len(t, completed.Response.Output, 3)
	assert.Equal(t, "message", completed.Response.Output[0].Type)
	assert.Equal(t, "before", completed.Response.Output[0].Content[0].Text)
	assert.Equal(t, "function_call", completed.Response.Output[1].Type)
	assert.Equal(t, "message", completed.Response.Output[2].Type)
	assert.Equal(t, "after", completed.Response.Output[2].Content[0].Text)
	assert.Equal(t, added[0].Item.ID, completed.Response.Output[0].ID)
	assert.Equal(t, added[2].Item.ID, completed.Response.Output[2].ID)
}

// The message item's response.output_item.added must serialize "content":[],
// or Codex drops every output_text delta that follows (#1922). A function_call
// item stays without content.
func TestAnthropicBetaToResponsesConverter_OutputItemAddedWire(t *testing.T) {
	marshal := func(v map[string]any) string {
		b, _ := json.Marshal(v)
		return string(b)
	}
	events := []string{
		marshal(map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg_wire", "type": "message", "role": "assistant", "content": []any{},
				"model": "claude-3-5-sonnet-20241022", "usage": map[string]any{"input_tokens": 1, "output_tokens": 0},
			},
		}),
		marshal(map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}}),
		marshal(map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "hello"}}),
		marshal(map[string]any{"type": "content_block_stop", "index": 0}),
		marshal(map[string]any{
			"type": "content_block_start", "index": 1,
			"content_block": map[string]any{"type": "tool_use", "id": "tool_1", "name": "get_weather", "input": map[string]any{}},
		}),
		marshal(map[string]any{"type": "content_block_stop", "index": 1}),
		marshal(map[string]any{"type": "message_stop"}),
	}
	conv := newAnthropicBetaToResponsesConverter(
		anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](newFakeAnthropicDecoder(events), nil),
		"claude-3-5-sonnet-20241022",
		nil,
	)

	var added []map[string]json.RawMessage
	for {
		evt, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		if e, ok := evt.(wire.ResponsesOutputItemAddedEvent); ok {
			b, err := json.Marshal(e)
			require.NoError(t, err)
			var raw struct {
				Item map[string]json.RawMessage `json:"item"`
			}
			require.NoError(t, json.Unmarshal(b, &raw))
			added = append(added, raw.Item)
		}
	}

	require.Len(t, added, 2)
	assert.JSONEq(t, `"message"`, string(added[0]["type"]))
	assert.JSONEq(t, `[]`, string(added[0]["content"]), "message item must carry content")
	assert.JSONEq(t, `"function_call"`, string(added[1]["type"]))
	assert.NotContains(t, added[1], "content")
}

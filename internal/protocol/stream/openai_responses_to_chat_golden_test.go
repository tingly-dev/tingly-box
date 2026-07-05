package stream

import (
	"encoding/json"
	"testing"

	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// newResponsesIterFromEvents builds a ResponsesStreamIter from a slice of JSON
// event maps, reusing the fakeResponsesDecoder already in the test package.
func newResponsesIterFromEvents(events []map[string]any) ResponsesStreamIter {
	strs := make([]string, len(events))
	for i, e := range events {
		b, _ := json.Marshal(e)
		strs[i] = string(b)
	}
	return openaistream.NewStream[responses.ResponseStreamEventUnion](newFakeResponsesDecoder(strs), nil)
}

// TestResponsesToChatConverter_GoldenSequence is a hermetic regression oracle for
// the responsesToChatConverter. It feeds a realistic multi-event Responses API
// stream (text deltas + a tool call assembled across two chunks, then a terminal
// response.completed with usage) through the full Next() iterator and asserts the
// exact ordered sequence of emitted Chat Completion chunks plus key payload fields.
func TestResponsesToChatConverter_GoldenSequence(t *testing.T) {
	events := []map[string]any{
		{ // 1: response.created → role chunk (assistant)
			"type": "response.created",
			"response": map[string]any{
				"id": "resp_golden", "object": "response",
				"status": "in_progress", "output": []any{},
			},
		},
		{ // 2: first text delta
			"type":          "response.output_text.delta",
			"item_id":       "item_1",
			"output_index":  0,
			"content_index": 0,
			"delta":         "Hello",
		},
		{ // 3: second text delta
			"type":          "response.output_text.delta",
			"item_id":       "item_1",
			"output_index":  0,
			"content_index": 0,
			"delta":         ", World!",
		},
		{ // 4: function call opens → tool start chunk
			"type":         "response.output_item.added",
			"output_index": 1,
			"item": map[string]any{
				"id": "fc_1", "type": "function_call", "call_id": "call_1",
				"name": "get_weather", "status": "in_progress",
			},
		},
		{ // 5: first args fragment → tool delta chunk
			"type":         "response.function_call_arguments.delta",
			"item_id":      "fc_1",
			"output_index": 1,
			"call_id":      "call_1",
			"delta":        `{"city":`,
		},
		{ // 6: second args fragment → tool delta chunk
			"type":         "response.function_call_arguments.delta",
			"item_id":      "fc_1",
			"output_index": 1,
			"call_id":      "call_1",
			"delta":        `"Paris"}`,
		},
		{ // 7: args done — no chunk emitted when deltas already accumulated
			"type":         "response.function_call_arguments.done",
			"item_id":      "fc_1",
			"output_index": 1,
			"call_id":      "call_1",
			"arguments":    `{"city":"Paris"}`,
		},
		{ // 8: stream complete → final chunk with finish_reason=tool_calls + usage
			"type": "response.completed",
			"response": map[string]any{
				"id": "resp_golden", "object": "response", "status": "completed",
				"output": []any{
					map[string]any{
						"id": "msg_1", "type": "message", "role": "assistant",
						"status":  "completed",
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
			},
		},
	}

	conv := newResponsesToChatConverter(newResponsesIterFromEvents(events), "gpt-4o-mini", false)

	var got []wire.ChatStreamChunk
	for {
		evt, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		chunk, ok := evt.(wire.ChatStreamChunk)
		require.Truef(t, ok, "emitted event %T is not wire.ChatStreamChunk", evt)
		got = append(got, chunk)
	}

	// 1. Exact chunk count: role + 2 text + tool start + 2 tool deltas + final.
	require.Len(t, got, 7, "expected 7 chunks: role, 2 text deltas, tool start, 2 tool deltas, final")

	// 2. Chunk 0: role chunk.
	assert.Equal(t, "assistant", got[0].Choices[0].Delta.Role)
	assert.Empty(t, got[0].Choices[0].Delta.Content)
	assert.Nil(t, got[0].Choices[0].FinishReason)

	// 3. Text deltas.
	assert.Equal(t, "Hello", got[1].Choices[0].Delta.Content)
	assert.Equal(t, ", World!", got[2].Choices[0].Delta.Content)

	// 4. Tool call start chunk.
	require.Len(t, got[3].Choices[0].Delta.ToolCalls, 1)
	tc := got[3].Choices[0].Delta.ToolCalls[0]
	assert.Equal(t, 1, tc.Index)
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "function", tc.Type)
	assert.Equal(t, "get_weather", tc.Function.Name)
	require.NotNil(t, tc.Function.Arguments)
	assert.Equal(t, "", *tc.Function.Arguments)

	// 5. Tool call delta chunks.
	require.Len(t, got[4].Choices[0].Delta.ToolCalls, 1)
	require.NotNil(t, got[4].Choices[0].Delta.ToolCalls[0].Function.Arguments)
	assert.Equal(t, `{"city":`, *got[4].Choices[0].Delta.ToolCalls[0].Function.Arguments)
	assert.Equal(t, `"Paris"}`, *got[5].Choices[0].Delta.ToolCalls[0].Function.Arguments)

	// 6. Final chunk: finish_reason=tool_calls + usage.
	require.NotNil(t, got[6].Choices[0].FinishReason)
	assert.Equal(t, "tool_calls", *got[6].Choices[0].FinishReason)
	require.NotNil(t, got[6].Usage)
	assert.Equal(t, int64(10), got[6].Usage.PromptTokens)
	assert.Equal(t, int64(5), got[6].Usage.CompletionTokens)
	assert.Equal(t, int64(15), got[6].Usage.TotalTokens)

	// 7. Usage accessor reflects response.completed token counts.
	usage := conv.Usage()
	assert.Equal(t, 10, usage.InputTokens)
	assert.Equal(t, 5, usage.OutputTokens)
}

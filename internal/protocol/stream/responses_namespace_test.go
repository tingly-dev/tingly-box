package stream

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/toolnamespace"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

var linearNamespaces = toolnamespace.Index{
	"mcp__linear__get_issue": {Namespace: "mcp__linear__", Name: "get_issue"},
}

// functionCallItems collects every function_call item a converter emitted, in
// both output_item.added/done events and the final response.completed output.
func functionCallItems(events []wire.ResponsesEvent) []wire.ResponsesOutputItemWire {
	var items []wire.ResponsesOutputItemWire
	for _, event := range events {
		switch e := event.(type) {
		case wire.ResponsesOutputItemAddedEvent:
			items = append(items, e.Item)
		case wire.ResponsesOutputItemDoneEvent:
			items = append(items, e.Item)
		case wire.ResponsesCompletedEvent:
			items = append(items, e.Response.Output...)
		}
	}
	var calls []wire.ResponsesOutputItemWire
	for _, item := range items {
		if item.Type == "function_call" {
			calls = append(calls, item)
		}
	}
	return calls
}

func TestChatToResponsesConverter_RestoresNamespace(t *testing.T) {
	conv := NewChatToResponsesConverter(nil, "gpt-5", linearNamespaces)
	conv.hasSentCreated = true

	conv.processChunk(&openai.ChatCompletionChunk{Choices: []openai.ChatCompletionChunkChoice{{
		Delta: openai.ChatCompletionChunkChoiceDelta{ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{{
			Index: 0,
			ID:    "call_1",
			Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
				Name: "mcp__linear__get_issue", Arguments: `{"id":"A-1"}`,
			},
		}}},
	}}})
	conv.processChunk(&openai.ChatCompletionChunk{Choices: []openai.ChatCompletionChunkChoice{{FinishReason: "tool_calls"}}})
	conv.emitCompletionEvents()

	calls := functionCallItems(conv.pending)
	require.NotEmpty(t, calls)
	for _, call := range calls {
		assert.Equal(t, "mcp__linear__", call.Namespace)
		assert.Equal(t, "get_issue", call.Name)
	}
}

func TestAnthropicBetaToResponsesConverter_RestoresNamespace(t *testing.T) {
	marshal := func(v map[string]any) string {
		b, _ := json.Marshal(v)
		return string(b)
	}
	events := []string{
		marshal(map[string]any{"type": "message_start", "message": map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant", "content": []any{}, "model": "claude",
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 0},
		}}),
		marshal(map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{
			"type": "tool_use", "id": "toolu_1", "name": "mcp__linear__get_issue", "input": map[string]any{},
		}}),
		marshal(map[string]any{"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": `{"id":"A-1"}`}}),
		marshal(map[string]any{"type": "content_block_stop", "index": 0}),
		marshal(map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use"},
			"usage": map[string]any{"output_tokens": 2}}),
		marshal(map[string]any{"type": "message_stop"}),
	}
	stream := anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](newFakeAnthropicDecoder(events), nil)
	conv := NewAnthropicBetaToOpenAIResponsesConverter(stream, "gpt-5", linearNamespaces)

	var got []wire.ResponsesEvent
	for {
		evt, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		got = append(got, evt.(wire.ResponsesEvent))
	}

	calls := functionCallItems(got)
	require.Len(t, calls, 3, "added, done and the completed output")
	for _, call := range calls {
		assert.Equal(t, "mcp__linear__", call.Namespace)
		assert.Equal(t, "get_issue", call.Name)
	}
}

// Parallel calls: a namespaced and a plain call in one answer must each come
// out with their own namespace and name, not swapped or shared.
func TestChatToResponsesConverter_ParallelCallsKeepTheirOwnNamespace(t *testing.T) {
	namespaces := toolnamespace.Index{
		"mcp__linear__get_issue":  {Namespace: "mcp__linear__", Name: "get_issue"},
		"mcp__github__get_issue":  {Namespace: "mcp__github__", Name: "get_issue"},
		"mcp__github__list_repos": {Namespace: "mcp__github__", Name: "list_repos"},
	}
	conv := NewChatToResponsesConverter(nil, "gpt-5", namespaces)
	conv.hasSentCreated = true

	call := func(index int, id, name, args string) openai.ChatCompletionChunkChoiceDeltaToolCall {
		return openai.ChatCompletionChunkChoiceDeltaToolCall{
			Index: int64(index), ID: id,
			Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: name, Arguments: args},
		}
	}
	conv.processChunk(&openai.ChatCompletionChunk{Choices: []openai.ChatCompletionChunkChoice{{
		Delta: openai.ChatCompletionChunkChoiceDelta{ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
			call(0, "call_a", "mcp__linear__get_issue", `{"id":"L-1"}`),
			call(1, "call_b", "shell", `{"cmd":"ls"}`),
			call(2, "call_c", "mcp__github__get_issue", `{"id":"G-1"}`),
		}},
	}}})
	conv.processChunk(&openai.ChatCompletionChunk{Choices: []openai.ChatCompletionChunkChoice{{FinishReason: "tool_calls"}}})
	conv.emitCompletionEvents()

	got := map[string][2]string{}
	for _, item := range functionCallItems(conv.pending) {
		got[item.CallID] = [2]string{item.Namespace, item.Name}
	}
	assert.Equal(t, map[string][2]string{
		"call_a": {"mcp__linear__", "get_issue"},
		"call_b": {"", "shell"},
		"call_c": {"mcp__github__", "get_issue"},
	}, got)
}

// A stream cut short must not turn a namespaced call into a finished one.
func TestChatToResponsesConverter_TruncatedNamespaceCallStaysIncomplete(t *testing.T) {
	conv := NewChatToResponsesConverter(nil, "gpt-5", linearNamespaces)
	conv.hasSentCreated = true

	conv.processChunk(&openai.ChatCompletionChunk{Choices: []openai.ChatCompletionChunkChoice{{
		Delta: openai.ChatCompletionChunkChoiceDelta{ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{{
			Index: 0, ID: "call_1",
			Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: "mcp__linear__get_issue", Arguments: `{"id":`},
		}}},
		FinishReason: "length",
	}}})
	conv.emitCompletionEvents()

	var completed *wire.ResponsesOutputItemWire
	for _, event := range conv.pending {
		if e, ok := event.(wire.ResponsesCompletedEvent); ok {
			completed = &e.Response.Output[0]
		}
		if e, ok := event.(wire.ResponsesIncompleteEvent); ok {
			completed = &e.Response.Output[0]
		}
	}
	require.NotNil(t, completed)
	assert.Equal(t, "incomplete", completed.Status)
	assert.Equal(t, "mcp__linear__", completed.Namespace)
	assert.Equal(t, "get_issue", completed.Name)
}

func TestAnthropicBetaToResponsesConverter_ParallelCallsKeepTheirOwnNamespace(t *testing.T) {
	marshal := func(v map[string]any) string {
		b, _ := json.Marshal(v)
		return string(b)
	}
	toolStart := func(index int, id, name string) string {
		return marshal(map[string]any{"type": "content_block_start", "index": index, "content_block": map[string]any{
			"type": "tool_use", "id": id, "name": name, "input": map[string]any{},
		}})
	}
	toolStop := func(index int) string { return marshal(map[string]any{"type": "content_block_stop", "index": index}) }
	events := []string{
		marshal(map[string]any{"type": "message_start", "message": map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant", "content": []any{}, "model": "claude",
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 0},
		}}),
		toolStart(0, "toolu_a", "mcp__linear__get_issue"), toolStop(0),
		toolStart(1, "toolu_b", "shell"), toolStop(1),
		marshal(map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use"},
			"usage": map[string]any{"output_tokens": 2}}),
		marshal(map[string]any{"type": "message_stop"}),
	}
	stream := anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](newFakeAnthropicDecoder(events), nil)
	conv := NewAnthropicBetaToOpenAIResponsesConverter(stream, "gpt-5", linearNamespaces)

	var got []wire.ResponsesEvent
	for {
		evt, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		got = append(got, evt.(wire.ResponsesEvent))
	}

	byCall := map[string][2]string{}
	for _, item := range functionCallItems(got) {
		byCall[item.CallID] = [2]string{item.Namespace, item.Name}
	}
	assert.Equal(t, map[string][2]string{
		"toolu_a": {"mcp__linear__", "get_issue"},
		"toolu_b": {"", "shell"},
	}, byCall)
}

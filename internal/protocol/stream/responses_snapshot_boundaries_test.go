package stream

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestResponsesChatToolIndicesExcludeOtherOutput(t *testing.T) {
	first := map[string]any{"type": "function_call", "id": "fc1", "call_id": "call1", "name": "first", "arguments": ""}
	second := map[string]any{"type": "function_call", "id": "fc2", "call_id": "call2", "name": "second", "arguments": ""}
	events := []map[string]any{
		{"type": "response.output_item.added", "output_index": 2, "item": first},
		{"type": "response.function_call_arguments.delta", "output_index": 2, "item_id": "fc1", "delta": `{"a":`},
		{"type": "response.output_item.added", "output_index": 4, "item": second},
		{"type": "response.function_call_arguments.delta", "output_index": 4, "item_id": "fc2", "delta": `{"b":2}`},
		{"type": "response.function_call_arguments.delta", "output_index": 2, "item_id": "fc1", "delta": `1}`},
		{"type": "response.function_call_arguments.done", "output_index": 2, "item_id": "fc1", "arguments": `{"a":1}`},
		{"type": "response.function_call_arguments.done", "output_index": 4, "item_id": "fc2", "arguments": `{"b":2}`},
		{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{}}},
	}
	conv := NewOpenAIResponsesToChatConverter(newResponsesIterFromEvents(events), "model", false)
	ids := map[int]string{}
	args := map[int]*strings.Builder{}
	for i := 0; ; i++ {
		require.Less(t, i, 30)
		e, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		for _, tc := range e.(wire.ChatStreamChunk).Choices[0].Delta.ToolCalls {
			if tc.ID != "" {
				ids[tc.Index] = tc.ID
			}
			if args[tc.Index] == nil {
				args[tc.Index] = &strings.Builder{}
			}
			if tc.Function.Arguments != nil {
				args[tc.Index].WriteString(*tc.Function.Arguments)
			}
		}
	}
	require.Equal(t, map[int]string{0: "call1", 1: "call2"}, ids)
	require.Equal(t, `{"a":1}`, args[0].String())
	require.Equal(t, `{"b":2}`, args[1].String())
}

func TestResponsesSnapshotKeepsAlreadyEmittedPrefix(t *testing.T) {
	tracker := make(responseTextTracker)
	require.Equal(t, "hello", tracker.retain(0, 0, "text", "hello", false))
	require.Empty(t, tracker.retain(0, 0, "text", "hello", true), "repeated snapshot must not duplicate content")
	require.Empty(t, tracker.retain(0, 0, "text", "different", true), "a conflicting snapshot cannot retract already streamed bytes")
	require.Equal(t, " world", tracker.retain(0, 0, "text", "hello world", true))
	require.Equal(t, "separate", tracker.retain(1, 0, "text", "separate", true), "another item must remain independent")
	require.Equal(t, "part", tracker.retain(0, 1, "text", "part", true), "another content part must remain independent")
}

package stream

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestChatToResponsesUsageAfterFinishReason(t *testing.T) {
	raw := chunkSSE(t, []map[string]any{
		{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "summary"}}}},
		{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}},
		{"choices": []any{}, "usage": map[string]any{"prompt_tokens": 190000, "completion_tokens": 10, "total_tokens": 190010}},
	})
	conv := NewChatToResponsesConverter(newChatChunkStream(raw), "model", nil)
	var completed []wire.ResponsesCompletedEvent
	for {
		event, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		if e, ok := event.(wire.ResponsesCompletedEvent); ok {
			completed = append(completed, e)
		}
	}
	require.Len(t, completed, 1)
	require.EqualValues(t, 190000, completed[0].Response.Usage.InputTokens, "Codex's auto-compact trigger needs the trailing usage in the terminal response")
	require.EqualValues(t, 10, completed[0].Response.Usage.OutputTokens)
}

func TestChatToResponsesFailureAfterFinishIsNotSuccessfulCompletion(t *testing.T) {
	raw := `data: {"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"stop"}]}` + "\n\n" +
		`data: {"error":{"code":"server_error","message":"usage tail failed"}}` + "\n\n"
	conv := NewChatToResponsesConverter(newChatChunkStream(raw), "model", nil)
	for {
		event, done, err := conv.Next()
		if err != nil {
			require.ErrorContains(t, err, "usage tail failed")
			break
		}
		require.False(t, done, "upstream failure must not become success")
		_, completed := event.(wire.ResponsesCompletedEvent)
		require.False(t, completed)
	}
}

func TestAnthropicToResponsesInitialToolInput(t *testing.T) {
	for _, tc := range []struct{ name, initial, delta, want string }{
		{"complete start input", `{"cmd":"printf hello"}`, "", `{"cmd":"printf hello"}`},
		{"empty arguments", `{}`, "", `{}`},
		{"large integer input", `{"id":9007199254740993}`, "", `{"id":9007199254740993}`},
		{"streamed input", `{}`, `{"cmd":"printf streamed"}`, `{"cmd":"printf streamed"}`},
		{"deltas replace start snapshot", `{"cmd":"old"}`, `{"cmd":"new"}`, `{"cmd":"new"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conv := newAnthropicBetaToResponsesConverter(nil, "model", nil)
			feed := func(raw string) {
				var event anthropic.BetaRawMessageStreamEventUnion
				require.NoError(t, json.Unmarshal([]byte(raw), &event))
				conv.processEvent(&event)
			}
			feed(`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[]}}`)
			feed(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"exec_command","input":` + tc.initial + `}}`)
			if tc.delta != "" {
				encoded, _ := json.Marshal(tc.delta)
				feed(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":` + string(encoded) + `}}`)
			}
			feed(`{"type":"content_block_stop","index":0}`)
			feed(`{"type":"message_stop"}`)
			var delta strings.Builder
			var doneArguments, completedArguments string
			for _, e := range conv.pending {
				switch e := e.(type) {
				case wire.ResponsesFunctionCallArgumentsDeltaEvent:
					delta.WriteString(e.Delta)
				case wire.ResponsesOutputItemDoneEvent:
					if e.Item.Arguments != nil {
						doneArguments = *e.Item.Arguments
					}
				case wire.ResponsesCompletedEvent:
					completedArguments = *e.Response.Output[0].Arguments
				}
			}
			require.JSONEq(t, tc.want, delta.String())
			require.JSONEq(t, tc.want, doneArguments)
			require.JSONEq(t, tc.want, completedArguments)
			if tc.name == "large integer input" {
				require.Contains(t, doneArguments, "9007199254740993")
				require.Contains(t, completedArguments, "9007199254740993")
			}
		})
	}
}

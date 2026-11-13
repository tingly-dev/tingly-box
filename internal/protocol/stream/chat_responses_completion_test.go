package stream

import (
	"testing"

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

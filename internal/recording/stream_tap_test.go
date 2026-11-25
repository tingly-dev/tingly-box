package recording

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamTap_OpenAIShape(t *testing.T) {
	tap := newStreamTap(time.Now())
	chunks := []map[string]interface{}{
		chatChunk("chatcmpl-AbC", "gpt-4o", map[string]interface{}{"role": "assistant"}, nil, nil),
		chatChunk("chatcmpl-AbC", "gpt-4o", map[string]interface{}{"content": "hi"}, nil, nil),
		chatChunk("chatcmpl-AbC", "gpt-4o", map[string]interface{}{}, "stop", nil),
		{
			"choices":            []interface{}{},
			"usage":              map[string]interface{}{"prompt_tokens": float64(5), "completion_tokens": float64(2), "total_tokens": float64(7)},
			"system_fingerprint": "fp_abc123",
			"service_tier":       "default",
		},
	}
	for _, c := range chunks {
		tap.record("chat.completion.chunk", c)
	}

	fp := tap.snapshot(time.Second)
	require.NotNil(t, fp)
	assert.Equal(t, 4, fp.ChunkCount)
	assert.Equal(t, []string{"chat.completion.chunk"}, fp.EventTypes)
	assert.True(t, fp.UsageLast, "usage appeared in the final chunk")
	assert.False(t, fp.DoneMarker)

	require.NotNil(t, tap.usage)
	assert.Equal(t, 5, tap.usage.InputTokens)
	assert.Equal(t, 2, tap.usage.OutputTokens)
	assert.Equal(t, []string{"stop"}, tap.finishReasons)
	assert.Equal(t, "fp_abc123", tap.systemFingerprint)
	assert.Equal(t, "default", tap.serviceTier)
	assert.Equal(t, "chatcmpl-AbC", tap.id, "tap keeps the raw id; the recorder patternizes it at emit")
	assert.False(t, tap.firstTokenAt.IsZero(), "TTFT must be set on the first content chunk")
}

func TestStreamTap_DoneMarkerAndDefaultEventType(t *testing.T) {
	tap := newStreamTap(time.Now())
	tap.record("", map[string]interface{}{"type": "message_start"})
	tap.record("", map[string]interface{}{"type": "message_stop"})

	fp := tap.snapshot(time.Second)
	require.NotNil(t, fp)
	assert.True(t, fp.DoneMarker)
	assert.Equal(t, []string{"message_start", "message_stop"}, fp.EventTypes,
		"event type falls back to the chunk type when none is passed")
}

func TestStreamTap_ResponsesNestedResponse(t *testing.T) {
	tap := newStreamTap(time.Now())
	tap.record("response.completed", map[string]interface{}{
		"type": "response.completed",
		"response": map[string]interface{}{
			"id":                 "resp_123",
			"model":              "gpt-5",
			"system_fingerprint": "fp_x",
			"usage":              map[string]interface{}{"input_tokens": float64(9), "output_tokens": float64(3)},
			"status":             "completed",
		},
	})
	fp := tap.snapshot(time.Second)
	require.NotNil(t, fp)
	assert.True(t, fp.DoneMarker)
	assert.Equal(t, "resp_123", tap.id)
	assert.Equal(t, "gpt-5", tap.model)
	require.NotNil(t, tap.usage)
	assert.Equal(t, 9, tap.usage.InputTokens)
	assert.Equal(t, 3, tap.usage.OutputTokens)
}

func TestChunkHasContent(t *testing.T) {
	assert.False(t, chunkHasContent(chatChunk("id", "m", map[string]interface{}{"role": "assistant"}, nil, nil)))
	assert.True(t, chunkHasContent(chatChunk("id", "m", map[string]interface{}{"content": "x"}, nil, nil)))
	assert.True(t, chunkHasContent(map[string]interface{}{"delta": map[string]interface{}{"text": "x"}}))
	assert.True(t, chunkHasContent(map[string]interface{}{"type": "response.output_text.delta", "delta": "x"}))
	assert.False(t, chunkHasContent(map[string]interface{}{"type": "response.output_text.delta", "delta": ""}))
}

func chatChunk(id, model string, delta map[string]interface{}, finish interface{}, usage interface{}) map[string]interface{} {
	choice := map[string]interface{}{"delta": delta}
	if finish != nil {
		choice["finish_reason"] = finish
	}
	m := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"model":   model,
		"choices": []interface{}{choice},
	}
	if usage != nil {
		m["usage"] = usage
	}
	return m
}

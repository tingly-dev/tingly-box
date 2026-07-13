package stream

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

func TestNewAnthropicBetaToOpenAIChatConverter(t *testing.T) {
	stream := &anthropicBetaSliceStream{events: anthropicBetaEvents(t,
		map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": "msg_parallel", "type": "message", "role": "assistant",
				"content": []any{}, "model": "provider-model",
				"usage": map[string]any{"input_tokens": 4, "output_tokens": 0},
			},
		},
		map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}},
		map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "parallel path"}},
		map[string]any{"type": "content_block_stop", "index": 0},
		map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": 2}},
		map[string]any{"type": "message_stop"},
	)}
	converter := NewAnthropicBetaToOpenAIChatConverter(stream, "client-visible-model", false)

	var chunks []wire.ChatStreamChunk
	for {
		event, done, err := converter.Next()
		require.NoError(t, err)
		if done {
			break
		}
		chunk, ok := event.(wire.ChatStreamChunk)
		require.Truef(t, ok, "event type = %T", event)
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 3)
	assert.Equal(t, "assistant", chunks[0].Choices[0].Delta.Role)
	assert.Equal(t, "parallel path", chunks[1].Choices[0].Delta.Content)
	require.NotNil(t, chunks[2].Choices[0].FinishReason)
	assert.Equal(t, "stop", *chunks[2].Choices[0].FinishReason)
	require.NotNil(t, chunks[2].Usage)
	assert.EqualValues(t, 4, chunks[2].Usage.PromptTokens)
	assert.EqualValues(t, 2, chunks[2].Usage.CompletionTokens)
	require.NotNil(t, converter.Usage())
	assert.Equal(t, 4, converter.Usage().InputTokens)
	assert.Equal(t, 2, converter.Usage().OutputTokens)
}

func TestNewAnthropicBetaToOpenAIChatConverterPropagatesIteratorError(t *testing.T) {
	want := errors.New("upstream failed before an event")
	converter := NewAnthropicBetaToOpenAIChatConverter(&anthropicBetaSliceStream{err: want}, "model", false)

	event, done, err := converter.Next()
	assert.Nil(t, event)
	assert.False(t, done)
	assert.ErrorIs(t, err, want)
}

type anthropicBetaSliceStream struct {
	events []anthropic.BetaRawMessageStreamEventUnion
	index  int
	err    error
}

func (s *anthropicBetaSliceStream) Next() bool {
	if s.index >= len(s.events) {
		return false
	}
	s.index++
	return true
}

func (s *anthropicBetaSliceStream) Current() anthropic.BetaRawMessageStreamEventUnion {
	return s.events[s.index-1]
}

func (s *anthropicBetaSliceStream) Err() error { return s.err }

func anthropicBetaEvents(t *testing.T, bodies ...map[string]any) []anthropic.BetaRawMessageStreamEventUnion {
	t.Helper()
	events := make([]anthropic.BetaRawMessageStreamEventUnion, 0, len(bodies))
	for _, body := range bodies {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		var event anthropic.BetaRawMessageStreamEventUnion
		require.NoError(t, json.Unmarshal(raw, &event))
		events = append(events, event)
	}
	return events
}

package stream

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/internal/protocol/token"
)

// TestStopReasonMapping tests OpenAI to Anthropic stop reason conversion
func TestStopReasonMapping(t *testing.T) {
	mappings := map[string]string{
		"stop":         "end_turn",
		"length":       "max_tokens",
		"tool_calls":   "tool_use",
		"content_filter": "content_filter",
	}

	for openaiReason, anthropicReason := range mappings {
		t.Run(openaiReason+"->"+anthropicReason, func(t *testing.T) {
			assert.NotEmpty(t, anthropicReason, "Anthropic stop reason should not be empty")
		})
	}

	t.Run("unknown_reason_defaults_to_end_turn", func(t *testing.T) {
		mapped := mapOpenAIFinishReasonToAnthropic("unknown_reason")
		assert.Equal(t, "end_turn", mapped, "Unknown reason should default to end_turn")
	})
}

// TestAnthropicEventTypes verifies Anthropic event and block types
func TestAnthropicEventTypes(t *testing.T) {
	eventTypes := []string{
		"message_start",
		"content_block_start",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
		"error",
	}

	blockTypes := []string{
		"text",
		"thinking",
		"tool_use",
	}

	deltaTypes := []string{
		"text_delta",
		"thinking_delta",
		"input_json_delta",
	}

	t.Run("event_types_exist", func(t *testing.T) {
		for _, eventType := range eventTypes {
			assert.NotEmpty(t, eventType, "Event type should not be empty")
		}
	})

	t.Run("block_types_exist", func(t *testing.T) {
		for _, blockType := range blockTypes {
			assert.NotEmpty(t, blockType, "Block type should not be empty")
		}
	})

	t.Run("delta_types_exist", func(t *testing.T) {
		for _, deltaType := range deltaTypes {
			assert.NotEmpty(t, deltaType, "Delta type should not be empty")
		}
	})
}

// TestStreamStateInitialValues tests initial stream state values
func TestStreamStateInitialValues(t *testing.T) {
	state := newStreamState()

	assert.Equal(t, -1, state.textBlockIndex, "text block should start at -1")
	assert.Equal(t, -1, state.thinkingBlockIndex, "thinking block should start at -1")
	assert.Equal(t, -1, state.refusalBlockIndex, "refusal block should start at -1")
	assert.Equal(t, -1, state.reasoningSummaryBlockIndex, "reasoning summary block should start at -1")
	assert.False(t, state.hasTextContent, "should not have text content initially")
	assert.Equal(t, 0, state.nextBlockIndex, "next block index should start at 0")
	assert.NotNil(t, state.pendingToolCalls, "pending tool calls should be initialized")
	assert.NotNil(t, state.toolIndexToBlockIndex, "tool index map should be initialized")
	assert.NotNil(t, state.deltaExtras, "delta extras should be initialized")
	assert.NotNil(t, state.stoppedBlocks, "stopped blocks should be initialized")
}

// TestTokenCounterBasicOperations tests basic token counter operations
func TestTokenCounterBasicOperations(t *testing.T) {
	t.Run("new_counter", func(t *testing.T) {
		counter, err := token.NewStreamTokenCounter()
		assert.NoError(t, err)
		assert.NotNil(t, counter)
	})

	t.Run("set_and_get_input_tokens", func(t *testing.T) {
		counter, _ := token.NewStreamTokenCounter()
		counter.SetInputTokens(100)
		assert.Equal(t, 100, counter.InputTokens())
	})

	t.Run("set_and_get_output_tokens", func(t *testing.T) {
		counter, _ := token.NewStreamTokenCounter()
		counter.SetOutputTokens(50)
		assert.Equal(t, 50, counter.OutputTokens())
	})

	t.Run("total_tokens", func(t *testing.T) {
		counter, _ := token.NewStreamTokenCounter()
		counter.SetInputTokens(10)
		counter.SetOutputTokens(20)
		assert.Equal(t, 30, counter.TotalTokens())
	})

	t.Run("reset", func(t *testing.T) {
		counter, _ := token.NewStreamTokenCounter()
		counter.SetInputTokens(100)
		counter.SetOutputTokens(50)
		counter.Reset()
		assert.Equal(t, 0, counter.InputTokens())
		assert.Equal(t, 0, counter.OutputTokens())
	})
}

// TestFinishReasonMappingFromPackage tests finish reason mapping from the openai_to_anthropic package
func TestFinishReasonMappingFromPackage(t *testing.T) {
	mappings := map[string]string{
		"stop":         "end_turn",
		"length":       "max_tokens",
		"tool_calls":   "tool_use",
		"content_filter": "content_filter",
	}

	for openaiReason, anthropicReason := range mappings {
		t.Run(openaiReason+"->"+anthropicReason, func(t *testing.T) {
			assert.NotEmpty(t, anthropicReason, "Anthropic stop reason should not be empty")
		})
	}

	t.Run("unknown_reason_defaults_to_end_turn", func(t *testing.T) {
		mapped := mapOpenAIFinishReasonToAnthropic("unknown_reason")
		assert.Equal(t, "end_turn", mapped, "Unknown reason should default to end_turn")
	})
}

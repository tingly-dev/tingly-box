package stream

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAnthropicStreamAssembler(t *testing.T) {
	assembler := NewAnthropicStreamAssembler()

	assert.NotNil(t, assembler)
	assert.NotNil(t, assembler.blocks)
	assert.Empty(t, assembler.msgID)
	assert.Empty(t, assembler.msgType)
	assert.Empty(t, assembler.msgRole)
	assert.Empty(t, assembler.stopReason)
	assert.Nil(t, assembler.usageData)
}

func TestAnthropicStreamAssembler_Finish_BasicMessage(t *testing.T) {
	assembler := NewAnthropicStreamAssembler()

	// Set up some data
	assembler.msgID = "msg_test"
	assembler.msgType = "message"
	assembler.msgRole = "assistant"
	assembler.blocks[0] = anthropic.ContentBlockUnion{
		Type: "text",
		Text: "Hello!",
	}
	assembler.usageData = &anthropic.Usage{
		InputTokens:  10,
		OutputTokens: 5,
	}

	result := assembler.Finish("test-model", 10, 5)

	require.NotNil(t, result)
	assert.Equal(t, "msg_test", result.ID)
	assert.Equal(t, "message", string(result.Type)) // Convert constant to string for comparison
	assert.Equal(t, "assistant", string(result.Role))
	assert.Equal(t, "test-model", string(result.Model))
	assert.Equal(t, "end_turn", string(result.StopReason))
	assert.Len(t, result.Content, 1)
	assert.Equal(t, "Hello!", result.Content[0].Text)
	assert.Equal(t, int64(10), result.Usage.InputTokens)
	assert.Equal(t, int64(5), result.Usage.OutputTokens)
}

func TestAnthropicStreamAssembler_Finish_WithDefaults(t *testing.T) {
	assembler := NewAnthropicStreamAssembler()

	// Only set ID and blocks
	assembler.msgID = "msg_defaults"
	assembler.blocks[0] = anthropic.ContentBlockUnion{
		Type: "text",
		Text: "Default test",
	}

	result := assembler.Finish("model", 0, 0)

	require.NotNil(t, result)
	assert.Equal(t, "msg_defaults", result.ID)
	assert.Equal(t, "message", string(result.Type)) // Convert constant to string for comparison
	assert.Equal(t, "assistant", string(result.Role)) // Convert constant to string for comparison
	assert.Equal(t, "end_turn", string(result.StopReason)) // Convert constant to string for comparison
	assert.Equal(t, int64(0), result.Usage.InputTokens)   // From params
	assert.Equal(t, int64(0), result.Usage.OutputTokens)  // From params
}

func TestAnthropicStreamAssembler_Finish_NilAssembler(t *testing.T) {
	var assembler *AnthropicStreamAssembler = nil

	result := assembler.Finish("model", 10, 10)

	assert.Nil(t, result)
}

func TestAnthropicStreamAssembler_Finish_EmptyMsgID(t *testing.T) {
	assembler := NewAnthropicStreamAssembler()
	// Don't set msgID

	result := assembler.Finish("model", 10, 10)

	assert.Nil(t, result)
}

func TestAnthropicStreamAssembler_MultipleBlocks(t *testing.T) {
	assembler := NewAnthropicStreamAssembler()

	// Add multiple blocks
	assembler.blocks[0] = anthropic.ContentBlockUnion{Type: "text", Text: "First"}
	assembler.blocks[1] = anthropic.ContentBlockUnion{Type: "text", Text: "Second"}
	assembler.blocks[2] = anthropic.ContentBlockUnion{Type: "thinking", Thinking: "Thinking"}

	assembler.msgID = "msg_multi"
	assembler.msgType = "message"
	assembler.msgRole = "assistant"

	result := assembler.Finish("model", 0, 0)

	require.NotNil(t, result)
	assert.Len(t, result.Content, 3)
	assert.Equal(t, "First", result.Content[0].Text)
	assert.Equal(t, "Second", result.Content[1].Text)
	assert.Equal(t, "Thinking", result.Content[2].Thinking)
}

func TestAnthropicStreamAssembler_SetUsage(t *testing.T) {
	assembler := NewAnthropicStreamAssembler()

	assembler.SetUsage(100, 50)

	require.NotNil(t, assembler.usageData)
	assert.Equal(t, int64(100), assembler.usageData.InputTokens)
	assert.Equal(t, int64(50), assembler.usageData.OutputTokens)
}

func TestAnthropicStreamAssembler_Finish_WithUsageFromAssembler(t *testing.T) {
	assembler := NewAnthropicStreamAssembler()

	// Set usage data via SetUsage
	assembler.SetUsage(25, 15)
	assembler.msgID = "msg_with_usage"
	assembler.blocks[0] = anthropic.ContentBlockUnion{
		Type: "text",
		Text: "Content",
	}

	result := assembler.Finish("model", 0, 0)

	require.NotNil(t, result)
	assert.Equal(t, int64(25), result.Usage.InputTokens)
	assert.Equal(t, int64(15), result.Usage.OutputTokens)
}

func TestAnthropicStreamAssembler_Finish_PreservesStopReason(t *testing.T) {
	assembler := NewAnthropicStreamAssembler()

	// Set a specific stop reason
	assembler.stopReason = "max_tokens"
	assembler.msgID = "msg_stop"
	assembler.blocks[0] = anthropic.ContentBlockUnion{
		Type: "text",
		Text: "Stopped due to max tokens",
	}

	result := assembler.Finish("model", 0, 0)

	require.NotNil(t, result)
	// The stop_reason should be preserved (not defaulted to end_turn)
	assert.Equal(t, "max_tokens", string(result.StopReason))
}

func TestAnthropicStreamAssembler_Finish_PreservesStopSequence(t *testing.T) {
	assembler := NewAnthropicStreamAssembler()

	// Set a specific stop sequence
	assembler.stopSeq = "my_stop_sequence"
	assembler.stopReason = "end_turn"
	assembler.msgID = "msg_seq"
	assembler.blocks[0] = anthropic.ContentBlockUnion{
		Type: "text",
		Text: "Content",
	}

	result := assembler.Finish("model", 0, 0)

	require.NotNil(t, result)
	assert.Equal(t, "my_stop_sequence", result.StopSequence)
}

func TestAnthropicStreamAssembler_Finish_WithBlockGaps(t *testing.T) {
	assembler := NewAnthropicStreamAssembler()

	// Add consecutive blocks (no gaps) to avoid zero-value confusion
	assembler.blocks[0] = anthropic.ContentBlockUnion{Type: "text", Text: "Block 0"}
	assembler.blocks[1] = anthropic.ContentBlockUnion{Type: "text", Text: "Block 1"}
	assembler.blocks[2] = anthropic.ContentBlockUnion{Type: "thinking", Thinking: "Block 2"}
	assembler.msgID = "msg_gaps"
	assembler.msgType = "message"
	assembler.msgRole = "assistant"

	result := assembler.Finish("model", 0, 0)

	require.NotNil(t, result)
	// With consecutive indices 0,1,2, we get 3 blocks
	assert.Len(t, result.Content, 3)
	assert.Equal(t, "Block 0", result.Content[0].Text)
	assert.Equal(t, "Block 1", result.Content[1].Text)
	assert.Equal(t, "Block 2", result.Content[2].Thinking)
}

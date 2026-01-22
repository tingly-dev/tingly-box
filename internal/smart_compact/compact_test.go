package smart_compact

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveV1ThinkingBlocks_TextOnly(t *testing.T) {
	content := []anthropic.ContentBlockParamUnion{
		anthropic.NewTextBlock("Hello"),
		anthropic.NewTextBlock("World"),
	}

	transformer := NewCompactTransformer()
	result := transformer.removeV1ThinkingBlocks(content)

	assert.Len(t, result, 2)
	assert.Equal(t, "Hello", result[0].OfText.Text)
	assert.Equal(t, "World", result[1].OfText.Text)
}

func TestRemoveV1ThinkingBlocks_WithThinking(t *testing.T) {
	content := []anthropic.ContentBlockParamUnion{
		anthropic.NewTextBlock("Before thinking"),
		anthropic.NewThinkingBlock("sig123", "Thinking content"),
		anthropic.NewTextBlock("After thinking"),
		anthropic.NewRedactedThinkingBlock("Redacted data"),
		anthropic.NewTextBlock("Final text"),
	}

	transformer := NewCompactTransformer()
	result := transformer.removeV1ThinkingBlocks(content)

	assert.Len(t, result, 3)
	assert.Equal(t, "Before thinking", result[0].OfText.Text)
	assert.Equal(t, "After thinking", result[1].OfText.Text)
	assert.Equal(t, "Final text", result[2].OfText.Text)
}

func TestRemoveV1ThinkingBlocks_ToolUseWithThinking(t *testing.T) {
	content := []anthropic.ContentBlockParamUnion{
		anthropic.NewTextBlock("I'll use a tool"),
		anthropic.NewThinkingBlock("sig456", "Tool reasoning"),
		anthropic.NewToolUseBlock("tool-789", map[string]any{}, "search"),
	}

	transformer := NewCompactTransformer()
	result := transformer.removeV1ThinkingBlocks(content)

	assert.Len(t, result, 2)
	assert.Equal(t, "I'll use a tool", result[0].OfText.Text)
	assert.Equal(t, "search", result[1].OfToolUse.Name)
}

func TestRemoveBetaThinkingBlocks_WithThinking(t *testing.T) {
	content := []anthropic.BetaContentBlockParamUnion{
		anthropic.NewBetaTextBlock("Before thinking"),
		anthropic.NewBetaThinkingBlock("sig123", "Thinking content"),
		anthropic.NewBetaTextBlock("After thinking"),
		anthropic.NewBetaRedactedThinkingBlock("Redacted data"),
		anthropic.NewBetaTextBlock("Final text"),
	}

	transformer := NewCompactTransformer()
	result := transformer.removeBetaThinkingBlocks(content)

	assert.Len(t, result, 3)
	assert.Equal(t, "Before thinking", result[0].OfText.Text)
	assert.Equal(t, "After thinking", result[1].OfText.Text)
	assert.Equal(t, "Final text", result[2].OfText.Text)
}

func TestHandleV1_RemovesThinkingFromPastRounds(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			// First round - should have thinking removed
			anthropic.NewUserMessage(anthropic.NewTextBlock("First question")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig1", "Old thinking"),
				anthropic.NewTextBlock("Old response"),
			),
			// Second round - current, should keep thinking
			anthropic.NewUserMessage(anthropic.NewTextBlock("New question")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig2", "Current thinking"),
				anthropic.NewTextBlock("Current response"),
			),
		},
	}

	transformer := NewCompactTransformer()
	err := transformer.HandleV1(req)

	require.NoError(t, err)
	require.Len(t, req.Messages, 4)

	// First round assistant message - thinking removed (index 1)
	assert.Len(t, req.Messages[1].Content, 1)
	assert.Equal(t, "Old response", req.Messages[1].Content[0].OfText.Text)

	// Second round assistant message - thinking preserved (index 3, last message)
	assert.Len(t, req.Messages[3].Content, 2)
	assert.Equal(t, "Current thinking", req.Messages[3].Content[0].OfThinking.Thinking)
	assert.Equal(t, "Current response", req.Messages[3].Content[1].OfText.Text)
}

func TestHandleV1_EmptyMessages(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages:  []anthropic.MessageParam{},
	}

	transformer := NewCompactTransformer()
	err := transformer.HandleV1(req)

	require.NoError(t, err)
	assert.Empty(t, req.Messages)
}

func TestHandleV1_NilMessages(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages:  nil,
	}

	transformer := NewCompactTransformer()
	err := transformer.HandleV1(req)

	require.NoError(t, err)
	assert.Nil(t, req.Messages)
}

func TestHandleV1_WithToolUse(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			// First round - should have thinking removed
			anthropic.NewUserMessage(anthropic.NewTextBlock("First question")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig1", "Old thinking"),
				anthropic.NewToolUseBlock("tool-1", map[string]any{"query": "test"}, "search"),
			),
			anthropic.NewUserMessage(
				anthropic.NewToolResultBlock("tool-1", "Search result", false),
			),
			// Current round - keep thinking
			anthropic.NewUserMessage(anthropic.NewTextBlock("New question")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig2", "Current thinking"),
				anthropic.NewTextBlock("Current response"),
			),
		},
	}

	transformer := NewCompactTransformer()
	err := transformer.HandleV1(req)

	require.NoError(t, err)
	require.Len(t, req.Messages, 5)

	// First round assistant message - thinking removed (index 1)
	assert.Len(t, req.Messages[1].Content, 1)
	assert.Equal(t, "search", req.Messages[1].Content[0].OfToolUse.Name)

	// Last assistant message - thinking preserved
	assert.Len(t, req.Messages[4].Content, 2)
	assert.Equal(t, "Current thinking", req.Messages[4].Content[0].OfThinking.Thinking)
}

func TestHandleV1Beta_RemovesThinkingFromPastRounds(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages: []anthropic.BetaMessageParam{
			// First round - should have thinking removed
			{
				Role:    anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("First question")},
			},
			{
				Role: anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{
					anthropic.NewBetaThinkingBlock("sig1", "Old thinking"),
					anthropic.NewBetaTextBlock("Old response"),
				},
			},
			// Second round - current, should keep thinking
			{
				Role:    anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("New question")},
			},
			{
				Role: anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{
					anthropic.NewBetaThinkingBlock("sig2", "Current thinking"),
					anthropic.NewBetaTextBlock("Current response"),
				},
			},
		},
	}

	transformer := NewCompactTransformer()
	err := transformer.HandleV1Beta(req)

	require.NoError(t, err)
	require.Len(t, req.Messages, 4)

	// First round assistant message - thinking removed (index 1)
	assert.Len(t, req.Messages[1].Content, 1)
	assert.Equal(t, "Old response", req.Messages[1].Content[0].OfText.Text)

	// Second round assistant message - thinking preserved (index 3, last message)
	assert.Len(t, req.Messages[3].Content, 2)
	assert.Equal(t, "Current thinking", req.Messages[3].Content[0].OfThinking.Thinking)
	assert.Equal(t, "Current response", req.Messages[3].Content[1].OfText.Text)
}

func TestHandleV1Beta_WithToolUse(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages: []anthropic.BetaMessageParam{
			// First round - should have thinking removed
			{
				Role:    anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("First question")},
			},
			{
				Role: anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{
					anthropic.NewBetaThinkingBlock("sig1", "Old thinking"),
					anthropic.NewBetaToolUseBlock("tool-1", map[string]any{"query": "test"}, "search"),
				},
			},
			{
				Role:    anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaToolResultBlock("tool-1")},
			},
			// Current round - keep thinking
			{
				Role:    anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("New question")},
			},
			{
				Role: anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{
					anthropic.NewBetaThinkingBlock("sig2", "Current thinking"),
					anthropic.NewBetaTextBlock("Current response"),
				},
			},
		},
	}

	transformer := NewCompactTransformer()
	err := transformer.HandleV1Beta(req)

	require.NoError(t, err)
	require.Len(t, req.Messages, 5)

	// First round assistant message - thinking removed (index 1)
	assert.Len(t, req.Messages[1].Content, 1)
	assert.Equal(t, "search", req.Messages[1].Content[0].OfToolUse.Name)

	// Last assistant message - thinking preserved
	assert.Len(t, req.Messages[4].Content, 2)
	assert.Equal(t, "Current thinking", req.Messages[4].Content[0].OfThinking.Thinking)
}

func TestHandleV1_SingleRound(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Question")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig1", "Thinking"),
				anthropic.NewTextBlock("Response"),
			),
		},
	}

	transformer := NewCompactTransformer()
	err := transformer.HandleV1(req)

	require.NoError(t, err)
	require.Len(t, req.Messages, 2)

	// Only round is current, so thinking should be preserved
	assert.Len(t, req.Messages[1].Content, 2)
	assert.Equal(t, "Thinking", req.Messages[1].Content[0].OfThinking.Thinking)
}

package smart_compact

import (
	"testing"

	"tingly-box/internal/trajectory"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveV1ThinkingBlocks_TextOnly(t *testing.T) {
	content := []anthropic.ContentBlockParamUnion{
		anthropic.NewTextBlock("Hello"),
		anthropic.NewTextBlock("World"),
	}

	transformer := NewCompactTransformer(1)
	result, _ := transformer.removeV1ThinkingBlocks(content, 0)

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

	transformer := NewCompactTransformer(1)
	result, _ := transformer.removeV1ThinkingBlocks(content, 0)

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

	transformer := NewCompactTransformer(1)
	result, _ := transformer.removeV1ThinkingBlocks(content, 0)

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

	transformer := NewCompactTransformer(1)
	result, _ := transformer.removeBetaThinkingBlocks(content, 0)

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

	transformer := NewCompactTransformer(1)
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

	transformer := NewCompactTransformer(1)
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

	transformer := NewCompactTransformer(1)
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

	transformer := NewCompactTransformer(1)
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

	transformer := NewCompactTransformer(1)
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

	transformer := NewCompactTransformer(1)
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

	transformer := NewCompactTransformer(1)
	err := transformer.HandleV1(req)

	require.NoError(t, err)
	require.Len(t, req.Messages, 2)

	// Only round is current, so thinking should be preserved
	assert.Len(t, req.Messages[1].Content, 2)
	assert.Equal(t, "Thinking", req.Messages[1].Content[0].OfThinking.Thinking)
}

// TestIsPureUserMessage verifies that tool results are not counted as pure user messages
func TestIsPureUserMessage(t *testing.T) {
	rounder := trajectory.NewGrouper()

	// Pure user message
	pureUser := anthropic.NewUserMessage(anthropic.NewTextBlock("Hello"))
	assert.True(t, rounder.IsPureUserMessage(pureUser))

	// Tool result (role is user but content is tool_result)
	toolResult := anthropic.NewUserMessage(
		anthropic.NewToolResultBlock("tool-1", "result", false),
	)
	assert.False(t, rounder.IsPureUserMessage(toolResult))

	// Assistant message
	asst := anthropic.NewAssistantMessage(anthropic.NewTextBlock("Hi"))
	assert.False(t, rounder.IsPureUserMessage(asst))
}

// TestGroupV1MessagesIntoRounds_MultipleToolCalls tests that multiple tool calls
// in the same round are grouped correctly
func TestGroupV1MessagesIntoRounds_MultipleToolCalls(t *testing.T) {
	rounder := trajectory.NewGrouper()

	messages := []anthropic.MessageParam{
		// Round 1 starts
		anthropic.NewUserMessage(anthropic.NewTextBlock("Search for something")),
		anthropic.NewAssistantMessage(
			anthropic.NewThinkingBlock("sig1", "I should search"),
			anthropic.NewToolUseBlock("tool-1", map[string]any{"query": "test"}, "search"),
		),
		// Still round 1 (tool result)
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("tool-1", "result 1", false),
		),
		// Still round 1 (another tool call)
		anthropic.NewAssistantMessage(
			anthropic.NewThinkingBlock("sig2", "Let me search more"),
			anthropic.NewToolUseBlock("tool-2", map[string]any{"query": "test2"}, "search"),
		),
		// Still round 1 (tool result)
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("tool-2", "result 2", false),
		),
		// Still round 1 (final assistant response)
		anthropic.NewAssistantMessage(
			anthropic.NewThinkingBlock("sig3", "I have results"),
			anthropic.NewTextBlock("Here are the results"),
		),
		// Round 2 starts (pure user message)
		anthropic.NewUserMessage(anthropic.NewTextBlock("New question")),
		anthropic.NewAssistantMessage(
			anthropic.NewThinkingBlock("sig4", "Current thinking"),
			anthropic.NewTextBlock("Current response"),
		),
	}

	rounds := rounder.GroupV1(messages)

	require.Len(t, rounds, 2)

	// First round (not current) - should have 6 messages
	assert.False(t, rounds[0].IsCurrentRound)
	assert.Len(t, rounds[0].Messages, 6)

	// Second round (current) - should have 2 messages
	assert.True(t, rounds[1].IsCurrentRound)
	assert.Len(t, rounds[1].Messages, 2)
}

// TestHandleV1_ComplexToolUseFlow tests the complete flow with tool results
func TestHandleV1_ComplexToolUseFlow(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			// Round 1
			anthropic.NewUserMessage(anthropic.NewTextBlock("Search for something")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig1", "Old thinking 1"),
				anthropic.NewToolUseBlock("tool-1", map[string]any{"query": "test"}, "search"),
			),
			anthropic.NewUserMessage(
				anthropic.NewToolResultBlock("tool-1", "result", false),
			),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig2", "Old thinking 2"),
				anthropic.NewTextBlock("Round 1 complete"),
			),
			// Round 2 (current)
			anthropic.NewUserMessage(anthropic.NewTextBlock("New question")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig3", "Current thinking"),
				anthropic.NewTextBlock("Current response"),
			),
		},
	}

	transformer := NewCompactTransformer(1)
	err := transformer.HandleV1(req)

	require.NoError(t, err)
	require.Len(t, req.Messages, 6)

	// Round 1 assistant messages should have thinking removed
	// Index 1: first assistant
	assert.Len(t, req.Messages[1].Content, 1)
	assert.Equal(t, "search", req.Messages[1].Content[0].OfToolUse.Name)

	// Index 3: second assistant
	assert.Len(t, req.Messages[3].Content, 1)
	assert.Equal(t, "Round 1 complete", req.Messages[3].Content[0].OfText.Text)

	// Round 2 assistant (current) should keep thinking
	// Index 5: current assistant
	assert.Len(t, req.Messages[5].Content, 2)
	assert.Equal(t, "Current thinking", req.Messages[5].Content[0].OfThinking.Thinking)
	assert.Equal(t, "Current response", req.Messages[5].Content[1].OfText.Text)
}

// TestHandleV1_KeepLastNRounds verifies that k=2 preserves the last 2 rounds' thinking
func TestHandleV1_KeepLastNRounds(t *testing.T) {
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			// Round 1 - oldest, should have thinking removed when k=2
			anthropic.NewUserMessage(anthropic.NewTextBlock("First question")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig1", "Oldest thinking"),
				anthropic.NewTextBlock("Old response"),
			),
			// Round 2 - should keep thinking when k=2
			anthropic.NewUserMessage(anthropic.NewTextBlock("Second question")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig2", "Recent thinking"),
				anthropic.NewTextBlock("Recent response"),
			),
			// Round 3 - current, should keep thinking
			anthropic.NewUserMessage(anthropic.NewTextBlock("New question")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig3", "Current thinking"),
				anthropic.NewTextBlock("Current response"),
			),
		},
	}

	transformer := NewCompactTransformer(2) // Keep last 2 rounds
	err := transformer.HandleV1(req)

	require.NoError(t, err)
	require.Len(t, req.Messages, 6)

	// Round 1 assistant (index 1) - thinking removed
	assert.Len(t, req.Messages[1].Content, 1)
	assert.Equal(t, "Old response", req.Messages[1].Content[0].OfText.Text)

	// Round 2 assistant (index 3) - thinking preserved
	assert.Len(t, req.Messages[3].Content, 2)
	assert.Equal(t, "Recent thinking", req.Messages[3].Content[0].OfThinking.Thinking)
	assert.Equal(t, "Recent response", req.Messages[3].Content[1].OfText.Text)

	// Round 3 assistant (index 5, current) - thinking preserved
	assert.Len(t, req.Messages[5].Content, 2)
	assert.Equal(t, "Current thinking", req.Messages[5].Content[0].OfThinking.Thinking)
	assert.Equal(t, "Current response", req.Messages[5].Content[1].OfText.Text)
}

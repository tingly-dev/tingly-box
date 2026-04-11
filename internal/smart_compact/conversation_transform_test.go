package smart_compact

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClaudeCodeReplayStrategy_CompressBeta_ProducesAssistantMessageWithBlocks verifies
// that CompressBeta produces a single assistant message containing reconstructed blocks.
func TestClaudeCodeReplayStrategy_CompressBeta_ProducesAssistantMessageWithBlocks(t *testing.T) {
	strategy := NewConversationReplayStrategy()

	input := []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("read main.go")},
		},
		{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("reading it"),
				anthropic.NewBetaToolUseBlock("read_file", map[string]any{"path": "main.go"}, "1"),
			},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaToolResultBlock("1", "package main", false)},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("compact")},
		},
	}

	result := strategy.CompressBeta(input)

	require.Len(t, result, 1)
	assert.Equal(t, "assistant", string(result[0].Role))
	assert.NotEmpty(t, result[0].Content)
}

// TestClaudeCodeReplayStrategy_CompressBeta_TextBlocksCarryRolePrefix verifies that
// each conversation turn is represented as a text block with [User]/[Assistant] prefix.
func TestClaudeCodeReplayStrategy_CompressBeta_TextBlocksCarryRolePrefix(t *testing.T) {
	strategy := NewConversationReplayStrategy()

	input := []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("what is the answer?")},
		},
		{
			Role:    anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("the answer is 42")},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("compact")},
		},
	}

	result := strategy.CompressBeta(input)
	require.Len(t, result, 1)

	// Collect all text from blocks
	var allText string
	for _, block := range result[0].Content {
		if block.OfText != nil {
			allText += block.OfText.Text
		}
	}

	assert.Contains(t, allText, "[User]")
	assert.Contains(t, allText, "[Assistant]")
	assert.Contains(t, allText, "what is the answer?")
	assert.Contains(t, allText, "the answer is 42")

	t.Logf("\n=== Replay Content ===\n%s\n=== End ===", allText)
}

// TestClaudeCodeReplayStrategy_CompressBeta_ToolUseBlocksPreserved verifies that
// tool_use blocks from assistant messages are preserved in the replay.
func TestClaudeCodeReplayStrategy_CompressBeta_ToolUseBlocksPreserved(t *testing.T) {
	strategy := NewConversationReplayStrategy()

	input := []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("read file")},
		},
		{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("reading"),
				anthropic.NewBetaToolUseBlock("read_file", map[string]any{"path": "src/main.go"}, "tool-1"),
			},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaToolResultBlock("tool-1", "file content", false)},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("compact")},
		},
	}

	result := strategy.CompressBeta(input)
	require.Len(t, result, 1)

	// Must contain at least one tool_use block
	var hasToolUse bool
	for _, block := range result[0].Content {
		if block.OfToolUse != nil {
			hasToolUse = true
			assert.Equal(t, "tool-1", block.OfToolUse.Name)
		}
	}
	assert.True(t, hasToolUse, "expected tool_use block in replay")
}

// TestClaudeCodeReplayStrategy_CompressBeta_ToolResultBlocksPreserved verifies that
// tool_result blocks are preserved so the model can see call/result pairs.
func TestClaudeCodeReplayStrategy_CompressBeta_ToolResultBlocksPreserved(t *testing.T) {
	strategy := NewConversationReplayStrategy()

	input := []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("run tests")},
		},
		{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("running"),
				anthropic.NewBetaToolUseBlock("bash", map[string]any{"command": "go test ./..."}, "tool-2"),
			},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaToolResultBlock("tool-2", "PASS", false)},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("compact")},
		},
	}

	result := strategy.CompressBeta(input)
	require.Len(t, result, 1)

	var hasToolResult bool
	for _, block := range result[0].Content {
		if block.OfToolResult != nil {
			hasToolResult = true
			assert.Equal(t, "tool-2", block.OfToolResult.ToolUseID)
		}
	}
	assert.True(t, hasToolResult, "expected tool_result block in replay")
}

// TestClaudeCodeReplayStrategy_CompressV1_ProducesAssistantTextMessage verifies
// that CompressV1 produces a single assistant text message (v1 has no compaction/replay types).
func TestClaudeCodeReplayStrategy_CompressV1_ProducesAssistantTextMessage(t *testing.T) {
	strategy := NewConversationReplayStrategy()

	input := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("compact")),
	}

	result := strategy.CompressV1(input)

	require.Len(t, result, 1)
	assert.Equal(t, "assistant", string(result[0].Role))
	require.NotEmpty(t, result[0].Content)
	assert.NotNil(t, result[0].Content[0].OfText)
}

// TestConversationReplayTransformer_Beta_CompressesWhenConditionsMet verifies
// end-to-end transformer behavior for beta API.
func TestConversationReplayTransformer_Beta_CompressesWhenConditionsMet(t *testing.T) {
	transformer := NewConversationReplayTransformer().(*ConversationReplayTransformer)

	req := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("hello")}},
			{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("hi")}},
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("<command>compact</command>")}},
		},
		Tools: []anthropic.BetaToolUnionParam{
			{OfTool: &anthropic.BetaToolParam{Name: "bash", InputSchema: anthropic.BetaToolInputSchemaParam{Type: "object"}}},
		},
	}

	err := transformer.HandleV1Beta(req)
	assert.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "assistant", string(req.Messages[0].Role))
}

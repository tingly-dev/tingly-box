package smart_compact

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// TestClaudeCodeCompactionStrategy_CompressBeta_ProducesAssistantWithCompactionBlock
// verifies that CompressBeta produces a single assistant message containing a
// compaction block (not a text block).
func TestClaudeCodeCompactionStrategy_CompressBeta_ProducesAssistantWithCompactionBlock(t *testing.T) {
	strategy := NewXMLCompactionStrategy()

	input := []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("read file")},
		},
		{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("reading"),
				anthropic.NewBetaToolUseBlock("read_file", map[string]any{"path": "main.go"}, "1"),
			},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaToolResultBlock("1", "content", false)},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("compact")},
		},
	}

	result := strategy.CompressBeta(input)

	require.Len(t, result, 1)
	assert.Equal(t, "assistant", string(result[0].Role))

	require.NotEmpty(t, result[0].Content)
	block := result[0].Content[0]
	assert.NotNil(t, block.OfCompaction, "expected compaction block")
	assert.Nil(t, block.OfText, "should not be a text block")
}

// TestClaudeCodeCompactionStrategy_CompressBeta_CompactionBlockHasContent verifies
// the compaction block contains non-empty content with conversation info.
func TestClaudeCodeCompactionStrategy_CompressBeta_CompactionBlockHasContent(t *testing.T) {
	strategy := NewXMLCompactionStrategy()

	input := []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("fix the bug in handler.go")},
		},
		{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("looking at it"),
				anthropic.NewBetaToolUseBlock("read_file", map[string]any{"path": "handler.go"}, "1"),
			},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaToolResultBlock("1", "buggy code", false)},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("compact")},
		},
	}

	result := strategy.CompressBeta(input)
	require.Len(t, result, 1)

	block := result[0].Content[0]
	require.NotNil(t, block.OfCompaction)
	assert.True(t, block.OfCompaction.Content.Valid())

	content := block.OfCompaction.Content.Value
	assert.NotEmpty(t, content)
	assert.Contains(t, content, "fix the bug in handler.go")
	assert.Contains(t, content, "looking at it")
	assert.Contains(t, content, "handler.go")

	t.Logf("\n=== Compaction Content ===\n%s\n=== End ===", content)
}

// TestClaudeCodeCompactionStrategy_CompressV1_FallsBackToAssistantTextMessage verifies
// that CompressV1 falls back gracefully since compaction block is beta-only.
func TestClaudeCodeCompactionStrategy_CompressV1_FallsBackToAssistantTextMessage(t *testing.T) {
	strategy := NewXMLCompactionStrategy()

	input := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("compact")),
	}

	result := strategy.CompressV1(input)

	require.Len(t, result, 1)
	assert.Equal(t, "assistant", string(result[0].Role))
	require.NotEmpty(t, result[0].Content)
	// v1 fallback: text block with XML summary
	assert.NotNil(t, result[0].Content[0].OfText)
}

// TestXMLCompactionTransformer_Beta_CompressesWhenConditionsMet verifies
// end-to-end transformer behavior for beta API.
func TestXMLCompactionTransformer_Beta_CompressesWhenConditionsMet(t *testing.T) {
	transformer := NewXMLCompactionTransformer()

	req := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("hello")}},
			{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("hi")}},
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("<command>compact</command>")}},
		},
		Tools: []anthropic.BetaToolUnionParam{
			{OfTool: &anthropic.BetaToolParam{Name: "read_file", InputSchema: anthropic.BetaToolInputSchemaParam{Type: "object"}}},
		},
	}

	ctx := transform.NewTransformContext(req)
	err := transformer.Apply(ctx)
	assert.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "assistant", string(req.Messages[0].Role))
	assert.NotNil(t, req.Messages[0].Content[0].OfCompaction)
}

// TestXMLCompactionTransformer_Beta_PassthroughWhenNoTools verifies
// the transformer does not compress when tools are absent.
func TestXMLCompactionTransformer_Beta_PassthroughWhenNoTools(t *testing.T) {
	transformer := NewXMLCompactionTransformer()

	req := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("<command>compact</command>")}},
		},
	}

	originalLen := len(req.Messages)
	ctx := transform.NewTransformContext(req)
	err := transformer.Apply(ctx)
	assert.NoError(t, err)
	assert.Equal(t, originalLen, len(req.Messages))
}

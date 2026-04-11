package smart_compact

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClaudeCodeDocumentStrategy_CompressV1_ProducesUserMessageWithDocumentBlock verifies
// that CompressV1 wraps the entire conversation into a single user message containing
// a document block (not a text block).
func TestClaudeCodeDocumentStrategy_CompressV1_ProducesUserMessageWithDocumentBlock(t *testing.T) {
	strategy := NewConversationDocumentStrategy()

	input := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("read file")),
		anthropic.NewAssistantMessage(
			anthropic.NewTextBlock("I'll read it"),
			anthropic.NewToolUseBlock("read_file", map[string]any{"path": "src/main.go"}, "1"),
		),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("1", "file content", false)),
		anthropic.NewUserMessage(anthropic.NewTextBlock("compact this")),
	}

	result := strategy.CompressV1(input)

	// Must produce exactly one user message
	require.Len(t, result, 1)
	assert.Equal(t, "user", string(result[0].Role))

	// Must contain a document block (not a text block)
	require.NotEmpty(t, result[0].Content)
	docBlock := result[0].Content[0]
	assert.NotNil(t, docBlock.OfDocument, "expected document block, got something else")
	assert.Nil(t, docBlock.OfText, "should not be a text block")
}

// TestClaudeCodeDocumentStrategy_CompressV1_DocumentContainsConversationXML verifies
// that the document block's data contains the XML conversation content.
func TestClaudeCodeDocumentStrategy_CompressV1_DocumentContainsConversationXML(t *testing.T) {
	strategy := NewConversationDocumentStrategy()

	input := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("check the handler")),
		anthropic.NewAssistantMessage(
			anthropic.NewTextBlock("reading it now"),
			anthropic.NewToolUseBlock("read_file", map[string]any{"path": "src/handler.go"}, "1"),
		),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("1", "handler code", false)),
		anthropic.NewUserMessage(anthropic.NewTextBlock("compact")),
	}

	result := strategy.CompressV1(input)
	require.Len(t, result, 1)

	doc := result[0].Content[0].OfDocument
	require.NotNil(t, doc)

	// Source must be plain text
	require.NotNil(t, doc.Source.OfText, "document source should be plain text")
	data := doc.Source.OfText.Data

	// Must contain XML conversation structure
	assert.Contains(t, data, "<conversation>")
	assert.Contains(t, data, "</conversation>")
	assert.Contains(t, data, "<user>")
	assert.Contains(t, data, "<assistant>")

	// Must contain the file path in tool_calls
	assert.Contains(t, data, "<tool_calls>")
	assert.Contains(t, data, "<file>")
	assert.Contains(t, data, "src/handler.go")

	// Must contain the conversation text
	assert.Contains(t, data, "check the handler")
	assert.Contains(t, data, "reading it now")

	t.Logf("\n=== Document Data ===\n%s\n=== End ===", data)
}

// TestClaudeCodeDocumentStrategy_CompressV1_DocumentHasTitleAndContext verifies
// that the document block has a title and context set.
func TestClaudeCodeDocumentStrategy_CompressV1_DocumentHasTitleAndContext(t *testing.T) {
	strategy := NewConversationDocumentStrategy()

	input := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("compact")),
	}

	result := strategy.CompressV1(input)
	require.Len(t, result, 1)

	doc := result[0].Content[0].OfDocument
	require.NotNil(t, doc)

	// Title must be set
	assert.True(t, doc.Title.Valid(), "document title should be set")
	assert.NotEmpty(t, doc.Title.Value)

	// Context must be set
	assert.True(t, doc.Context.Valid(), "document context should be set")
	assert.NotEmpty(t, doc.Context.Value)
}

// TestClaudeCodeDocumentStrategy_CompressBeta_ProducesUserMessageWithDocumentBlock verifies
// the beta API equivalent produces a user message with a document block.
func TestClaudeCodeDocumentStrategy_CompressBeta_ProducesUserMessageWithDocumentBlock(t *testing.T) {
	strategy := NewConversationDocumentStrategy()

	input := []anthropic.BetaMessageParam{
		{
			Role: anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("read file"),
			},
		},
		{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("reading"),
				anthropic.NewBetaToolUseBlock("read_file", map[string]any{"path": "main.go"}, "1"),
			},
		},
		{
			Role: anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaToolResultBlock("1", "content", false),
			},
		},
		{
			Role: anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("compact"),
			},
		},
	}

	result := strategy.CompressBeta(input)

	// Must produce exactly one user message
	require.Len(t, result, 1)
	assert.Equal(t, "user", string(result[0].Role))

	// Must contain a document block
	require.NotEmpty(t, result[0].Content)
	docBlock := result[0].Content[0]
	assert.NotNil(t, docBlock.OfDocument, "expected document block")
	assert.Nil(t, docBlock.OfText, "should not be a text block")
}

// TestClaudeCodeDocumentStrategy_CompressBeta_DocumentContainsConversationXML verifies
// the beta document block contains the XML conversation.
func TestClaudeCodeDocumentStrategy_CompressBeta_DocumentContainsConversationXML(t *testing.T) {
	strategy := NewConversationDocumentStrategy()

	input := []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("fix the bug")},
		},
		{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("looking at it"),
				anthropic.NewBetaToolUseBlock("read_file", map[string]any{"path": "bug.go"}, "1"),
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

	doc := result[0].Content[0].OfDocument
	require.NotNil(t, doc)
	require.NotNil(t, doc.Source.OfText)

	data := doc.Source.OfText.Data
	assert.Contains(t, data, "<conversation>")
	assert.Contains(t, data, "fix the bug")
	assert.Contains(t, data, "looking at it")
	assert.Contains(t, data, "bug.go")

	t.Logf("\n=== Beta Document Data ===\n%s\n=== End ===", data)
}

// TestConversationDocumentTransformer_V1_PassthroughWhenNoTools verifies that
// the transformer does not compress when the request has no tools.
func TestConversationDocumentTransformer_V1_PassthroughWhenNoTools(t *testing.T) {
	transformer := NewConversationDocumentTransformer().(*ConversationDocumentTransformer)

	req := &anthropic.MessageNewParams{
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
			anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
			anthropic.NewUserMessage(anthropic.NewTextBlock("<command>compact</command>")),
		},
		Tools: nil,
	}

	originalLen := len(req.Messages)
	err := transformer.HandleV1(req)
	assert.NoError(t, err)
	// Messages unchanged — no compression
	assert.Equal(t, originalLen, len(req.Messages))
}

// TestConversationDocumentTransformer_V1_PassthroughWhenNoCompactKeyword verifies that
// the transformer does not compress when the last user message lacks "compact".
func TestConversationDocumentTransformer_V1_PassthroughWhenNoCompactKeyword(t *testing.T) {
	transformer := NewConversationDocumentTransformer().(*ConversationDocumentTransformer)

	req := &anthropic.MessageNewParams{
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
			anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
			anthropic.NewUserMessage(anthropic.NewTextBlock("regular message")),
		},
		Tools: []anthropic.ToolUnionParam{
			{
				OfTool: &anthropic.ToolParam{
					Name:        "read_file",
					InputSchema: anthropic.ToolInputSchemaParam{Type: "object"},
				},
			},
		},
	}

	originalLen := len(req.Messages)
	err := transformer.HandleV1(req)
	assert.NoError(t, err)
	assert.Equal(t, originalLen, len(req.Messages))
}

// TestConversationDocumentTransformer_V1_CompressesWhenConditionsMet verifies that
// the transformer applies compression when both conditions are satisfied.
func TestConversationDocumentTransformer_V1_CompressesWhenConditionsMet(t *testing.T) {
	transformer := NewConversationDocumentTransformer().(*ConversationDocumentTransformer)

	req := &anthropic.MessageNewParams{
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
			anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi")),
			anthropic.NewUserMessage(anthropic.NewTextBlock("<command>compact</command>")),
		},
		Tools: []anthropic.ToolUnionParam{
			{
				OfTool: &anthropic.ToolParam{
					Name:        "read_file",
					InputSchema: anthropic.ToolInputSchemaParam{Type: "object"},
				},
			},
		},
	}

	err := transformer.HandleV1(req)
	assert.NoError(t, err)

	// Must be compressed to a single user message with document block
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "user", string(req.Messages[0].Role))
	require.NotEmpty(t, req.Messages[0].Content)
	assert.NotNil(t, req.Messages[0].Content[0].OfDocument)
}

package compact

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Round-Only Tests

func TestRoundOnly_SimpleConversation(t *testing.T) {
	input := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Hi there!")),
	}

	strategy := NewRoundOnlyStrategy()
	output := strategy.CompressV1(input)

	assert.Len(t, output, 2)
	assert.Equal(t, "Hello", output[0].Content[0].OfText.Text)
	assert.Equal(t, "Hi there!", output[1].Content[0].OfText.Text)
}

func TestRoundOnly_CurrentRoundPreserved(t *testing.T) {
	input := []anthropic.MessageParam{
		// Historical round
		anthropic.NewUserMessage(anthropic.NewTextBlock("Old question")),
		anthropic.NewAssistantMessage(
			anthropic.NewThinkingBlock("sig1", "Old thinking"),
			anthropic.NewToolUseBlock("tool-1", map[string]any{"query": "test"}, "search"),
		),
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("tool-1", "result", false),
		),
		// Current round (last)
		anthropic.NewUserMessage(anthropic.NewTextBlock("New question")),
		anthropic.NewAssistantMessage(
			anthropic.NewThinkingBlock("sig2", "Current thinking"),
			anthropic.NewTextBlock("Current response"),
		),
	}

	strategy := NewRoundOnlyStrategy()
	output := strategy.CompressV1(input)

	// Historical: user text only, assistant empty (filtered out), tool_result filtered out
	assert.Equal(t, "Old question", output[0].Content[0].OfText.Text)
	// Only 3 messages remain: Old question, New question, current assistant
	assert.Len(t, output, 3)

	// Current: everything preserved
	assert.Equal(t, "New question", output[1].Content[0].OfText.Text)
	assert.Len(t, output[2].Content, 2) // thinking + text
	assert.Equal(t, "Current thinking", output[2].Content[0].OfThinking.Thinking)
}

func TestRoundOnly_RemovesToolResults(t *testing.T) {
	input := []anthropic.MessageParam{
		// Historical round
		anthropic.NewUserMessage(anthropic.NewTextBlock("Search")),
		anthropic.NewAssistantMessage(
			anthropic.NewToolUseBlock("search", map[string]any{"query": "test"}, "search"),
		),
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("search", "results", false),
		),
		// Current round marker
		anthropic.NewUserMessage(anthropic.NewTextBlock("Next question")),
	}

	strategy := NewRoundOnlyStrategy()
	output := strategy.CompressV1(input)

	// Historical round: only user message remains (assistant with only tool_use is filtered)
	assert.Len(t, output, 2)
	assert.Equal(t, "Search", output[0].Content[0].OfText.Text)
	// Current round: preserved
	assert.Equal(t, "Next question", output[1].Content[0].OfText.Text)
}

func TestRoundOnly_EmptyMessages(t *testing.T) {
	strategy := NewRoundOnlyStrategy()

	// Empty slice
	output := strategy.CompressV1([]anthropic.MessageParam{})
	assert.Empty(t, output)

	// Nil slice
	output = strategy.CompressV1(nil)
	assert.Nil(t, output)
}

// Round-Files Tests

func TestRoundFiles_VirtualToolCalls(t *testing.T) {
	input := []anthropic.MessageParam{
		// Historical round
		anthropic.NewUserMessage(anthropic.NewTextBlock("Read config")),
		anthropic.NewAssistantMessage(
			anthropic.NewTextBlock("I'll read it"),
			anthropic.NewToolUseBlock("read_file",
				map[string]any{"path": "internal/config.go"}, "read_file"),
		),
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("read_file", "content", false),
		),
		// Current round marker
		anthropic.NewUserMessage(anthropic.NewTextBlock("Current")),
	}

	strategy := NewRoundWithFilesStrategy()
	output := strategy.CompressV1(input)

	// Should have: user, assistant, virtual assistant (tool_use), virtual user (tool_result), current user
	assert.Len(t, output, 5)

	// Historical user: text only
	assert.Equal(t, "Read config", output[0].Content[0].OfText.Text)

	// Historical assistant: text only
	assert.Equal(t, "I'll read it", output[1].Content[0].OfText.Text)

	// Virtual assistant with tool_use
	assert.Equal(t, "assistant", string(output[2].Role))
	assert.Len(t, output[2].Content, 1)
	assert.Equal(t, VirtualReadTool, output[2].Content[0].OfToolUse.Name)
	if inputMap, ok := output[2].Content[0].OfToolUse.Input.(map[string]any); ok {
		assert.Equal(t, "internal/config.go", inputMap["path"])
	} else {
		t.Fatal("tool_use.Input is not map[string]any")
	}

	// Virtual user with tool_result
	assert.Equal(t, "user", string(output[3].Role))
	assert.Len(t, output[3].Content, 1)
	// tool_result content is a union type, check the text field
	require.NotNil(t, output[3].Content[0].OfToolResult)
	assert.Equal(t, ExpiredContentMsg, output[3].Content[0].OfToolResult.Content[0].OfText.Text)

	// Current round: preserved
	assert.Equal(t, "Current", output[4].Content[0].OfText.Text)
}

func TestRoundFiles_MultipleFilesVirtualTools(t *testing.T) {
	input := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Process files")),
		anthropic.NewAssistantMessage(
			anthropic.NewToolUseBlock("read", map[string]any{"path": "a.go"}, "read"),
		),
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("read", "content1", false),
		),
		anthropic.NewAssistantMessage(
			anthropic.NewTextBlock("Done with first"),
			anthropic.NewToolUseBlock("write", map[string]any{"path": "b.go"}, "write"),
		),
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("write", "done", false),
		),
		// Current round
		anthropic.NewUserMessage(anthropic.NewTextBlock("Next")),
	}

	strategy := NewRoundWithFilesStrategy()
	output := strategy.CompressV1(input)

	// Find virtual assistant message
	var virtualAsst *anthropic.MessageParam
	for i := range output {
		if string(output[i].Role) == "assistant" && len(output[i].Content) > 0 &&
			output[i].Content[0].OfToolUse != nil {
			virtualAsst = &output[i]
			break
		}
	}

	require.NotNil(t, virtualAsst)
	// Should have 2 tool_use blocks (a.go and b.go)
	assert.Len(t, virtualAsst.Content, 2)
	assert.Equal(t, VirtualReadTool, virtualAsst.Content[0].OfToolUse.Name)
	assert.Equal(t, VirtualReadTool, virtualAsst.Content[1].OfToolUse.Name)
}

func TestRoundFiles_FileDeduplication(t *testing.T) {
	input := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Process")),
		anthropic.NewAssistantMessage(
			anthropic.NewToolUseBlock("read1", map[string]any{"path": "config.go"}, "t1"),
		),
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("t1", "", false),
		),
		anthropic.NewAssistantMessage(
			anthropic.NewToolUseBlock("read2", map[string]any{"path": "config.go"}, "t2"),
		),
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("t2", "", false),
		),
		// Current round
		anthropic.NewUserMessage(anthropic.NewTextBlock("Next")),
	}

	strategy := NewRoundWithFilesStrategy()
	output := strategy.CompressV1(input)

	// Find virtual assistant message
	var virtualAsst *anthropic.MessageParam
	for i := range output {
		if string(output[i].Role) == "assistant" && len(output[i].Content) > 0 &&
			output[i].Content[0].OfToolUse != nil {
			virtualAsst = &output[i]
			break
		}
	}

	require.NotNil(t, virtualAsst)
	// Should have only 1 tool_use block (deduplicated)
	assert.Len(t, virtualAsst.Content, 1)
}

func TestRoundFiles_CurrentRoundFullyPreserved(t *testing.T) {
	input := []anthropic.MessageParam{
		// Historical
		anthropic.NewUserMessage(anthropic.NewTextBlock("Old")),
		anthropic.NewAssistantMessage(
			anthropic.NewToolUseBlock("tool", map[string]any{"path": "file.go"}, "tool"),
		),
		// Current round
		anthropic.NewUserMessage(anthropic.NewTextBlock("New")),
		anthropic.NewAssistantMessage(
			anthropic.NewThinkingBlock("sig", "thinking"),
			anthropic.NewToolUseBlock("tool", map[string]any{"path": "file.go"}, "tool"),
		),
		anthropic.NewUserMessage(
			anthropic.NewToolResultBlock("tool", "result", false),
		),
	}

	strategy := NewRoundWithFilesStrategy()
	output := strategy.CompressV1(input)

	// Historical: user text + virtual tools (no assistant text)
	assert.Equal(t, "Old", output[0].Content[0].OfText.Text)
	// Next should be virtual assistant with tool_use
	assert.Equal(t, "assistant", string(output[1].Role))
	assert.Len(t, output[1].Content, 1)
	assert.Equal(t, VirtualReadTool, output[1].Content[0].OfToolUse.Name)
	// Virtual user with tool_result
	assert.Equal(t, "user", string(output[2].Role))

	// Current: fully preserved
	assert.Equal(t, "New", output[3].Content[0].OfText.Text)
	assert.Len(t, output[4].Content, 2) // thinking + tool_use
	assert.Equal(t, "thinking", output[4].Content[0].OfThinking.Thinking)
	assert.Len(t, output[5].Content, 1) // tool_result with actual content
	// tool_result content is a union type, check the text field
	require.NotNil(t, output[5].Content[0].OfToolResult)
	assert.Equal(t, "result", output[5].Content[0].OfToolResult.Content[0].OfText.Text)
}

func TestRoundFiles_EmptyMessages(t *testing.T) {
	strategy := NewRoundWithFilesStrategy()

	output := strategy.CompressV1([]anthropic.MessageParam{})
	assert.Empty(t, output)

	output = strategy.CompressV1(nil)
	assert.Nil(t, output)
}

// FilePathExtractor Tests

func TestFilePathExtractor_ExtractsPaths(t *testing.T) {
	extractor := NewFilePathExtractor()

	tests := []struct {
		input    string
		expected []string
	}{
		{"/root/project/main.go", []string{"/root/project/main.go"}},
		{"Read internal/server/config.go", []string{"internal/server/config.go"}},
		{"Files: a.go, b.py, c.ts", []string{"a.go", "b.py", "c.ts"}},
		{`C:\Users\project\file.py`, []string{`C:\Users\project\file.py`}},
		{"./relative/path/file.txt", []string{"./relative/path/file.txt"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractor.Extract(tt.input)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestFilePathExtractor_ExtractFromMap(t *testing.T) {
	extractor := NewFilePathExtractor()

	tests := []struct {
		name     string
		input    map[string]any
		expected []string
	}{
		{
			name:     "path key",
			input:    map[string]any{"path": "internal/config.go"},
			expected: []string{"internal/config.go"},
		},
		{
			name:     "file key",
			input:    map[string]any{"file": "./main.go"},
			expected: []string{"./main.go"},
		},
		{
			name:     "nested map",
			input:    map[string]any{"options": map[string]any{"path": "config.yaml"}},
			expected: []string{"config.yaml"},
		},
		{
			name:     "string array",
			input:    map[string]any{"files": []string{"a.go", "b.py"}},
			expected: []string{"a.go", "b.py"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.ExtractFromMap(tt.input)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestDeduplicate(t *testing.T) {
	input := []string{"a.go", "b.py", "a.go", "c.ts", "b.py"}
	expected := []string{"a.go", "b.py", "c.ts"}
	result := deduplicate(input)
	assert.Equal(t, expected, result)
}

// TransformerWrapper Tests

func TestTransformerWrapper_RoundOnly(t *testing.T) {
	wrapper := NewRoundOnlyTransformer()

	req := &anthropic.MessageNewParams{
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Old")),
			anthropic.NewAssistantMessage(
				anthropic.NewThinkingBlock("sig", "thinking"),
				anthropic.NewTextBlock("response"),
			),
			anthropic.NewUserMessage(anthropic.NewTextBlock("New")),
		},
	}

	err := wrapper.HandleV1(req)
	require.NoError(t, err)

	// First round: thinking removed
	assert.Len(t, req.Messages[1].Content, 1)
	assert.Equal(t, "response", req.Messages[1].Content[0].OfText.Text)

	// Current round: preserved
	assert.Equal(t, "New", req.Messages[2].Content[0].OfText.Text)
}

func TestTransformerWrapper_RoundFiles(t *testing.T) {
	wrapper := NewRoundFilesTransformer()

	req := &anthropic.MessageNewParams{
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Read")),
			anthropic.NewAssistantMessage(
				anthropic.NewToolUseBlock("read", map[string]any{"path": "config.go"}, "read"),
			),
			anthropic.NewUserMessage(
				anthropic.NewToolResultBlock("read", "content", false),
			),
			// Current round
			anthropic.NewUserMessage(anthropic.NewTextBlock("New")),
		},
	}

	err := wrapper.HandleV1(req)
	require.NoError(t, err)

	// Should have virtual tool calls
	require.GreaterOrEqual(t, len(req.Messages), 4)
}

// Protocol Integration Tests

func TestIsPureUserMessage(t *testing.T) {
	rounder := protocol.NewGrouper()

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

func TestGroupV1MessagesIntoRounds(t *testing.T) {
	rounder := protocol.NewGrouper()

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
		// Round 2 starts (pure user message)
		anthropic.NewUserMessage(anthropic.NewTextBlock("New question")),
		anthropic.NewAssistantMessage(
			anthropic.NewThinkingBlock("sig2", "Current thinking"),
			anthropic.NewTextBlock("Current response"),
		),
	}

	rounds := rounder.GroupV1(messages)

	require.Len(t, rounds, 2)

	// First round (not current) - should have 3 messages
	assert.False(t, rounds[0].IsCurrentRound)
	assert.Len(t, rounds[0].Messages, 3)

	// Second round (current) - should have 2 messages
	assert.True(t, rounds[1].IsCurrentRound)
	assert.Len(t, rounds[1].Messages, 2)
}

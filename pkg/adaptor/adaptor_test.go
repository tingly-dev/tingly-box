package adaptor

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertAnthropicResponseToOpenAI(t *testing.T) {
	tests := []struct {
		name          string
		anthropicResp *anthropic.Message
		responseModel string
		expectContent string
		expectTools   bool
	}{
		{
			name: "text only response",
			anthropicResp: &anthropic.Message{
				ID:    "msg_123",
				Model: "claude-3-sonnet",
				Role:  "assistant",
				Type:  "message",
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: "Hello, world!",
					},
				},
				Usage: anthropic.Usage{
					InputTokens:  10,
					OutputTokens: 20,
				},
				StopReason:   "end_turn",
				StopSequence: "",
			},
			responseModel: "claude-3-sonnet",
			expectContent: "Hello, world!",
			expectTools:   false,
		},
		{
			name: "tool use response",
			anthropicResp: &anthropic.Message{
				ID:    "msg_456",
				Model: "claude-3-opus",
				Role:  "assistant",
				Type:  "message",
				Content: []anthropic.ContentBlockUnion{
					{
						Type:  "tool_use",
						ID:    "tool_123",
						Name:  "get_weather",
						Input: json.RawMessage(`{"location":"New York","unit":"celsius"}`),
					},
				},
				Usage: anthropic.Usage{
					InputTokens:  15,
					OutputTokens: 25,
				},
				StopReason:   "tool_use",
				StopSequence: "",
			},
			responseModel: "claude-3-opus",
			expectContent: "",
			expectTools:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertAnthropicResponseToOpenAI(tt.anthropicResp, tt.responseModel)

			// Check that the result has the correct structure
			assert.Equal(t, tt.anthropicResp.ID, result["id"])
			assert.Equal(t, "chat.completion", result["object"])
			assert.Equal(t, tt.responseModel, result["model"])

			// Check choices
			choices, ok := result["choices"].([]map[string]interface{})
			require.True(t, ok)
			require.Len(t, choices, 1)

			choice := choices[0]
			assert.Equal(t, int(0), choice["index"])

			// Check message
			message, ok := choice["message"].(map[string]interface{})
			require.True(t, ok)

			// Check content
			if tt.expectContent != "" {
				assert.Equal(t, tt.expectContent, message["content"])
			}

			// Check tool calls
			if tt.expectTools {
				toolCalls, ok := message["tool_calls"].([]map[string]interface{})
				require.True(t, ok)
				require.Len(t, toolCalls, 1)

				toolCall := toolCalls[0]
				assert.Equal(t, "tool_123", toolCall["id"])
				assert.Equal(t, "function", toolCall["type"])

				funcMap := toolCall["function"].(map[string]interface{})
				assert.Equal(t, "get_weather", funcMap["name"])
				assert.Equal(t, `{"location":"New York","unit":"celsius"}`, funcMap["arguments"])
			}

			// Check usage
			usage, ok := result["usage"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, tt.anthropicResp.Usage.InputTokens, usage["prompt_tokens"])
			assert.Equal(t, tt.anthropicResp.Usage.OutputTokens, usage["completion_tokens"])
			assert.Equal(t, tt.anthropicResp.Usage.InputTokens+tt.anthropicResp.Usage.OutputTokens, usage["total_tokens"])

			// Check that created timestamp is recent (within 5 seconds)
			created, ok := result["created"].(int64)
			require.True(t, ok)
			assert.InDelta(t, time.Now().Unix(), created, 5)
		})
	}
}

func TestConvertOpenAIToAnthropicRequest(t *testing.T) {
	tests := []struct {
		name               string
		req                *openai.ChatCompletionNewParams
		expectedModel      string
		expectedMaxTokens  int64
		expectedMessageLen int
	}{
		{
			name: "simple user message",
			req: &openai.ChatCompletionNewParams{
				Model:     openai.ChatModel("gpt-4"),
				Messages:  []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello, how are you?")},
				MaxTokens: openai.Opt(int64(100)),
			},
			expectedModel:      "gpt-4",
			expectedMaxTokens:  100,
			expectedMessageLen: 1,
		},
		{
			name: "system and user messages",
			req: &openai.ChatCompletionNewParams{
				Model:     openai.ChatModel("gpt-3.5-turbo"),
				Messages:  []openai.ChatCompletionMessageParamUnion{openai.SystemMessage("You are a helpful assistant."), openai.UserMessage("What is the capital of France?")},
				MaxTokens: openai.Opt(int64(100)),
			},
			expectedModel:      "gpt-3.5-turbo",
			expectedMaxTokens:  100,
			expectedMessageLen: 1, // System messages are handled separately in Anthropic
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertOpenAIToAnthropicRequest(tt.req)

			assert.Equal(t, anthropic.Model(tt.expectedModel), result.Model)
			assert.Equal(t, tt.expectedMaxTokens, result.MaxTokens)
			assert.Len(t, result.Messages, tt.expectedMessageLen)
		})
	}
}

func TestConvertOpenAIToolsToAnthropic(t *testing.T) {
	tests := []struct {
		name     string
		tools    []openai.ChatCompletionToolUnionParam
		expected int
	}{
		{
			name:     "empty tools",
			tools:    []openai.ChatCompletionToolUnionParam{},
			expected: 0,
		},
		{
			name:     "nil tools",
			tools:    nil,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertOpenAIToolsToAnthropic(tt.tools)
			assert.Len(t, result, tt.expected)
		})
	}
}

func TestConvertOpenAIToolChoice(t *testing.T) {
	tests := []struct {
		name string
		tc   *openai.ChatCompletionToolChoiceOptionUnionParam
	}{
		{
			name: "auto tool choice",
			tc: &openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: openai.Opt("auto"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertOpenAIToolChoice(tt.tc)
			assert.NotNil(t, result)
		})
	}
}

func TestConvertAnthropicToOpenAI(t *testing.T) {
	tests := []struct {
		name               string
		anthropicReq       *anthropic.MessageNewParams
		expectedModel      string
		expectedMaxTokens  int64
		expectedMessageLen int
	}{
		{
			name: "user message only",
			anthropicReq: &anthropic.MessageNewParams{
				Model:     anthropic.Model("claude-3-5-sonnet-latest"),
				MaxTokens: 100,
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock("Hello, world!")),
				},
			},
			expectedModel:      "claude-3-5-sonnet-latest",
			expectedMaxTokens:  100,
			expectedMessageLen: 1,
		},
		{
			name: "system and user messages",
			anthropicReq: &anthropic.MessageNewParams{
				Model:     anthropic.Model("claude-3-5-haiku-latest"),
				MaxTokens: 150,
				System: []anthropic.TextBlockParam{
					{Text: "You are a helpful assistant."},
				},
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock("What is 2+2?")),
				},
			},
			expectedModel:      "claude-3-5-haiku-latest",
			expectedMaxTokens:  150,
			expectedMessageLen: 2, // System message + user message
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertAnthropicToOpenAI(tt.anthropicReq)

			assert.Equal(t, openai.ChatModel(tt.expectedModel), result.Model)
			assert.Equal(t, tt.expectedMaxTokens, result.MaxTokens.Value)
			assert.Len(t, result.Messages, tt.expectedMessageLen)
		})
	}
}

func TestConvertContentBlocksToString(t *testing.T) {
	tests := []struct {
		name     string
		blocks   []anthropic.ContentBlockParamUnion
		expected string
	}{
		{
			name:     "empty blocks",
			blocks:   []anthropic.ContentBlockParamUnion{},
			expected: "",
		},
		{
			name: "single text block",
			blocks: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("Hello, world!"),
			},
			expected: "Hello, world!",
		},
		{
			name: "multiple text blocks",
			blocks: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("Hello, "),
				anthropic.NewTextBlock("world!"),
			},
			expected: "Hello, world!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertContentBlocksToString(tt.blocks)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertTextBlocksToString(t *testing.T) {
	tests := []struct {
		name     string
		blocks   []anthropic.TextBlockParam
		expected string
	}{
		{
			name:     "empty blocks",
			blocks:   []anthropic.TextBlockParam{},
			expected: "",
		},
		{
			name: "single block",
			blocks: []anthropic.TextBlockParam{
				{Text: "Hello"},
			},
			expected: "Hello",
		},
		{
			name: "multiple blocks",
			blocks: []anthropic.TextBlockParam{
				{Text: "Hello"},
				{Text: ", "},
				{Text: "world!"},
			},
			expected: "Hello, world!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertTextBlocksToString(tt.blocks)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertOpenAIToAnthropic(t *testing.T) {
	tests := []struct {
		name       string
		openaiResp *openai.ChatCompletion
		model      string
	}{
		{
			name: "simple text response",
			openaiResp: &openai.ChatCompletion{
				ID: "chatcmpl-123",
				Choices: []openai.ChatCompletionChoice{
					{
						Index: 0,
						Message: openai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "Hello! How can I help you today?"},
						FinishReason: "stop",
					},
				},
				Usage: openai.CompletionUsage{
					PromptTokens:     10,
					CompletionTokens: 15,
					TotalTokens:      25,
				},
			},
			model: "gpt-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertOpenAIToAnthropic(tt.openaiResp, tt.model)

			// Basic structure checks
			assert.NotEmpty(t, result.ID)
			assert.Equal(t, constant.Message("message"), result.Type)
			assert.Equal(t, constant.Message("assistant"), constant.Message(result.Role))

			// Check content blocks
			assert.NotEmpty(t, result.Content)

			// Verify usage tokens are mapped correctly
			assert.Equal(t, tt.openaiResp.Usage.PromptTokens, result.Usage.InputTokens)
			assert.Equal(t, tt.openaiResp.Usage.CompletionTokens, result.Usage.OutputTokens)
		})
	}
}

// Benchmark tests
func BenchmarkConvertAnthropicResponseToOpenAI(b *testing.B) {
	anthropicResp := &anthropic.Message{
		ID:    "msg_benchmark",
		Model: "claude-3-sonnet",
		Role:  "assistant",
		Type:  "message",
		Content: []anthropic.ContentBlockUnion{
			{
				Type: "text",
				Text: "This is a benchmark test response.",
			},
			{
				Type:  "tool_use",
				ID:    "tool_benchmark",
				Name:  "benchmark_tool",
				Input: json.RawMessage(`{"param1":"value1","param2":42}`),
			},
		},
		Usage: anthropic.Usage{
			InputTokens:  100,
			OutputTokens: 200,
		},
		StopReason:   "end_turn",
		StopSequence: "",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConvertAnthropicResponseToOpenAI(anthropicResp, "claude-3-sonnet")
	}
}

func BenchmarkConvertOpenAIToAnthropicRequest(b *testing.B) {
	req := &openai.ChatCompletionNewParams{
		Model:     openai.ChatModel("gpt-4"),
		Messages:  []openai.ChatCompletionMessageParamUnion{openai.SystemMessage("You are a helpful assistant."), openai.UserMessage("Hello, how are you?")},
		MaxTokens: openai.Opt(int64(100)),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConvertOpenAIToAnthropicRequest(req)
	}
}

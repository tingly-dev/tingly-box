package adaptor

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestConvertOpenAIToAnthropicRequest(t *testing.T) {
	tests := []struct {
		name               string
		req                *openai.ChatCompletionNewParams
		expectedModel      string
		expectedMaxTokens  int64
		expectedMessageLen int
		expectedSystem     string
		expectedTools      int
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
			expectedTools:      0,
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
			expectedSystem:     "You are a helpful assistant.",
			expectedTools:      0,
		},
		{
			name: "assistant message with tool call",
			req: &openai.ChatCompletionNewParams{
				Model: openai.ChatModel("gpt-4-turbo"),
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("What's the weather like in New York?"),
					// Use a simpler approach - test with a regular assistant message for now
					openai.AssistantMessage("I'll check the weather for you."),
				},
				MaxTokens: openai.Opt(int64(200)),
			},
			expectedModel:      "gpt-4-turbo",
			expectedMaxTokens:  200,
			expectedMessageLen: 2,
			expectedTools:      0, // No tools in this simple test
		},
		{
			name: "assistant with text",
			req: &openai.ChatCompletionNewParams{
				Model: openai.ChatModel("gpt-4"),
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("Send an email to john@example.com"),
					openai.AssistantMessage("I'll help you send that email."),
				},
				MaxTokens: openai.Opt(int64(150)),
			},
			expectedModel:      "gpt-4",
			expectedMaxTokens:  150,
			expectedMessageLen: 2,
			expectedTools:      0,
		},
		{
			name: "tool result message",
			req: &openai.ChatCompletionNewParams{
				Model: openai.ChatModel("gpt-4"),
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("What's the weather like?"),
					func() openai.ChatCompletionMessageParamUnion {
						msgRaw := json.RawMessage(`{
							"content": null,
							"tool_calls": [{
								"id": "call_789",
								"type": "function",
								"function": {
									"name": "get_weather",
									"arguments": "{\"location\":\"Paris\"}"
								}
							}]
						}`)
						var result openai.ChatCompletionMessageParamUnion
						_ = json.Unmarshal(msgRaw, &result)
						return result
					}(),
					openai.ToolMessage("call_789", "The weather in Paris is sunny, 22°C"),
				},
				MaxTokens: openai.Opt(int64(100)),
			},
			expectedModel:      "gpt-4",
			expectedMaxTokens:  100,
			expectedMessageLen: 2, // user message + tool result
			expectedTools:      0, // Tool messages are converted to tool_result blocks in user messages
		},
		{
			name: "tool result message",
			req: &openai.ChatCompletionNewParams{
				Model: openai.ChatModel("gpt-4"),
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("What's the weather like?"),
					func() openai.ChatCompletionMessageParamUnion {
						msgRaw := json.RawMessage(`{
							"content": null,
							"tool_calls": [{
								"id": "call_789",
								"type": "function",
								"function": {
									"name": "get_weather",
									"arguments": "{\"location\":\"Paris\"}"
								}
							}]
						}`)
						var result openai.ChatCompletionMessageParamUnion
						_ = json.Unmarshal(msgRaw, &result)
						return result
					}(),
					openai.ToolMessage("call_789", "The weather in Paris is sunny, 22°C"),
				},
				MaxTokens: openai.Opt(int64(100)),
				Tools: []openai.ChatCompletionToolUnionParam{
					newExampleTool(),
				},
			},
			expectedModel:      "gpt-4",
			expectedMaxTokens:  100,
			expectedMessageLen: 2, // user message + tool result
			expectedTools:      1, // Tool messages are converted to tool_result blocks in user messages
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertOpenAIToAnthropicRequest(tt.req, 8192) // Use default max tokens

			assert.Equal(t, anthropic.Model(tt.expectedModel), result.Model)
			assert.Equal(t, tt.expectedMaxTokens, result.MaxTokens)
			assert.Len(t, result.Messages, tt.expectedMessageLen)

			// Check system message if expected
			if tt.expectedSystem != "" {
				require.Len(t, result.System, 1)
				assert.Equal(t, tt.expectedSystem, result.System[0].Text)
			}

			// Count tool_use blocks
			toolCount := len(result.Tools)
			assert.Equal(t, tt.expectedTools, toolCount)

			for _, tool := range tt.req.Tools {
				data, _ := json.MarshalIndent(tool, "", "  ")
				fmt.Printf("%s\n", data)
			}

			for _, tool := range result.Tools {
				data, _ := json.MarshalIndent(tool, "", "  ")
				fmt.Printf("%s\n", data)
			}
		})
	}
}

func TestConvertOpenAIToAnthropicTools(t *testing.T) {
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
		{
			name: "simple tool",
			tools: func() []openai.ChatCompletionToolUnionParam {
				tool := newExampleTool()
				return []openai.ChatCompletionToolUnionParam{tool}
			}(),
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertOpenAIToAnthropicTools(tt.tools)
			assert.Len(t, result, tt.expected)

			// Verify tool structure if we have tools
			if tt.expected > 0 && len(result) > 0 && result[0].OfTool != nil {
				assert.Equal(t, "get_weather", result[0].OfTool.Name)
				assert.Equal(t, "Get the current weather for a location", result[0].OfTool.Description.Value)
			}
		})
	}
}

func TestConvertOpenAIToAnthropicToolChoice(t *testing.T) {
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
			result := ConvertOpenAIToAnthropicToolChoice(tt.tc)
			assert.NotNil(t, result)
		})
	}
}

func TestConvertAnthropicToOpenAIRequest(t *testing.T) {
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
			result := ConvertAnthropicToOpenAIRequest(tt.anthropicReq, false)

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

func TestTransformProperties(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "transform exclusiveMinimum to minimum",
			input: map[string]interface{}{
				"filePath": map[string]interface{}{
					"type":             "string",
					"description":      "The file path",
					"exclusiveMinimum": 0,
				},
			},
			expected: map[string]interface{}{
				"filePath": map[string]interface{}{
					"type":        "string",
					"description": "The file path",
					"minimum":     0,
				},
			},
		},
		{
			name: "transform exclusiveMaximum to maximum",
			input: map[string]interface{}{
				"count": map[string]interface{}{
					"type":             "integer",
					"description":      "The count",
					"exclusiveMaximum": 100,
				},
			},
			expected: map[string]interface{}{
				"count": map[string]interface{}{
					"type":        "integer",
					"description": "The count",
					"maximum":     100,
				},
			},
		},
		{
			name: "transform both exclusiveMinimum and exclusiveMaximum",
			input: map[string]interface{}{
				"value": map[string]interface{}{
					"type":             "number",
					"exclusiveMinimum": 0,
					"exclusiveMaximum": 100,
				},
			},
			expected: map[string]interface{}{
				"value": map[string]interface{}{
					"type":    "number",
					"minimum": 0,
					"maximum": 100,
				},
			},
		},
		{
			name: "preserve other fields",
			input: map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The name",
					"minLength":   1,
					"maxLength":   100,
				},
			},
			expected: map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The name",
					"minLength":   1,
					"maxLength":   100,
				},
			},
		},
		{
			name: "nested properties in items",
			input: map[string]interface{}{
				"items": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type":             "integer",
						"exclusiveMinimum": 1,
					},
				},
			},
			expected: map[string]interface{}{
				"items": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type":    "integer",
						"minimum": 1,
					},
				},
			},
		},
		{
			name: "nested properties in anyOf",
			input: map[string]interface{}{
				"value": map[string]interface{}{
					"anyOf": []interface{}{
						map[string]interface{}{
							"type":             "integer",
							"exclusiveMinimum": 0,
						},
						map[string]interface{}{
							"type":             "string",
							"exclusiveMinimum": "a",
						},
					},
				},
			},
			expected: map[string]interface{}{
				"value": map[string]interface{}{
					"anyOf": []interface{}{
						map[string]interface{}{
							"type":    "integer",
							"minimum": 0,
						},
						map[string]interface{}{
							"type":    "string",
							"minimum": "a",
						},
					},
				},
			},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty input",
			input:    map[string]interface{}{},
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transformProperties(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransformPropertySchema(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "transform exclusiveMinimum to minimum",
			input: map[string]interface{}{
				"type":             "number",
				"exclusiveMinimum": 0,
			},
			expected: map[string]interface{}{
				"type":    "number",
				"minimum": 0,
			},
		},
		{
			name: "transform exclusiveMaximum to maximum",
			input: map[string]interface{}{
				"type":             "number",
				"exclusiveMaximum": 100,
			},
			expected: map[string]interface{}{
				"type":    "number",
				"maximum": 100,
			},
		},
		{
			name: "transform both fields",
			input: map[string]interface{}{
				"type":             "number",
				"exclusiveMinimum": 0,
				"exclusiveMaximum": 100,
			},
			expected: map[string]interface{}{
				"type":    "number",
				"minimum": 0,
				"maximum": 100,
			},
		},
		{
			name: "nested items",
			input: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type":             "integer",
					"exclusiveMinimum": 1,
				},
			},
			expected: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type":    "integer",
					"minimum": 1,
				},
			},
		},
		{
			name: "nested anyOf",
			input: map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{
						"type":             "integer",
						"exclusiveMinimum": 0,
					},
				},
			},
			expected: map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{
						"type":    "integer",
						"minimum": 0,
					},
				},
			},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transformPropertySchema(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertOpenAIToGoogleRequestComplex tests complex OpenAI to Google request conversions
func TestConvertOpenAIToGoogleRequestComplex(t *testing.T) {
	t.Run("multi-turn conversation with tools", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("What's the weather in NYC?"),
				func() openai.ChatCompletionMessageParamUnion {
					msgRaw := json.RawMessage(`{
						"content": "I'll check the weather for you.",
						"tool_calls": [{
							"id": "call_1",
							"type": "function",
							"function": {
								"name": "get_weather",
								"arguments": "{\"location\":\"NYC\"}"
							}
						}]
					}`)
					var result openai.ChatCompletionMessageParamUnion
					_ = json.Unmarshal(msgRaw, &result)
					return result
				}(),
				openai.ToolMessage("call_1", "Sunny, 22°C"),
			},
			MaxTokens:   openai.Opt(int64(1000)),
			Temperature: openai.Opt(float64(0.7)),
			TopP:        openai.Opt(float64(0.9)),
		}

		model, contents, config := ConvertOpenAIToGoogleRequest(req, 4096)

		// Verify basic structure
		assert.Equal(t, "gpt-4", model)
		assert.Equal(t, int32(1000), config.MaxOutputTokens)
		assert.InDelta(t, 0.7, *config.Temperature, 0.01)
		assert.InDelta(t, 0.9, *config.TopP, 0.01)

		// Should have 3 contents: user, model (with function call), user (with function response)
		assert.Len(t, contents, 3)

		// First content: user message
		assert.Equal(t, "user", contents[0].Role)
		assert.Contains(t, contents[0].Parts[0].Text, "weather in NYC")

		// Second content: model with function call
		assert.Equal(t, "model", contents[1].Role)
		assert.NotNil(t, contents[1].Parts[0].FunctionCall)
		assert.Equal(t, "get_weather", contents[1].Parts[0].FunctionCall.Name)

		// Third content: function response
		assert.Equal(t, "user", contents[2].Role)
		assert.NotNil(t, contents[2].Parts[0].FunctionResponse)
	})

	t.Run("with system instruction", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage("You are a helpful assistant.\nAlways be accurate."),
				openai.UserMessage("Hello"),
			},
			MaxTokens: openai.Opt(int64(100)),
		}

		_, _, config := ConvertOpenAIToGoogleRequest(req, 4096)

		// System instruction should be set
		require.NotNil(t, config.SystemInstruction)
		assert.Equal(t, "system", config.SystemInstruction.Role)
		assert.Contains(t, config.SystemInstruction.Parts[0].Text, "helpful assistant")
		assert.Contains(t, config.SystemInstruction.Parts[0].Text, "accurate")
	})

	t.Run("with tools and function declarations", func(t *testing.T) {
		tool := openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "search",
			Description: param.Opt[string]{Value: "Search the web"},
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query",
					},
				},
				"required": []string{"query"},
			},
		})

		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Search for AI news"),
			},
			Tools:     []openai.ChatCompletionToolUnionParam{tool},
			MaxTokens: openai.Opt(int64(100)),
		}

		_, _, config := ConvertOpenAIToGoogleRequest(req, 4096)

		// Tools should be converted
		require.NotNil(t, config.Tools)
		require.Len(t, config.Tools, 1)
		require.Len(t, config.Tools[0].FunctionDeclarations, 1)

		funcDecl := config.Tools[0].FunctionDeclarations[0]
		assert.Equal(t, "search", funcDecl.Name)
		assert.Equal(t, "Search the web", funcDecl.Description)

		// Verify schema normalization (lowercase to uppercase)
		require.NotNil(t, funcDecl.Parameters)
		assert.Equal(t, genai.TypeObject, funcDecl.Parameters.Type)
	})

	t.Run("multimodal content with text array", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4-vision"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				func() openai.ChatCompletionMessageParamUnion {
					msgRaw := json.RawMessage(`{
						"role": "user",
						"content": [
							{"type": "text", "text": "What's in this image?"},
							{"type": "text", "text": "Please describe it."}
						]
					}`)
					var result openai.ChatCompletionMessageParamUnion
					_ = json.Unmarshal(msgRaw, &result)
					return result
				}(),
			},
			MaxTokens: openai.Opt(int64(100)),
		}

		_, contents, _ := ConvertOpenAIToGoogleRequest(req, 4096)

		// Should concatenate text parts
		assert.Len(t, contents, 1)
		assert.Len(t, contents[0].Parts, 2)
	})

	t.Run("multiple tool calls in assistant message", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Compare weather in NYC and Tokyo"),
				func() openai.ChatCompletionMessageParamUnion {
					msgRaw := json.RawMessage(`{
						"content": null,
						"tool_calls": [
							{
								"id": "call_nyc",
								"type": "function",
								"function": {
									"name": "get_weather",
									"arguments": "{\"location\":\"NYC\"}"
								}
							},
							{
								"id": "call_tokyo",
								"type": "function",
								"function": {
									"name": "get_weather",
									"arguments": "{\"location\":\"Tokyo\"}"
								}
							}
						]
					}`)
					var result openai.ChatCompletionMessageParamUnion
					_ = json.Unmarshal(msgRaw, &result)
					return result
				}(),
			},
			MaxTokens: openai.Opt(int64(100)),
		}

		_, contents, _ := ConvertOpenAIToGoogleRequest(req, 4096)

		// Assistant message should have 2 function calls
		assert.Len(t, contents, 2)
		assert.Equal(t, "model", contents[1].Role)
		assert.Len(t, contents[1].Parts, 2)

		// Verify both function calls
		assert.Equal(t, "call_nyc", contents[1].Parts[0].FunctionCall.ID)
		assert.Equal(t, "call_tokyo", contents[1].Parts[1].FunctionCall.ID)
	})
}

// TestConvertAnthropicToGoogleRequestComplex tests complex Anthropic to Google request conversions
func TestConvertAnthropicToGoogleRequestComplex(t *testing.T) {
	t.Run("multi-turn conversation with tool use", func(t *testing.T) {
		req := &anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-3"),
			MaxTokens: 4096,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("What's the weather in Paris?")),
				anthropic.NewAssistantMessage(
					anthropic.NewToolUseBlock("toolu_123", map[string]interface{}{"city": "Paris"}, "get_weather"),
				),
				anthropic.NewUserMessage(
					anthropic.NewToolResultBlock("toolu_123", "Sunny, 25°C", false),
				),
			},
		}

		_, contents, _ := ConvertAnthropicToGoogleRequest(req, 4096)

		// Should have 3 contents
		assert.Len(t, contents, 3)

		// First: user message
		assert.Equal(t, "user", contents[0].Role)

		// Second: model with function call
		assert.Equal(t, "model", contents[1].Role)
		assert.NotNil(t, contents[1].Parts[0].FunctionCall)
		assert.Equal(t, "get_weather", contents[1].Parts[0].FunctionCall.Name)
		assert.Equal(t, "toolu_123", contents[1].Parts[0].FunctionCall.ID)

		// Third: function response
		assert.Equal(t, "user", contents[2].Role)
		assert.NotNil(t, contents[2].Parts[0].FunctionResponse)
		assert.Equal(t, "toolu_123", contents[2].Parts[0].FunctionResponse.Name)
	})

	t.Run("with system instruction", func(t *testing.T) {
		req := &anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-3"),
			MaxTokens: 4096,
			System: []anthropic.TextBlockParam{
				{Text: "You are a helpful AI assistant."},
				{Text: "Always be concise and accurate."},
			},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
			},
		}

		_, _, config := ConvertAnthropicToGoogleRequest(req, 4096)

		// System instruction should be set with concatenated text
		require.NotNil(t, config.SystemInstruction)
		assert.Contains(t, config.SystemInstruction.Parts[0].Text, "helpful AI assistant")
		assert.Contains(t, config.SystemInstruction.Parts[0].Text, "concise and accurate")
	})

	t.Run("assistant message with text and tool use", func(t *testing.T) {
		req := &anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-3"),
			MaxTokens: 4096,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Search for news")),
				func() anthropic.MessageParam {
					return anthropic.NewAssistantMessage(
						anthropic.NewTextBlock("I'll search for recent news."),
						anthropic.NewToolUseBlock("toolu_456", map[string]interface{}{"query": "news"}, "search"),
					)
				}(),
			},
		}

		_, contents, _ := ConvertAnthropicToGoogleRequest(req, 4096)

		// Assistant message should have both text and function call
		assert.Len(t, contents, 2)
		assert.Equal(t, "model", contents[1].Role)
		assert.Len(t, contents[1].Parts, 2)

		// First part: text
		assert.Contains(t, contents[1].Parts[0].Text, "search for recent news")

		// Second part: function call
		assert.Equal(t, "search", contents[1].Parts[1].FunctionCall.Name)
	})

	t.Run("tool result with complex JSON data", func(t *testing.T) {
		req := &anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-3"),
			MaxTokens: 4096,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Get data")),
				anthropic.NewAssistantMessage(
					anthropic.NewToolUseBlock("toolu_789", map[string]interface{}{"id": 123}, "get_data"),
				),
				anthropic.NewUserMessage(
					anthropic.NewToolResultBlock("toolu_789", `{"status":"success","data":{"items":["a","b"],"count":2}}`, false),
				),
			},
		}

		_, contents, _ := ConvertAnthropicToGoogleRequest(req, 4096)

		// Tool result should be properly formatted
		assert.Equal(t, "user", contents[2].Role)
		funcResp := contents[2].Parts[0].FunctionResponse
		require.NotNil(t, funcResp)

		// Response should have proper structure
		assert.NotNil(t, funcResp.Response)
		assert.Contains(t, funcResp.Response, "status")
		assert.Contains(t, funcResp.Response, "data")
	})

	t.Run("with tools and tool choice", func(t *testing.T) {
		req := &anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-3"),
			MaxTokens: 4096,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Use a tool")),
			},
			Tools: []anthropic.ToolUnionParam{
				anthropic.ToolUnionParam{
					OfTool: &anthropic.ToolParam{
						Name:        "search",
						Description: anthropic.Opt("Search the web"),
						InputSchema: anthropic.ToolInputSchemaParam{
							Type: "object",
							Properties: map[string]interface{}{
								"query": map[string]interface{}{
									"type":        "string",
									"description": "Search query",
								},
							},
						},
					},
				},
			},
		}

		_, _, config := ConvertAnthropicToGoogleRequest(req, 4096)

		// Tools should be converted
		require.NotNil(t, config.Tools)
		require.Len(t, config.Tools[0].FunctionDeclarations, 1)

		funcDecl := config.Tools[0].FunctionDeclarations[0]
		assert.Equal(t, "search", funcDecl.Name)

		// Verify schema type normalization
		assert.Equal(t, genai.TypeObject, funcDecl.Parameters.Type)
	})
}

// TestNormalizeSchemaTypes tests schema type normalization from lowercase to uppercase
func TestNormalizeSchemaTypes(t *testing.T) {
	t.Run("basic type normalization", func(t *testing.T) {
		schema := &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"name":   {Type: "string"},
				"age":    {Type: "integer"},
				"score":  {Type: "number"},
				"active": {Type: "boolean"},
				"items":  {Type: "array"},
			},
		}

		normalizeSchemaTypes(schema)

		assert.Equal(t, genai.TypeObject, schema.Type)
		assert.Equal(t, genai.TypeString, schema.Properties["name"].Type)
		assert.Equal(t, genai.TypeInteger, schema.Properties["age"].Type)
		assert.Equal(t, genai.TypeNumber, schema.Properties["score"].Type)
		assert.Equal(t, genai.TypeBoolean, schema.Properties["active"].Type)
		assert.Equal(t, genai.TypeArray, schema.Properties["items"].Type)
	})

	t.Run("nested array items", func(t *testing.T) {
		schema := &genai.Schema{
			Type: "array",
			Items: &genai.Schema{
				Type: "object",
				Properties: map[string]*genai.Schema{
					"id": {Type: "string"},
				},
			},
		}

		normalizeSchemaTypes(schema)

		assert.Equal(t, genai.TypeArray, schema.Type)
		assert.Equal(t, genai.TypeObject, schema.Items.Type)
		assert.Equal(t, genai.TypeString, schema.Items.Properties["id"].Type)
	})

	t.Run("nested anyOf schemas", func(t *testing.T) {
		schema := &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"value": {
					AnyOf: []*genai.Schema{
						{Type: "string"},
						{Type: "integer"},
					},
				},
			},
		}

		normalizeSchemaTypes(schema)

		assert.Equal(t, genai.TypeObject, schema.Type)
		assert.Equal(t, genai.TypeString, schema.Properties["value"].AnyOf[0].Type)
		assert.Equal(t, genai.TypeInteger, schema.Properties["value"].AnyOf[1].Type)
	})

	t.Run("nil schema", func(t *testing.T) {
		normalizeSchemaTypes(nil)
		// Should not panic
	})
}

// TestConvertOpenAIToGoogleTools tests tool conversion
func TestConvertOpenAIToGoogleTools(t *testing.T) {
	t.Run("multiple tools with complex schemas", func(t *testing.T) {
		tools := []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        "get_weather",
				Description: param.Opt[string]{Value: "Get weather"},
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]interface{}{
							"type":        "string",
							"description": "Location",
						},
					},
				},
			}),
			openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
				Name:        "calculate",
				Description: param.Opt[string]{Value: "Calculate"},
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"expression": map[string]interface{}{
							"type":        "string",
							"description": "Math expression",
						},
					},
				},
			}),
		}

		result := ConvertOpenAIToGoogleTools(tools)

		assert.Len(t, result, 2)
		assert.Equal(t, "get_weather", result[0].Name)
		assert.Equal(t, "calculate", result[1].Name)

		// Verify schema normalization
		assert.Equal(t, genai.TypeObject, result[0].Parameters.Type)
	})

	t.Run("empty tools", func(t *testing.T) {
		result := ConvertOpenAIToGoogleTools([]openai.ChatCompletionToolUnionParam{})
		assert.Nil(t, result)
	})
}

// TestConvertOpenAIToGoogleToolChoice tests tool choice conversion
func TestConvertOpenAIToGoogleToolChoice(t *testing.T) {
	t.Run("auto mode", func(t *testing.T) {
		tc := &openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.Opt("auto"),
		}

		result := ConvertOpenAIToGoogleToolChoice(tc)

		assert.Equal(t, genai.FunctionCallingConfigModeAuto, result.FunctionCallingConfig.Mode)
	})

	t.Run("specific function", func(t *testing.T) {
		// Create tool choice with specific function using JSON
		tcRaw := json.RawMessage(`{
			"type": "function",
			"function": {
				"name": "get_weather"
			}
		}`)
		var tc openai.ChatCompletionToolChoiceOptionUnionParam
		_ = json.Unmarshal(tcRaw, &tc)

		result := ConvertOpenAIToGoogleToolChoice(&tc)

		assert.Equal(t, genai.FunctionCallingConfigModeAny, result.FunctionCallingConfig.Mode)
		assert.Equal(t, []string{"get_weather"}, result.FunctionCallingConfig.AllowedFunctionNames)
	})
}

// TestConvertAnthropicToGoogleTools tests Anthropic tool conversion
func TestConvertAnthropicToGoogleTools(t *testing.T) {
	t.Run("convert Anthropic tools to Google format", func(t *testing.T) {
		tools := []anthropic.ToolUnionParam{
			{
				OfTool: &anthropic.ToolParam{
					Name:        "search",
					Description: anthropic.Opt("Search web"),
					InputSchema: anthropic.ToolInputSchemaParam{
						Type: "object",
						Properties: map[string]interface{}{
							"query": map[string]interface{}{
								"type":        "string",
								"description": "Query",
							},
						},
					},
				},
			},
		}

		result := ConvertAnthropicToGoogleTools(tools)

		assert.Len(t, result, 1)
		assert.Equal(t, "search", result[0].Name)
		assert.Equal(t, "Search web", result[0].Description)

		// Verify type normalization
		assert.Equal(t, genai.TypeObject, result[0].Parameters.Type)
	})
}

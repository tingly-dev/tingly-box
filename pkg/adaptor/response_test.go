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
	"google.golang.org/genai"
)

func TestConvertAnthropicToOpenAIResponse(t *testing.T) {
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
			result := ConvertAnthropicToOpenAIResponse(tt.anthropicResp, tt.responseModel)

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

func TestConvertOpenAIToAnthropicResponse(t *testing.T) {
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
			result := ConvertOpenAIToAnthropicResponse(tt.openaiResp, tt.model)

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

// TestConvertOpenAIToGoogleResponse tests converting OpenAI response to Google format
func TestConvertOpenAIToGoogleResponse(t *testing.T) {
	t.Run("text only response", func(t *testing.T) {
		resp := &openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Role:    "assistant",
						Content: "Hello!",
					},
					FinishReason: "stop",
				},
			},
			Usage: openai.CompletionUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}

		googleResp := ConvertOpenAIToGoogleResponse(resp)

		if len(googleResp.Candidates) != 1 {
			t.Errorf("expected 1 candidate, got %d", len(googleResp.Candidates))
		}
		if googleResp.Candidates[0].Content.Role != "model" {
			t.Errorf("expected role 'model', got '%s'", googleResp.Candidates[0].Content.Role)
		}
		if len(googleResp.Candidates[0].Content.Parts) != 1 {
			t.Errorf("expected 1 part, got %d", len(googleResp.Candidates[0].Content.Parts))
		}
		if googleResp.Candidates[0].Content.Parts[0].Text != "Hello!" {
			t.Errorf("expected text 'Hello!', got '%s'", googleResp.Candidates[0].Content.Parts[0].Text)
		}
		if googleResp.UsageMetadata.PromptTokenCount != 10 {
			t.Errorf("expected prompt tokens 10, got %d", googleResp.UsageMetadata.PromptTokenCount)
		}
	})

	t.Run("with tool calls", func(t *testing.T) {
		resp := &openai.ChatCompletion{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Role:    "assistant",
						Content: "Calling tool",
					},
					FinishReason: "tool_calls",
				},
			},
			Usage: openai.CompletionUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}

		googleResp := ConvertOpenAIToGoogleResponse(resp)

		if len(googleResp.Candidates) != 1 {
			t.Errorf("expected 1 candidate, got %d", len(googleResp.Candidates))
		}
		if googleResp.Candidates[0].Content.Parts[0].Text != "Calling tool" {
			t.Errorf("expected text 'Calling tool', got '%s'", googleResp.Candidates[0].Content.Parts[0].Text)
		}
	})
}

// TestConvertAnthropicToGoogleResponse tests converting Anthropic response to Google format
func TestConvertAnthropicToGoogleResponse(t *testing.T) {
	t.Run("text only response", func(t *testing.T) {
		resp := &anthropic.Message{
			ID:   "msg_123",
			Type: "message",
			Role: "assistant",
			Content: []anthropic.ContentBlockUnion{
				{Type: "text", Text: "Hello!"},
			},
			StopReason: "end_turn",
			Usage: anthropic.Usage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		}

		googleResp := ConvertAnthropicToGoogleResponse(resp)

		if len(googleResp.Candidates) != 1 {
			t.Errorf("expected 1 candidate, got %d", len(googleResp.Candidates))
		}
		if googleResp.Candidates[0].Content.Parts[0].Text != "Hello!" {
			t.Errorf("expected text 'Hello!', got '%s'", googleResp.Candidates[0].Content.Parts[0].Text)
		}
		if googleResp.Candidates[0].FinishReason != genai.FinishReasonStop {
			t.Errorf("expected finish reason STOP, got %v", googleResp.Candidates[0].FinishReason)
		}
	})

	t.Run("with tool use", func(t *testing.T) {
		resp := &anthropic.Message{
			ID:   "msg_123",
			Type: "message",
			Role: "assistant",
			Content: []anthropic.ContentBlockUnion{
				{
					Type:  "tool_use",
					ID:    "toolu_123",
					Name:  "get_weather",
					Input: []byte(`{"loc":"NYC"}`),
				},
			},
			StopReason: "tool_use",
			Usage: anthropic.Usage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		}

		googleResp := ConvertAnthropicToGoogleResponse(resp)

		if googleResp.Candidates[0].Content.Parts[0].FunctionCall == nil {
			t.Error("expected function call")
		} else if googleResp.Candidates[0].Content.Parts[0].FunctionCall.Name != "get_weather" {
			t.Errorf("expected function name 'get_weather', got '%s'", googleResp.Candidates[0].Content.Parts[0].FunctionCall.Name)
		}
	})
}

package nonstream

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

func TestHandleAnthropicToOpenAI(t *testing.T) {
	tests := []struct {
		name          string
		anthropicResp *anthropic.BetaMessage
		responseModel string
		expectContent string
		expectTools   bool
	}{
		{
			name: "text only response",
			anthropicResp: &anthropic.BetaMessage{
				ID:    "msg_123",
				Model: "claude-3-sonnet",
				Role:  "assistant",
				Type:  "message",
				Content: []anthropic.BetaContentBlockUnion{
					{
						Type: "text",
						Text: "Hello, world!",
					},
				},
				Usage: anthropic.BetaUsage{
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
			anthropicResp: &anthropic.BetaMessage{
				ID:    "msg_456",
				Model: "claude-3-opus",
				Role:  "assistant",
				Type:  "message",
				Content: []anthropic.BetaContentBlockUnion{
					{
						Type:  "tool_use",
						ID:    "tool_123",
						Name:  "get_weather",
						Input: json.RawMessage(`{"location":"New York","unit":"celsius"}`),
					},
				},
				Usage: anthropic.BetaUsage{
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
			result := HandleAnthropicBetaToOpenAIResponse(tt.anthropicResp, tt.responseModel)

			assert.Equal(t, tt.anthropicResp.ID, result.ID)
			assert.Equal(t, "chat.completion", result.Object)
			assert.Equal(t, tt.responseModel, result.Model)

			require.Len(t, result.Choices, 1)
			choice := result.Choices[0]
			assert.Equal(t, 0, choice.Index)

			if tt.expectContent != "" {
				assert.Equal(t, tt.expectContent, choice.Message.Content)
			}

			if tt.expectTools {
				require.Len(t, choice.Message.ToolCalls, 1)
				tc := choice.Message.ToolCalls[0]
				assert.Equal(t, "tool_123", tc.ID)
				assert.Equal(t, "function", tc.Type)
				assert.Equal(t, "get_weather", tc.Function.Name)
				assert.Equal(t, `{"location":"New York","unit":"celsius"}`, tc.Function.Arguments)
			}

			// prompt_tokens = total (uncached + cache_read + cache_creation)
			wantPrompt := tt.anthropicResp.Usage.InputTokens +
				tt.anthropicResp.Usage.CacheReadInputTokens +
				tt.anthropicResp.Usage.CacheCreationInputTokens
			assert.Equal(t, wantPrompt, result.Usage.PromptTokens)
			assert.Equal(t, tt.anthropicResp.Usage.OutputTokens, result.Usage.CompletionTokens)
			assert.Equal(t, wantPrompt+tt.anthropicResp.Usage.OutputTokens, result.Usage.TotalTokens)

			assert.InDelta(t, time.Now().Unix(), result.Created, 5)
		})
	}
}

func TestHandleOpenAIToAnthropic(t *testing.T) {
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
			result := HandleOpenAIChatToAnthropic(tt.openaiResp, tt.model)

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

// TestHandleOpenAIToGoogle tests converting OpenAI response to Google format
func TestHandleOpenAIToGoogle(t *testing.T) {
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

		googleResp := HandleOpenAIToGoogle(resp)

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

		googleResp := HandleOpenAIToGoogle(resp)

		if len(googleResp.Candidates) != 1 {
			t.Errorf("expected 1 candidate, got %d", len(googleResp.Candidates))
		}
		if googleResp.Candidates[0].Content.Parts[0].Text != "Calling tool" {
			t.Errorf("expected text 'Calling tool', got '%s'", googleResp.Candidates[0].Content.Parts[0].Text)
		}
	})
}

// TestHandleAnthropicToGoogle tests converting Anthropic response to Google format
func TestHandleAnthropicToGoogle(t *testing.T) {
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

		googleResp := HandleAnthropicToGoogle(resp)

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

		googleResp := HandleAnthropicToGoogle(resp)

		if googleResp.Candidates[0].Content.Parts[0].FunctionCall == nil {
			t.Error("expected function call")
		} else if googleResp.Candidates[0].Content.Parts[0].FunctionCall.Name != "get_weather" {
			t.Errorf("expected function name 'get_weather', got '%s'", googleResp.Candidates[0].Content.Parts[0].FunctionCall.Name)
		}
	})
}

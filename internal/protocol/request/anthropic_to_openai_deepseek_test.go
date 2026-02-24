package request

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// TestDeepSeekReasoningContent verifies that all assistant messages include x_thinking field
// The x_thinking field is later converted to reasoning_content by applyDeepSeekTransform
func TestDeepSeekReasoningContent(t *testing.T) {
	tests := []struct {
		name           string
		anthropicMsg   anthropic.MessageParam
		expectHasField bool
	}{
		{
			name: "Simple text message without thinking",
			anthropicMsg: anthropic.MessageParam{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock("Hello!"),
				},
			},
			expectHasField: true, // x_thinking field should be present (empty string)
		},
		{
			name: "Message with thinking",
			anthropicMsg: anthropic.MessageParam{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewThinkingBlock("thinking-123", "Let me think about this"),
					anthropic.NewTextBlock("Hello!"),
				},
			},
			expectHasField: true,
		},
		{
			name: "Message with tool_calls",
			anthropicMsg: anthropic.MessageParam{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock("I'll call a tool"),
					anthropic.NewToolUseBlock("tool_123", map[string]any{"arg": "value"}, "my_function"),
				},
			},
			expectHasField: true,
		},
		{
			name: "Message with tool_calls and thinking",
			anthropicMsg: anthropic.MessageParam{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewThinkingBlock("thinking-456", "Planning the tool call"),
					anthropic.NewTextBlock("I'll call a tool"),
					anthropic.NewToolUseBlock("tool_789", map[string]any{"arg": "value"}, "my_function"),
				},
			},
			expectHasField: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to OpenAI format
			openaiMsg := convertAnthropicAssistantMessageToOpenAI(tt.anthropicMsg)

			// Marshal to JSON to check if x_thinking is present
			// (reasoning_content is added by applyDeepSeekTransform at a higher level)
			jsonBytes, err := json.Marshal(openaiMsg)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var result map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &result); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			_, hasXThinking := result["x_thinking"]
			if tt.expectHasField && !hasXThinking {
				t.Errorf("Expected x_thinking field to be present, but it was missing. JSON: %s", string(jsonBytes))
			}

			t.Logf("JSON output: %s", string(jsonBytes))
		})
	}
}

// TestDeepSeekRequestConversion tests the full request conversion for DeepSeek compatibility
func TestDeepSeekRequestConversion(t *testing.T) {
	tests := []struct {
		name          string
		anthropicReq  anthropic.MessageNewParams
		expectAllHave bool // expect all assistant messages to have reasoning_content
	}{
		{
			name: "Simple conversation",
			anthropicReq: anthropic.MessageNewParams{
				Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
				MaxTokens: 1024,
				Messages: []anthropic.MessageParam{
					{
						Role: anthropic.MessageParamRoleUser,
						Content: []anthropic.ContentBlockParamUnion{
							anthropic.NewTextBlock("Hello"),
						},
					},
					{
						Role: anthropic.MessageParamRoleAssistant,
						Content: []anthropic.ContentBlockParamUnion{
							anthropic.NewTextBlock("Hi there!"),
						},
					},
				},
			},
			expectAllHave: true,
		},
		{
			name: "Multi-turn conversation with thinking",
			anthropicReq: anthropic.MessageNewParams{
				Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
				MaxTokens: 1024,
				Messages: []anthropic.MessageParam{
					{
						Role: anthropic.MessageParamRoleUser,
						Content: []anthropic.ContentBlockParamUnion{
							anthropic.NewTextBlock("Hello"),
						},
					},
					{
						Role: anthropic.MessageParamRoleAssistant,
						Content: []anthropic.ContentBlockParamUnion{
							anthropic.NewThinkingBlock("thinking-1", "User said hello"),
							anthropic.NewTextBlock("Hi there!"),
						},
					},
					{
						Role: anthropic.MessageParamRoleUser,
						Content: []anthropic.ContentBlockParamUnion{
							anthropic.NewTextBlock("How are you?"),
						},
					},
					{
						Role: anthropic.MessageParamRoleAssistant,
						Content: []anthropic.ContentBlockParamUnion{
							anthropic.NewTextBlock("I'm doing well!"),
						},
					},
				},
			},
			expectAllHave: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openaiReq, _ := ConvertAnthropicToOpenAIRequest(&tt.anthropicReq, false, false)

			// Check all assistant messages have x_thinking
			// (reasoning_content is added by applyDeepSeekTransform at a higher level)
			for i, msg := range openaiReq.Messages {
				if msg.OfAssistant == nil {
					continue // Skip non-assistant messages
				}

				jsonBytes, err := json.Marshal(msg)
				if err != nil {
					t.Fatalf("Failed to marshal message %d: %v", i, err)
				}

				var result map[string]interface{}
				if err := json.Unmarshal(jsonBytes, &result); err != nil {
					t.Fatalf("Failed to unmarshal message %d: %v", i, err)
				}

				_, hasXThinking := result["x_thinking"]
				if tt.expectAllHave && !hasXThinking {
					t.Errorf("Assistant message %d missing x_thinking field. JSON: %s", i, string(jsonBytes))
				}

				t.Logf("Message %d: %s", i, string(jsonBytes))
			}
		})
	}
}

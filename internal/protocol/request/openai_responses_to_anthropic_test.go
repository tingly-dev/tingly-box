package request

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

func TestConvertOpenAIResponsesToAnthropicBetaRequest_SimpleInput(t *testing.T) {
	// Test simple string input with Beta API
	params := responses.ResponseNewParams{
		Model:           "gpt-4o",
		Instructions:    param.NewOpt("You are a helpful assistant."),
		MaxOutputTokens: param.NewOpt(int64(1000)),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: param.NewOpt("Hello, how are you?"),
		},
	}

	result := ConvertOpenAIResponsesToAnthropicBetaRequest(params, 4096)

	// Verify model
	if string(result.Model) != "gpt-4o" {
		t.Errorf("Expected model gpt-4o, got %s", result.Model)
	}

	// Verify system message
	if len(result.System) != 1 {
		t.Errorf("Expected 1 system message, got %d", len(result.System))
	} else if result.System[0].Text != "You are a helpful assistant." {
		t.Errorf("Expected system message 'You are a helpful assistant.', got '%s'", result.System[0].Text)
	}

	// Verify messages
	if len(result.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(result.Messages))
	} else if string(result.Messages[0].Role) != "user" {
		t.Errorf("Expected user role, got %s", result.Messages[0].Role)
	}

	// Verify max_tokens
	if result.MaxTokens != 1000 {
		t.Errorf("Expected max_tokens 1000, got %d", result.MaxTokens)
	}
}

func TestConvertOpenAIResponsesToAnthropicBetaRequest_InputItems(t *testing.T) {
	// Test input with multiple messages using Beta API
	params := responses.ResponseNewParams{
		Model: "gpt-4o",
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRole("user"),
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: param.NewOpt("What is the weather?"),
						},
					},
				},
				{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRole("assistant"),
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: param.NewOpt("It's sunny today."),
						},
					},
				},
			},
		},
	}

	result := ConvertOpenAIResponsesToAnthropicBetaRequest(params, 4096)

	// Verify messages
	if len(result.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(result.Messages))
	}

	if len(result.Messages) >= 1 && string(result.Messages[0].Role) != "user" {
		t.Errorf("Expected first message role 'user', got %s", result.Messages[0].Role)
	}

	if len(result.Messages) >= 2 && string(result.Messages[1].Role) != "assistant" {
		t.Errorf("Expected second message role 'assistant', got %s", result.Messages[1].Role)
	}
}

func TestConvertOpenAIResponsesToAnthropicBetaRequest_FunctionCall(t *testing.T) {
	// Test function call conversion with Beta API
	params := responses.ResponseNewParams{
		Model: "gpt-4o",
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				{
					OfFunctionCall: &responses.ResponseFunctionToolCallParam{
						CallID:    "call_123",
						Name:      "get_weather",
						Arguments: `{"location":"NYC"}`,
					},
				},
			},
		},
	}

	result := ConvertOpenAIResponsesToAnthropicBetaRequest(params, 4096)

	// Verify messages
	if len(result.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(result.Messages))
	}

	msg := result.Messages[0]
	if string(msg.Role) != "assistant" {
		t.Errorf("Expected assistant role, got %s", msg.Role)
	}

	// Verify tool_use block
	if len(msg.Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(msg.Content))
	}

	block := msg.Content[0]
	if block.OfToolUse == nil {
		t.Fatal("Expected tool_use block, got nil")
	}

	if block.OfToolUse.ID != "call_123" {
		t.Errorf("Expected call_id 'call_123', got '%s'", block.OfToolUse.ID)
	}

	if block.OfToolUse.Name != "get_weather" {
		t.Errorf("Expected tool name 'get_weather', got '%s'", block.OfToolUse.Name)
	}
}

func TestConvertResponsesToolChoiceToAnthropicBeta(t *testing.T) {
	tests := []struct {
		name     string
		tc       responses.ResponseNewParamsToolChoiceUnion
		expected anthropic.BetaToolChoiceUnionParam
	}{
		{
			name: "auto mode",
			tc: responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
			},
			expected: anthropic.BetaToolChoiceUnionParam{
				OfAuto: &anthropic.BetaToolChoiceAutoParam{},
			},
		},
		{
			name: "required mode",
			tc: responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsRequired),
			},
			expected: anthropic.BetaToolChoiceUnionParam{
				OfAny: &anthropic.BetaToolChoiceAnyParam{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertResponsesToolChoiceToAnthropicBeta(tt.tc)

			// Check if the result matches the expected type
			if tt.expected.OfAuto != nil && result.OfAuto == nil {
				t.Errorf("Expected OfAuto, got nil")
			}
			if tt.expected.OfAny != nil && result.OfAny == nil {
				t.Errorf("Expected OfAny, got nil")
			}
		})
	}
}

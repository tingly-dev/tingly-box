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

func TestConvertOpenAIResponsesToAnthropicBetaRequest_ParallelToolCalls(t *testing.T) {
	// Regression: codex re-sends parallel tool calls as consecutive function_call
	// items followed by consecutive function_call_output items; they must fold
	// into one assistant message and one user message so every tool_use is
	// answered by its tool_result in the very next message. Before the fix the
	// output was back-to-back assistant tool_use messages, rejected upstream.
	params := responses.ResponseNewParams{
		Model: "cc",
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				{
					OfMessage: &responses.EasyInputMessageParam{
						Role:    responses.EasyInputMessageRole("user"),
						Content: responses.EasyInputMessageContentUnionParam{OfString: param.NewOpt("do two things")},
					},
				},
				{
					OfFunctionCall: &responses.ResponseFunctionToolCallParam{
						CallID: "call_00_AAAA", Name: "exec", Arguments: `{"cmd":"echo hello"}`,
					},
				},
				{
					OfFunctionCall: &responses.ResponseFunctionToolCallParam{
						CallID: "call_00_BBBB", Name: "exec", Arguments: `{"cmd":"ls"}`,
					},
				},
				{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						CallID: "call_00_AAAA",
						Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{OfString: param.NewOpt("hello")},
					},
				},
				{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						CallID: "call_00_BBBB",
						Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{OfString: param.NewOpt("AGENTS.md\nCLAUDE.md")},
					},
				},
			},
		},
	}

	result := ConvertOpenAIResponsesToAnthropicBetaRequest(params, 4096)

	// user, assistant[tool_use A, tool_use B], user[tool_result A, tool_result B]
	if len(result.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result.Messages))
	}
	assistant := result.Messages[1]
	if string(assistant.Role) != "assistant" || len(assistant.Content) != 2 ||
		assistant.Content[0].OfToolUse == nil || assistant.Content[1].OfToolUse == nil {
		t.Fatalf("expected messages[1] to hold 2 tool_use blocks, got %v", assistant)
	}
	user := result.Messages[2]
	if string(user.Role) != "user" || len(user.Content) != 2 ||
		user.Content[0].OfToolResult == nil || user.Content[1].OfToolResult == nil {
		t.Fatalf("expected messages[2] to hold 2 tool_result blocks, got %v", user)
	}
}

func TestConvertOpenAIResponsesToAnthropicBetaRequest_SequentialToolCalls(t *testing.T) {
	// Sequential calls (call → result, then call → result) must stay separate:
	// each assistant tool_use message is followed by its own user tool_result.
	params := responses.ResponseNewParams{
		Model: "cc",
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				{
					OfMessage: &responses.EasyInputMessageParam{
						Role:    responses.EasyInputMessageRole("user"),
						Content: responses.EasyInputMessageContentUnionParam{OfString: param.NewOpt("run tools one at a time")},
					},
				},
				{
					OfFunctionCall: &responses.ResponseFunctionToolCallParam{
						CallID: "call_00_AAAA", Name: "exec", Arguments: `{"cmd":"echo hello"}`,
					},
				},
				{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						CallID: "call_00_AAAA",
						Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{OfString: param.NewOpt("hello")},
					},
				},
				{
					OfFunctionCall: &responses.ResponseFunctionToolCallParam{
						CallID: "call_00_BBBB", Name: "exec", Arguments: `{"cmd":"ls"}`,
					},
				},
				{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						CallID: "call_00_BBBB",
						Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{OfString: param.NewOpt("AGENTS.md\nCLAUDE.md")},
					},
				},
			},
		},
	}

	result := ConvertOpenAIResponsesToAnthropicBetaRequest(params, 4096)

	// user, assistant[tool_use A], user[tool_result A], assistant[tool_use B], user[tool_result B]
	if len(result.Messages) != 5 {
		t.Fatalf("expected 5 messages for sequential calls, got %d", len(result.Messages))
	}
	for _, i := range []int{1, 3} {
		if string(result.Messages[i].Role) != "assistant" || len(result.Messages[i].Content) != 1 {
			t.Fatalf("expected messages[%d] to be a single assistant tool_use, got %v", i, result.Messages[i])
		}
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

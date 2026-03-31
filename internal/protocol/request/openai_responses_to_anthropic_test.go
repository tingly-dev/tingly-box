package request

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

func TestConvertOpenAIResponsesToAnthropicRequest_SimpleInput(t *testing.T) {
	// Test simple string input
	params := responses.ResponseNewParams{
		Model:           "gpt-4o",
		Instructions:    param.NewOpt("You are a helpful assistant."),
		MaxOutputTokens: param.NewOpt(int64(1000)),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: param.NewOpt("Hello, how are you?"),
		},
	}

	result := ConvertOpenAIResponsesToAnthropicRequest(params, 4096)

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

func TestConvertOpenAIResponsesToAnthropicRequest_InputItems(t *testing.T) {
	// Test input with multiple messages
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

	result := ConvertOpenAIResponsesToAnthropicRequest(params, 4096)

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

func TestConvertOpenAIResponsesToAnthropicRequest_FunctionCall(t *testing.T) {
	// Test function call conversion
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

	result := ConvertOpenAIResponsesToAnthropicRequest(params, 4096)

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

func TestConvertOpenAIResponsesToAnthropicRequest_FunctionCallOutput(t *testing.T) {
	// Test function call output conversion
	params := responses.ResponseNewParams{
		Model: "gpt-4o",
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						CallID: "call_123",
						Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
							OfString: param.NewOpt("Sunny, 75°F"),
						},
						Status: "completed",
					},
				},
			},
		},
	}

	result := ConvertOpenAIResponsesToAnthropicRequest(params, 4096)

	// Verify messages
	if len(result.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(result.Messages))
	}

	msg := result.Messages[0]
	if string(msg.Role) != "user" {
		t.Errorf("Expected user role, got %s", msg.Role)
	}

	// Verify tool_result block
	if len(msg.Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(msg.Content))
	}

	block := msg.Content[0]
	if block.OfToolResult == nil {
		t.Fatal("Expected tool_result block, got nil")
	}

	if block.OfToolResult.ToolUseID != "call_123" {
		t.Errorf("Expected tool_use_id 'call_123', got '%s'", block.OfToolResult.ToolUseID)
	}
}

func TestConvertOpenAIResponsesToAnthropicRequest_Tools(t *testing.T) {
	// Test tool conversion
	params := responses.ResponseNewParams{
		Model: "gpt-4o",
		Tools: []responses.ToolUnionParam{
			{
				OfFunction: &responses.FunctionToolParam{
					Name:        "get_weather",
					Description: param.NewOpt("Get current weather"),
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type": "string",
							},
						},
						"required": []string{"location"},
					},
				},
			},
		},
		ToolChoice: responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		},
	}

	result := ConvertOpenAIResponsesToAnthropicRequest(params, 4096)

	// Verify tools
	if len(result.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(result.Tools))
	}

	tool := result.Tools[0]
	if tool.OfTool == nil {
		t.Fatal("Expected OfTool, got nil")
	}

	if tool.OfTool.Name != "get_weather" {
		t.Errorf("Expected tool name 'get_weather', got '%s'", tool.OfTool.Name)
	}

	// Verify tool choice
	if result.ToolChoice.OfAuto == nil {
		t.Error("Expected auto tool choice, got nil")
	}
}

func TestConvertOpenAIResponsesToAnthropicRequest_TemperatureAndTopP(t *testing.T) {
	// Test temperature and top_p conversion
	params := responses.ResponseNewParams{
		Model:       "gpt-4o",
		Temperature: param.NewOpt(0.7),
		TopP:        param.NewOpt(0.9),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: param.NewOpt("Hello"),
		},
	}

	result := ConvertOpenAIResponsesToAnthropicRequest(params, 4096)

	// Verify temperature
	if param.IsOmitted(result.Temperature) {
		t.Error("Expected temperature to be set, got omitted")
	} else if result.Temperature.Value != 0.7 {
		t.Errorf("Expected temperature 0.7, got %f", result.Temperature.Value)
	}

	// Verify top_p
	if param.IsOmitted(result.TopP) {
		t.Error("Expected top_p to be set, got omitted")
	} else if result.TopP.Value != 0.9 {
		t.Errorf("Expected top_p 0.9, got %f", result.TopP.Value)
	}
}

// Beta API tests

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

func TestConvertResponsesToolChoiceToAnthropic(t *testing.T) {
	tests := []struct {
		name     string
		tc       responses.ResponseNewParamsToolChoiceUnion
		expected anthropic.ToolChoiceUnionParam
	}{
		{
			name: "auto mode",
			tc: responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
			},
			expected: anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{},
			},
		},
		{
			name: "required mode",
			tc: responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsRequired),
			},
			expected: anthropic.ToolChoiceUnionParam{
				OfAny: &anthropic.ToolChoiceAnyParam{},
			},
		},
		{
			name: "none mode",
			tc: responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsNone),
			},
			expected: anthropic.ToolChoiceUnionParam{
				OfAuto: &anthropic.ToolChoiceAutoParam{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertResponsesToolChoiceToAnthropic(tt.tc)

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

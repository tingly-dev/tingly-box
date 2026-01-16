package adaptor

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

// TestConvertGoogleToOpenAIRequestComplex tests complex Google to OpenAI request conversions
func TestConvertGoogleToOpenAIRequestComplex(t *testing.T) {
	t.Run("multi-turn conversation with tools", func(t *testing.T) {
		// Simulate a complete tool calling conversation
		contents := []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					genai.NewPartFromText("What's the weather in NYC and Tokyo?"),
				},
			},
			{
				Role: "model",
				Parts: []*genai.Part{
					{
						FunctionCall: &genai.FunctionCall{
							ID:   "call_1",
							Name: "get_weather",
							Args: map[string]interface{}{"location": "NYC"},
						},
					},
					{
						FunctionCall: &genai.FunctionCall{
							ID:   "call_2",
							Name: "get_weather",
							Args: map[string]interface{}{"location": "Tokyo"},
						},
					},
				},
			},
			{
				Role: "user",
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: "call_1",
							Response: map[string]interface{}{
								"output": "Sunny, 22°C in NYC",
							},
						},
					},
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: "call_2",
							Response: map[string]interface{}{
								"output": "Rainy, 18°C in Tokyo",
							},
						},
					},
				},
			},
		}

		temp := float32(0.7)
		topP := float32(0.9)
		config := &genai.GenerateContentConfig{
			Temperature:     &temp,
			TopP:            &topP,
			MaxOutputTokens: 1000,
		}

		openaiReq := ConvertGoogleToOpenAIRequest("gemini-pro", contents, config)

		// Verify basic request structure
		assert.Equal(t, openai.ChatModel("gemini-pro"), openaiReq.Model)
		assert.InDelta(t, 0.7, openaiReq.Temperature.Value, 0.01)
		assert.InDelta(t, 0.9, openaiReq.TopP.Value, 0.01)
		assert.Equal(t, int64(1000), openaiReq.MaxTokens.Value)

		// Should have 3 messages: user, assistant (with tool calls), user (with tool results)
		assert.Len(t, openaiReq.Messages, 3)

		// First message: user query
		userMsgBytes, _ := json.Marshal(openaiReq.Messages[0])
		var userMsg map[string]interface{}
		json.Unmarshal(userMsgBytes, &userMsg)
		assert.Equal(t, "user", userMsg["role"])
		assert.Contains(t, userMsg["content"], "What's the weather")

		// Second message: assistant with tool calls
		assistantMsgBytes, _ := json.Marshal(openaiReq.Messages[1])
		var assistantMsg map[string]interface{}
		json.Unmarshal(assistantMsgBytes, &assistantMsg)
		assert.Equal(t, "assistant", assistantMsg["role"])
		toolCalls, ok := assistantMsg["tool_calls"].([]map[string]interface{})
		require.True(t, ok)
		assert.Len(t, toolCalls, 2)

		// Third message: user with tool results
		toolResultMsgBytes, _ := json.Marshal(openaiReq.Messages[2])
		var toolResultMsg map[string]interface{}
		json.Unmarshal(toolResultMsgBytes, &toolResultMsg)
		assert.Equal(t, "tool", toolResultMsg["role"])
	})

	t.Run("system instruction with complex parts", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "system",
				Parts: []*genai.Part{
					genai.NewPartFromText("You are a helpful assistant.\n"),
					genai.NewPartFromText("Always be concise and accurate."),
				},
			},
			{
				Role: "user",
				Parts: []*genai.Part{
					genai.NewPartFromText("Hello"),
				},
			},
		}

		openaiReq := ConvertGoogleToOpenAIRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		// System message should be first
		systemMsgBytes, _ := json.Marshal(openaiReq.Messages[0])
		var systemMsg map[string]interface{}
		json.Unmarshal(systemMsgBytes, &systemMsg)
		assert.Equal(t, "system", systemMsg["role"])
		assert.Contains(t, systemMsg["content"], "helpful assistant")
		assert.Contains(t, systemMsg["content"], "concise and accurate")

		// User message should be second
		assert.Len(t, openaiReq.Messages, 2)
	})

	t.Run("function response with JSON output", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: "get_data",
							Response: map[string]interface{}{
								"output": `{"temperature": 22, "condition": "sunny"}`,
							},
						},
					},
				},
			},
		}

		openaiReq := ConvertGoogleToOpenAIRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		toolResultMsgBytes, _ := json.Marshal(openaiReq.Messages[0])
		var toolResultMsg map[string]interface{}
		json.Unmarshal(toolResultMsgBytes, &toolResultMsg)

		assert.Equal(t, "tool", toolResultMsg["role"])
		assert.Contains(t, toolResultMsg["content"], "temperature")
	})

	t.Run("function response with nested data", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: "get_data",
							Response: map[string]interface{}{
								"data": map[string]interface{}{
									"temperature": 22,
									"condition":   "sunny",
									"forecast": []interface{}{
										map[string]interface{}{"day": "tomorrow", "temp": 20},
									},
								},
							},
						},
					},
				},
			},
		}

		openaiReq := ConvertGoogleToOpenAIRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		toolResultMsgBytes, _ := json.Marshal(openaiReq.Messages[0])
		var toolResultMsg map[string]interface{}
		json.Unmarshal(toolResultMsgBytes, &toolResultMsg)

		// Should marshal nested response as JSON string
		contentStr, ok := toolResultMsg["content"].(string)
		require.True(t, ok)
		var contentData map[string]interface{}
		err := json.Unmarshal([]byte(contentStr), &contentData)
		require.NoError(t, err)
		assert.Contains(t, contentData, "data")
	})
}

// TestConvertGoogleToAnthropicRequestComplex tests complex Google to Anthropic request conversions
func TestConvertGoogleToAnthropicRequestComplex(t *testing.T) {
	t.Run("multi-turn conversation with tool use and results", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					genai.NewPartFromText("Search for recent news about AI"),
				},
			},
			{
				Role: "model",
				Parts: []*genai.Part{
					{
						FunctionCall: &genai.FunctionCall{
							ID:   "toolu_123",
							Name: "search",
							Args: map[string]interface{}{"query": "AI news", "limit": 5},
						},
					},
				},
			},
			{
				Role: "user",
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: "toolu_123",
							Response: map[string]interface{}{
								"results": []interface{}{
									"Article 1", "Article 2",
								},
							},
						},
					},
				},
			},
		}

		params := ConvertGoogleToAnthropicRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		// Should have 2 messages: user, assistant (tool_use is in model response)
		// Tool results are converted to user messages with tool_result
		assert.Len(t, params.Messages, 2)

		// First message should be user
		assert.Equal(t, anthropic.MessageParamRoleUser, params.Messages[0].Role)

		// Second message should be assistant with tool_use
		assert.Equal(t, anthropic.MessageParamRoleAssistant, params.Messages[1].Role)
		assert.Len(t, params.Messages[1].Content, 1)
		assert.NotNil(t, params.Messages[1].Content[0].OfToolUse)
	})

	t.Run("complex system instruction with multiple parts", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "system",
				Parts: []*genai.Part{
					genai.NewPartFromText("You are an AI assistant.\n"),
					genai.NewPartFromText("Be helpful and accurate."),
				},
			},
		}

		params := ConvertGoogleToAnthropicRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		// System blocks should be concatenated
		require.Len(t, params.System, 1)
		assert.Contains(t, params.System[0].Text, "AI assistant")
		assert.Contains(t, params.System[0].Text, "helpful and accurate")
	})

	t.Run("assistant message with text and function call", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "model",
				Parts: []*genai.Part{
					genai.NewPartFromText("I'll search for that information."),
					{
						FunctionCall: &genai.FunctionCall{
							ID:   "toolu_456",
							Name: "search",
							Args: map[string]interface{}{"q": "test"},
						},
					},
				},
			},
		}

		params := ConvertGoogleToAnthropicRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		assert.Len(t, params.Messages, 1)
		assert.Len(t, params.Messages[0].Content, 2) // text + tool_use
		assert.NotNil(t, params.Messages[0].Content[0].OfText)
		assert.NotNil(t, params.Messages[0].Content[1].OfToolUse)
	})

	t.Run("tool result with complex JSON response", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: "toolu_789",
							Response: map[string]interface{}{
								"status": "success",
								"data": map[string]interface{}{
									"items": []interface{}{"a", "b", "c"},
									"count": 3,
								},
							},
						},
					},
				},
			},
		}

		params := ConvertGoogleToAnthropicRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		assert.Len(t, params.Messages, 1)
		assert.Equal(t, anthropic.MessageParamRoleUser, params.Messages[0].Role)
		assert.Len(t, params.Messages[0].Content, 1)
		assert.NotNil(t, params.Messages[0].Content[0].OfToolResult)
		assert.Equal(t, "toolu_789", params.Messages[0].Content[0].OfToolResult.ToolUseID)

		// Verify the content is properly JSON-encoded
		resultContent := params.Messages[0].Content[0].OfToolResult.Content
		require.Len(t, resultContent, 1)
		var resultData map[string]interface{}
		err := json.Unmarshal([]byte(resultContent[0].OfText.Text), &resultData)
		require.NoError(t, err)
		assert.Equal(t, "success", resultData["status"])
	})
}

// TestConvertGoogleToolsToOpenAIComplex tests complex tool conversions
func TestConvertGoogleToolsToOpenAIComplex(t *testing.T) {
	t.Run("multiple tools with complex schemas", func(t *testing.T) {
		funcs := []*genai.FunctionDeclaration{
			{
				Name:        "get_weather",
				Description: "Get weather information",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"location": {
							Type:        genai.TypeString,
							Description: "The location to get weather for",
						},
						"unit": {
							Type:        genai.TypeString,
							Description: "Temperature unit (celsius or fahrenheit)",
							Enum:        []string{"celsius", "fahrenheit"},
						},
					},
					Required: []string{"location"},
				},
			},
			{
				Name:        "calculate",
				Description: "Perform mathematical calculations",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"expression": {
							Type:        genai.TypeString,
							Description: "Mathematical expression to evaluate",
						},
					},
					Required: []string{"expression"},
				},
			},
		}

		tools := ConvertGoogleToolsToOpenAI(funcs)

		assert.Len(t, tools, 2)

		// Check first tool
		firstTool := tools[0].GetFunction()
		require.NotNil(t, firstTool)
		assert.Equal(t, "get_weather", firstTool.Name)
		assert.Equal(t, "Get weather information", firstTool.Description.Value)

		// Verify parameters are properly converted
		require.NotNil(t, firstTool.Parameters)
		paramsBytes, _ := json.Marshal(firstTool.Parameters)
		var params map[string]interface{}
		json.Unmarshal(paramsBytes, &params)
		assert.Equal(t, "object", params["type"])

		props, ok := params["properties"].(map[string]interface{})
		require.True(t, ok)
		assert.Contains(t, props, "location")
		assert.Contains(t, props, "unit")
	})

	t.Run("nested schema properties", func(t *testing.T) {
		funcs := []*genai.FunctionDeclaration{
			{
				Name:        "process_data",
				Description: "Process complex data structures",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"items": {
							Type: genai.TypeArray,
							Items: &genai.Schema{
								Type: genai.TypeObject,
								Properties: map[string]*genai.Schema{
									"id":    {Type: genai.TypeString},
									"value": {Type: genai.TypeNumber},
								},
							},
						},
					},
				},
			},
		}

		tools := ConvertGoogleToolsToOpenAI(funcs)

		require.Len(t, tools, 1)
		fn := tools[0].GetFunction()
		require.NotNil(t, fn)

		paramsBytes, _ := json.Marshal(fn.Parameters)
		var params map[string]interface{}
		json.Unmarshal(paramsBytes, &params)
		props, ok := params["properties"].(map[string]interface{})
		require.True(t, ok)

		items, ok := props["items"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "array", items["type"])
		assert.Contains(t, items, "items")
	})
}

// TestConvertGoogleToolsToAnthropicComplex tests complex tool conversions to Anthropic format
func TestConvertGoogleToolsToAnthropicComplex(t *testing.T) {
	t.Run("complex nested schema with anyOf", func(t *testing.T) {
		funcs := []*genai.FunctionDeclaration{
			{
				Name:        "flexible_search",
				Description: "Search with flexible parameters",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"query": {
							Type:        genai.TypeString,
							Description: "Search query",
						},
						"filters": {
							Type: genai.TypeObject,
							Properties: map[string]*genai.Schema{
								"category": {Type: genai.TypeString},
								"date":     {Type: genai.TypeString},
							},
						},
					},
				},
			},
		}

		tools := ConvertGoogleToolsToAnthropic(funcs)

		require.Len(t, tools, 1)
		tool := tools[0].OfTool
		require.NotNil(t, tool)

		assert.Equal(t, "flexible_search", tool.Name)
		assert.Equal(t, "Search with flexible parameters", tool.Description.Value)

		// Verify input schema
		require.NotNil(t, tool.InputSchema.Properties)
		assert.Contains(t, tool.InputSchema.Properties, "query")
		assert.Contains(t, tool.InputSchema.Properties, "filters")
	})
}

// TestConvertGoogleToolChoice tests tool choice conversion
func TestConvertGoogleToolChoice(t *testing.T) {
	t.Run("auto mode", func(t *testing.T) {
		config := &genai.FunctionCallingConfig{
			Mode: genai.FunctionCallingConfigModeAuto,
		}

		openaiChoice := ConvertGoogleToolChoiceToOpenAI(config)
		anthropicChoice := ConvertGoogleToolChoiceToAnthropic(config)

		// Both should map to auto
		assert.Equal(t, "auto", openaiChoice.OfAuto.Value)
		assert.NotNil(t, anthropicChoice.OfAuto)
	})

	t.Run("any mode with specific function", func(t *testing.T) {
		config := &genai.FunctionCallingConfig{
			Mode:                 genai.FunctionCallingConfigModeAny,
			AllowedFunctionNames: []string{"get_weather"},
		}

		openaiChoice := ConvertGoogleToolChoiceToOpenAI(config)
		anthropicChoice := ConvertGoogleToolChoiceToAnthropic(config)

		// OpenAI should have specific tool
		assert.NotNil(t, openaiChoice.OfFunctionToolChoice)
		assert.Equal(t, "get_weather", openaiChoice.OfFunctionToolChoice.Function.Name)

		// Anthropic should have specific tool
		assert.Equal(t, "get_weather", anthropicChoice.OfTool.Name)
	})

	t.Run("none mode", func(t *testing.T) {
		config := &genai.FunctionCallingConfig{
			Mode: genai.FunctionCallingConfigModeNone,
		}

		openaiChoice := ConvertGoogleToolChoiceToOpenAI(config)
		anthropicChoice := ConvertGoogleToolChoiceToAnthropic(config)

		// Both should map to auto (since there's no direct "none" in their tool choice)
		assert.Equal(t, "auto", openaiChoice.OfAuto.Value)
		assert.NotNil(t, anthropicChoice.OfAuto)
	})
}

// TestConvertGooglePartsToStringComplex tests parts to string conversion with complex scenarios
func TestConvertGooglePartsToStringComplex(t *testing.T) {
	t.Run("mixed content with function calls", func(t *testing.T) {
		parts := []*genai.Part{
			genai.NewPartFromText("Let me check the weather"),
			{FunctionCall: &genai.FunctionCall{Name: "get_weather"}},
			genai.NewPartFromText(" for you."),
		}

		result := ConvertGooglePartsToString(parts)
		assert.Equal(t, "Let me check the weather for you.", result)
	})

	t.Run("only function calls", func(t *testing.T) {
		parts := []*genai.Part{
			{FunctionCall: &genai.FunctionCall{Name: "func1"}},
			{FunctionCall: &genai.FunctionCall{Name: "func2"}},
		}

		result := ConvertGooglePartsToString(parts)
		assert.Equal(t, "", result)
	})
}

// TestConvertGoogleToOpenAIRequest tests converting Google request to OpenAI format
func TestConvertGoogleToOpenAIRequest(t *testing.T) {
	t.Run("simple user content", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					genai.NewPartFromText("Hello, world!"),
				},
			},
		}

		openaiReq := ConvertGoogleToOpenAIRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		if string(openaiReq.Model) != "gemini-pro" {
			t.Errorf("expected model 'gemini-pro', got '%s'", openaiReq.Model)
		}
		if len(openaiReq.Messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(openaiReq.Messages))
		}
	})

	t.Run("with model role", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "model",
				Parts: []*genai.Part{
					genai.NewPartFromText("Hi there!"),
				},
			},
		}

		openaiReq := ConvertGoogleToOpenAIRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		if len(openaiReq.Messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(openaiReq.Messages))
		}
	})

	t.Run("with temperature", func(t *testing.T) {
		temp := float32(0.7)
		config := &genai.GenerateContentConfig{
			Temperature: &temp,
		}

		openaiReq := ConvertGoogleToOpenAIRequest("gemini-pro", nil, config)

		if openaiReq.Temperature.Value < 0.69 || openaiReq.Temperature.Value > 0.71 {
			t.Errorf("expected temperature ~0.7, got %f", openaiReq.Temperature.Value)
		}
	})

	t.Run("with function calls", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "model",
				Parts: []*genai.Part{
					{
						FunctionCall: &genai.FunctionCall{
							ID:   "call_123",
							Name: "get_weather",
							Args: map[string]interface{}{"loc": "NYC"},
						},
					},
				},
			},
		}

		openaiReq := ConvertGoogleToOpenAIRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		if len(openaiReq.Messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(openaiReq.Messages))
		}
	})
}

// TestConvertGoogleToAnthropicRequest tests converting Google request to Anthropic format
func TestConvertGoogleToAnthropicRequest(t *testing.T) {
	t.Run("simple user content", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					genai.NewPartFromText("Hello, world!"),
				},
			},
		}

		params := ConvertGoogleToAnthropicRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		if string(params.Model) != "gemini-pro" {
			t.Errorf("expected model 'gemini-pro', got '%s'", params.Model)
		}
		if len(params.Messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(params.Messages))
		}
	})

	t.Run("with system instruction", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "system",
				Parts: []*genai.Part{
					genai.NewPartFromText("You are helpful"),
				},
			},
		}

		params := ConvertGoogleToAnthropicRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		if len(params.System) != 1 {
			t.Errorf("expected 1 system block, got %d", len(params.System))
		}
	})

	t.Run("with function call", func(t *testing.T) {
		contents := []*genai.Content{
			{
				Role: "model",
				Parts: []*genai.Part{
					{
						FunctionCall: &genai.FunctionCall{
							ID:   "call_123",
							Name: "get_weather",
							Args: map[string]interface{}{"loc": "NYC"},
						},
					},
				},
			},
		}

		params := ConvertGoogleToAnthropicRequest("gemini-pro", contents, &genai.GenerateContentConfig{})

		if len(params.Messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(params.Messages))
		}
	})
}

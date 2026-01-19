package adaptor

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

// TestConvertGoogleToOpenAIResponseComplex tests complex Google to OpenAI response conversions
func TestConvertGoogleToOpenAIResponseComplex(t *testing.T) {
	t.Run("response with text and multiple function calls", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							genai.NewPartFromText("I'll check the weather for both cities."),
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
					FinishReason: genai.FinishReasonStop,
				},
			},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     20,
				CandidatesTokenCount: 30,
				TotalTokenCount:      50,
			},
		}

		result := ConvertGoogleToOpenAIResponse(resp, "gemini-pro")

		// Check basic structure
		assert.Equal(t, "gemini-pro", result["model"])
		assert.Equal(t, "chat.completion", result["object"])

		// Check choices
		choices := result["choices"].([]map[string]interface{})
		require.Len(t, choices, 1)

		message := choices[0]["message"].(map[string]interface{})
		assert.Equal(t, "assistant", message["role"])
		assert.Contains(t, message["content"], "check the weather")

		// Check tool calls
		toolCalls, ok := message["tool_calls"].([]map[string]interface{})
		require.True(t, ok)
		assert.Len(t, toolCalls, 2)

		// Verify first tool call
		assert.Equal(t, "call_1", toolCalls[0]["id"])
		fn1 := toolCalls[0]["function"].(map[string]interface{})
		assert.Equal(t, "get_weather", fn1["name"])
		assert.Contains(t, fn1["arguments"], "NYC")

		// Verify second tool call
		assert.Equal(t, "call_2", toolCalls[1]["id"])

		// Check usage
		usage := result["usage"].(map[string]interface{})
		assert.Equal(t, int32(20), usage["prompt_tokens"])
		assert.Equal(t, int32(30), usage["completion_tokens"])
		assert.Equal(t, int32(50), usage["total_tokens"])
	})

	t.Run("function call with complex nested arguments", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							{
								FunctionCall: &genai.FunctionCall{
									ID:   "call_complex",
									Name: "process_data",
									Args: map[string]interface{}{
										"query": map[string]interface{}{
											"filters": []interface{}{
												map[string]interface{}{
													"field": "category",
													"value": "tech",
												},
											},
											"limit": 10,
										},
										"options": map[string]interface{}{
											"verbose": true,
											"format":  "json",
										},
									},
								},
							},
						},
					},
					FinishReason: genai.FinishReasonStop,
				},
			},
		}

		result := ConvertGoogleToOpenAIResponse(resp, "gemini-pro")

		choices := result["choices"].([]map[string]interface{})
		message := choices[0]["message"].(map[string]interface{})
		toolCalls := message["tool_calls"].([]map[string]interface{})

		require.Len(t, toolCalls, 1)
		fn := toolCalls[0]["function"].(map[string]interface{})

		// Arguments should be JSON string
		argsStr, ok := fn["arguments"].(string)
		require.True(t, ok)

		var args map[string]interface{}
		err := json.Unmarshal([]byte(argsStr), &args)
		require.NoError(t, err)

		// Verify nested structure is preserved
		assert.Contains(t, args, "query")
		assert.Contains(t, args, "options")
	})

	t.Run("max tokens finish reason", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role:  "model",
						Parts: []*genai.Part{genai.NewPartFromText("Partial response...")},
					},
					FinishReason: genai.FinishReasonMaxTokens,
				},
			},
		}

		result := ConvertGoogleToOpenAIResponse(resp, "gemini-pro")

		choices := result["choices"].([]map[string]interface{})
		assert.Equal(t, "length", choices[0]["finish_reason"])
	})

	t.Run("safety finish reason", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role:  "model",
						Parts: []*genai.Part{},
					},
					FinishReason: genai.FinishReasonSafety,
				},
			},
		}

		result := ConvertGoogleToOpenAIResponse(resp, "gemini-pro")

		choices := result["choices"].([]map[string]interface{})
		assert.Equal(t, "content_filter", choices[0]["finish_reason"])
	})

	t.Run("nil response", func(t *testing.T) {
		result := ConvertGoogleToOpenAIResponse(nil, "gemini-pro")
		assert.Nil(t, result)
	})

	t.Run("empty candidates", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{},
		}

		result := ConvertGoogleToOpenAIResponse(resp, "gemini-pro")

		// Should still return a valid response structure
		assert.NotNil(t, result)
		assert.Equal(t, "gemini-pro", result["model"])
		choices := result["choices"].([]map[string]interface{})
		assert.Len(t, choices, 1)
		assert.Equal(t, "stop", choices[0]["finish_reason"])
	})
}

// TestConvertGoogleToAnthropicResponseComplex tests complex Google to Anthropic response conversions
func TestConvertGoogleToAnthropicResponseComplex(t *testing.T) {
	t.Run("response with text and tool use", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							genai.NewPartFromText("I'll search for that."),
							{
								FunctionCall: &genai.FunctionCall{
									ID:   "toolu_123",
									Name: "search",
									Args: map[string]interface{}{
										"query": "latest AI news",
										"limit": 5,
									},
								},
							},
						},
					},
					FinishReason: genai.FinishReasonStop,
				},
			},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     15,
				CandidatesTokenCount: 25,
				TotalTokenCount:      40,
			},
		}

		result := ConvertGoogleToAnthropicResponse(resp, "gemini-pro")

		// Check basic structure
		assert.Equal(t, "assistant", result.Role)
		assert.Equal(t, "message", result.Type)
		assert.Equal(t, "gemini-pro", result.Model)

		// Should have 2 content blocks: text and tool_use
		assert.Len(t, result.Content, 2)

		// First block: text
		assert.Equal(t, "text", result.Content[0].Type)
		assert.Contains(t, result.Content[0].Text, "search for that")

		// Second block: tool_use
		assert.Equal(t, "tool_use", result.Content[1].Type)
		assert.Equal(t, "toolu_123", result.Content[1].ID)
		assert.Equal(t, "search", result.Content[1].Name)

		// Verify tool arguments
		var args map[string]interface{}
		err := json.Unmarshal(result.Content[1].Input, &args)
		require.NoError(t, err)
		assert.Equal(t, "latest AI news", args["query"])
		assert.Equal(t, float64(5), args["limit"])

		// Check stop reason
		assert.Equal(t, "end_turn", result.StopReason)

		// Check usage
		assert.Equal(t, int64(15), result.Usage.InputTokens)
		assert.Equal(t, int64(25), result.Usage.OutputTokens)
	})

	t.Run("multiple tool uses in single response", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							{
								FunctionCall: &genai.FunctionCall{
									ID:   "toolu_1",
									Name: "get_weather",
									Args: map[string]interface{}{"city": "NYC"},
								},
							},
							{
								FunctionCall: &genai.FunctionCall{
									ID:   "toolu_2",
									Name: "get_weather",
									Args: map[string]interface{}{"city": "Tokyo"},
								},
							},
							{
								FunctionCall: &genai.FunctionCall{
									ID:   "toolu_3",
									Name: "get_weather",
									Args: map[string]interface{}{"city": "London"},
								},
							},
						},
					},
					FinishReason: genai.FinishReasonStop,
				},
			},
		}

		result := ConvertGoogleToAnthropicResponse(resp, "gemini-pro")

		assert.Len(t, result.Content, 3)

		// Verify all tool uses
		for i, block := range result.Content {
			assert.Equal(t, "tool_use", block.Type)
			assert.Equal(t, "get_weather", block.Name)
			assert.Equal(t, []string{"toolu_1", "toolu_2", "toolu_3"}[i], block.ID)
		}
	})

	t.Run("max tokens stop reason", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role:  "model",
						Parts: []*genai.Part{genai.NewPartFromText("...")},
					},
					FinishReason: genai.FinishReasonMaxTokens,
				},
			},
		}

		result := ConvertGoogleToAnthropicResponse(resp, "gemini-pro")
		assert.Equal(t, "max_tokens", result.StopReason)
	})

	t.Run("safety stop reason", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role:  "model",
						Parts: []*genai.Part{},
					},
					FinishReason: genai.FinishReasonSafety,
				},
			},
		}

		result := ConvertGoogleToAnthropicResponse(resp, "gemini-pro")
		assert.Equal(t, "content_filter", result.StopReason)
	})

	t.Run("complex nested tool arguments", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							{
								FunctionCall: &genai.FunctionCall{
									ID:   "toolu_complex",
									Name: "complex_query",
									Args: map[string]interface{}{
										"filters": []interface{}{
											map[string]interface{}{
												"field":    "status",
												"operator": "=",
												"value":    "active",
											},
										},
										"sort": []interface{}{
											map[string]interface{}{
												"field":     "created",
												"direction": "desc",
											},
										},
										"pagination": map[string]interface{}{
											"page": 1,
											"size": 20,
										},
									},
								},
							},
						},
					},
					FinishReason: genai.FinishReasonStop,
				},
			},
		}

		result := ConvertGoogleToAnthropicResponse(resp, "gemini-pro")

		require.Len(t, result.Content, 1)
		block := result.Content[0]

		assert.Equal(t, "tool_use", block.Type)
		assert.Equal(t, "complex_query", block.Name)

		var args map[string]interface{}
		err := json.Unmarshal(block.Input, &args)
		require.NoError(t, err)

		// Verify complex nested structure
		assert.Contains(t, args, "filters")
		assert.Contains(t, args, "sort")
		assert.Contains(t, args, "pagination")

		filters, ok := args["filters"].([]interface{})
		require.True(t, ok)
		require.Len(t, filters, 1)
	})

	t.Run("nil response", func(t *testing.T) {
		result := ConvertGoogleToAnthropicResponse(nil, "gemini-pro")
		assert.Equal(t, anthropic.Message{}, result)
	})
}

// TestConvertGoogleToAnthropicBetaResponseComplex tests complex Google to Anthropic beta response conversions
func TestConvertGoogleToAnthropicBetaResponseComplex(t *testing.T) {
	t.Run("complex tool use with text", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							genai.NewPartFromText("Processing your request..."),
							{
								FunctionCall: &genai.FunctionCall{
									ID:   "toolu_beta_1",
									Name: "process",
									Args: map[string]interface{}{
										"data": []interface{}{"a", "b", "c"},
										"mode": "batch",
									},
								},
							},
						},
					},
					FinishReason: genai.FinishReasonStop,
				},
			},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 20,
				TotalTokenCount:      30,
			},
		}

		result := ConvertGoogleToAnthropicBetaResponse(resp, "gemini-pro")

		// Check basic structure
		assert.Equal(t, "assistant", result.Role)
		assert.Equal(t, "message", result.Type)

		// Should have 2 content blocks
		assert.Len(t, result.Content, 2)

		// First block: text
		assert.Equal(t, "text", result.Content[0].Type)
		assert.Contains(t, result.Content[0].Text, "Processing")

		// Second block: tool_use
		assert.Equal(t, "tool_use", result.Content[1].Type)
		assert.Equal(t, "toolu_beta_1", result.Content[1].ID)
		assert.Equal(t, "process", result.Content[1].Name)

		// Verify arguments
		var args map[string]interface{}
		err := json.Unmarshal([]byte(result.Content[1].Input), &args)
		require.NoError(t, err)
		assert.Equal(t, "batch", args["mode"])
	})

	t.Run("finish reason mapping to beta format", func(t *testing.T) {
		tests := []struct {
			name         string
			finishReason genai.FinishReason
			expected     string
		}{
			{"stop", genai.FinishReasonStop, "end_turn"},
			{"max_tokens", genai.FinishReasonMaxTokens, "max_tokens"},
			{"safety", genai.FinishReasonSafety, "refusal"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				resp := &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{
						{
							Content:      &genai.Content{Role: "model", Parts: []*genai.Part{}},
							FinishReason: tt.finishReason,
						},
					},
				}

				result := ConvertGoogleToAnthropicBetaResponse(resp, "gemini-pro")
				assert.Equal(t, tt.expected, string(result.StopReason))
			})
		}
	})
}

// TestGoogleFinishReasonMapping tests finish reason mapping functions
func TestGoogleFinishReasonMapping(t *testing.T) {
	t.Run("mapGoogleFinishReasonToOpenAI", func(t *testing.T) {
		tests := []struct {
			input    genai.FinishReason
			expected string
		}{
			{genai.FinishReasonStop, "stop"},
			{genai.FinishReasonMaxTokens, "length"},
			{genai.FinishReasonSafety, "content_filter"},
			{genai.FinishReasonRecitation, "stop"},
			{genai.FinishReasonLanguage, "stop"},
			{genai.FinishReasonOther, "stop"},
		}

		for _, tt := range tests {
			t.Run(string(tt.input), func(t *testing.T) {
				result := mapGoogleFinishReasonToOpenAI(tt.input)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("mapGoogleFinishReasonToAnthropic", func(t *testing.T) {
		tests := []struct {
			input    genai.FinishReason
			expected string
		}{
			{genai.FinishReasonStop, "end_turn"},
			{genai.FinishReasonMaxTokens, "max_tokens"},
			{genai.FinishReasonSafety, "content_filter"},
			{genai.FinishReasonRecitation, "end_turn"},
			{genai.FinishReasonOther, "end_turn"},
		}

		for _, tt := range tests {
			t.Run(string(tt.input), func(t *testing.T) {
				result := mapGoogleFinishReasonToAnthropic(tt.input)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("mapGoogleFinishReasonToAnthropicBeta", func(t *testing.T) {
		tests := []struct {
			input    genai.FinishReason
			expected string
		}{
			{genai.FinishReasonStop, "end_turn"},
			{genai.FinishReasonMaxTokens, "max_tokens"},
			{genai.FinishReasonSafety, "refusal"},
			{genai.FinishReasonOther, "end_turn"},
		}

		for _, tt := range tests {
			t.Run(string(tt.input), func(t *testing.T) {
				result := mapGoogleFinishReasonToAnthropicBeta(tt.input)
				assert.Equal(t, tt.expected, string(result))
			})
		}
	})
}

// TestConvertGoogleToOpenAIResponse tests converting Google response to OpenAI format
func TestConvertGoogleToOpenAIResponse(t *testing.T) {
	t.Run("text only response", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							genai.NewPartFromText("Hello!"),
						},
					},
					FinishReason: genai.FinishReasonStop,
				},
			},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
				TotalTokenCount:      15,
			},
		}

		openaiResp := ConvertGoogleToOpenAIResponse(resp, "gemini-pro")

		if openaiResp["model"] != "gemini-pro" {
			t.Errorf("expected model 'gemini-pro', got '%v'", openaiResp["model"])
		}
		choices := openaiResp["choices"].([]map[string]interface{})
		if len(choices) != 1 {
			t.Errorf("expected 1 choice, got %d", len(choices))
		}
		if choices[0]["message"].(map[string]interface{})["content"] != "Hello!" {
			t.Errorf("expected content 'Hello!', got '%v'", choices[0]["message"].(map[string]interface{})["content"])
		}
	})

	t.Run("with function call", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
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
					FinishReason: genai.FinishReasonStop,
				},
			},
		}

		openaiResp := ConvertGoogleToOpenAIResponse(resp, "gemini-pro")

		choices := openaiResp["choices"].([]map[string]interface{})
		toolCalls := choices[0]["message"].(map[string]interface{})["tool_calls"].([]map[string]interface{})
		if len(toolCalls) != 1 {
			t.Errorf("expected 1 tool call, got %d", len(toolCalls))
		}
		if toolCalls[0]["function"].(map[string]interface{})["name"] != "get_weather" {
			t.Errorf("expected function name 'get_weather', got '%v'", toolCalls[0]["function"].(map[string]interface{})["name"])
		}
	})
}

// TestConvertGoogleToAnthropicResponse tests converting Google response to Anthropic format
func TestConvertGoogleToAnthropicResponse(t *testing.T) {
	t.Run("text only response", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role: "model",
						Parts: []*genai.Part{
							genai.NewPartFromText("Hello!"),
						},
					},
					FinishReason: genai.FinishReasonStop,
				},
			},
			UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
			},
		}

		anthropicResp := ConvertGoogleToAnthropicResponse(resp, "gemini-pro")

		if anthropicResp.Role != "assistant" {
			t.Errorf("expected role 'assistant', got '%s'", anthropicResp.Role)
		}
		if len(anthropicResp.Content) != 1 {
			t.Errorf("expected 1 content block, got %d", len(anthropicResp.Content))
		}
		if anthropicResp.Content[0].Type != "text" {
			t.Errorf("expected type 'text', got '%s'", anthropicResp.Content[0].Type)
		}
		if anthropicResp.Content[0].Text != "Hello!" {
			t.Errorf("expected text 'Hello!', got '%s'", anthropicResp.Content[0].Text)
		}
		if anthropicResp.StopReason != "end_turn" {
			t.Errorf("expected stop reason 'end_turn', got '%s'", anthropicResp.StopReason)
		}
	})

	t.Run("with function call", func(t *testing.T) {
		resp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
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
					FinishReason: genai.FinishReasonStop,
				},
			},
		}

		anthropicResp := ConvertGoogleToAnthropicResponse(resp, "gemini-pro")

		if len(anthropicResp.Content) != 1 {
			t.Errorf("expected 1 content block, got %d", len(anthropicResp.Content))
		}
		if anthropicResp.Content[0].Type != "tool_use" {
			t.Errorf("expected type 'tool_use', got '%s'", anthropicResp.Content[0].Type)
		}
		if anthropicResp.Content[0].Name != "get_weather" {
			t.Errorf("expected name 'get_weather', got '%s'", anthropicResp.Content[0].Name)
		}
	})
}

// TestConvertGoogleToolsToOpenAI tests converting Google tools to OpenAI format
func TestConvertGoogleToolsToOpenAI(t *testing.T) {
	t.Run("single tool", func(t *testing.T) {
		funcs := []*genai.FunctionDeclaration{
			{
				Name:        "get_weather",
				Description: "Get weather info",
				Parameters: &genai.Schema{
					Type: "object",
				},
			},
		}

		tools := ConvertGoogleToolsToOpenAI(funcs)

		if len(tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(tools))
		}
	})

	t.Run("nil tools", func(t *testing.T) {
		tools := ConvertGoogleToolsToOpenAI(nil)
		if tools != nil {
			t.Errorf("expected nil, got %v", tools)
		}
	})
}

// TestConvertGoogleToolsToAnthropic tests converting Google tools to Anthropic format
func TestConvertGoogleToolsToAnthropic(t *testing.T) {
	t.Run("single tool", func(t *testing.T) {
		funcs := []*genai.FunctionDeclaration{
			{
				Name:        "get_weather",
				Description: "Get weather info",
				Parameters: &genai.Schema{
					Type: "object",
				},
			},
		}

		tools := ConvertGoogleToolsToAnthropic(funcs)

		if len(tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(tools))
		}
	})

	t.Run("nil tools", func(t *testing.T) {
		tools := ConvertGoogleToolsToAnthropic(nil)
		if tools != nil {
			t.Errorf("expected nil, got %v", tools)
		}
	})
}

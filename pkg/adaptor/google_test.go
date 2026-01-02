package adaptor

import (
	"context"
	"os"
	"testing"
	client2 "tingly-box/pkg/client"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"google.golang.org/genai"
)

// TestGoogleGenerateContent tests calling Google genai API directly to generate content.
// Based on Vertex AI / Gemini API pattern.
// Set GOOGLE_API_KEY and GOOGLE_MODEL environment variables to run this test.
func TestGoogleGenerateContent(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	model := os.Getenv("GOOGLE_MODEL") // e.g., "gemini-2.5-flash", "gemini-2.0-flash-exp"

	if apiKey == "" || model == "" {
		t.Skip("Skipping test: GOOGLE_API_KEY and GOOGLE_MODEL must be set")
	}

	// Create Google client
	ctx := context.Background()
	client, err := genai.NewClient(
		ctx,
		&genai.ClientConfig{
			APIKey:     apiKey,
			HTTPClient: client2.CreateHTTPClientWithProxy(os.Getenv("HTTPS_PROXY")),
			HTTPOptions: genai.HTTPOptions{
				BaseURL:    os.Getenv("GOOGLE_API_URL"),
				APIVersion: os.Getenv("GOOGLE_API_VERSION"),
			},
		},
	)
	if err != nil {
		t.Fatalf("Failed to create Google client: %v", err)
	}

	// Prepare content for generation
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				genai.NewPartFromText("What is capital of France?"),
			},
		},
	}

	// Generation config
	config := &genai.GenerateContentConfig{
		MaxOutputTokens: 1000,
	}
	temp := float32(0.7)
	config.Temperature = &temp

	// System instruction
	config.SystemInstruction = &genai.Content{
		Role: "system",
		Parts: []*genai.Part{
			genai.NewPartFromText("You are a helpful assistant."),
		},
	}

	t.Logf("Calling Google API with model: %s", model)
	t.Logf("Config - MaxOutputTokens: %d, Temperature: %f", config.MaxOutputTokens, *config.Temperature)

	// Call GenerateContent
	// Note: The actual API call method signature depends on SDK version
	// This test focuses on request preparation
	// To make actual API call, uncomment and adjust based on SDK:
	resp, err := client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		t.Fatalf("Failed to generate content: %v", err)
	}

	// Verify response
	if resp == nil {
		t.Fatal("Response should not be nil")
	}

	t.Logf("Response received - Candidates: %d", len(resp.Candidates))

	if len(resp.Candidates) == 0 {
		t.Error("Expected at least one candidate")
		return
	}

	// Check content
	candidate := resp.Candidates[0]
	if candidate.Content != nil {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				t.Logf("Generated text: %s", part.Text)
			}
		}
	}

	// Check usage
	if resp.UsageMetadata != nil {
		t.Logf("Usage - Prompt: %d, Candidates: %d, Total: %d",
			resp.UsageMetadata.PromptTokenCount,
			resp.UsageMetadata.CandidatesTokenCount,
			resp.UsageMetadata.TotalTokenCount)
	}

	// Verify request preparation was successful
	if len(contents) == 0 {
		t.Error("Expected at least one content item")
	}
	if config.MaxOutputTokens != 1000 {
		t.Errorf("Expected MaxOutputTokens 1000, got %d", config.MaxOutputTokens)
	}
	if config.SystemInstruction == nil {
		t.Error("Expected system instruction to be set")
	}

	t.Log("Request preparation successful - API call commented out")
}

// TestOpenAIToGoogleWithRealAPI tests converting OpenAI request to Google format.
// This test verifies request conversion logic only.
// To test actual API calls, set up Google client separately.
func TestOpenAIToGoogleWithRealAPI(t *testing.T) {
	// Check if environment variables are set
	apiKey := os.Getenv("GOOGLE_API_KEY")
	model := os.Getenv("GOOGLE_MODEL") // e.g., "gemini-1.5-flash", "gemini-1.5-pro"

	if apiKey == "" || model == "" {
		t.Skip("Skipping test: GOOGLE_API_KEY and GOOGLE_MODEL must be set")
	}

	// Create OpenAI format request
	openaiReq := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel("gpt-4"),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are a helpful assistant."),
			openai.UserMessage("What is the capital of France?"),
		},
		MaxTokens: openai.Opt[int64](100),
	}

	// Convert to Google format
	googleModel, contents, config := ConvertOpenAIToGoogleRequest(openaiReq, 4096)

	t.Logf("Converted request - Model: %s, Contents: %d", googleModel, len(contents))
	t.Logf("Config - MaxOutputTokens: %d", config.MaxOutputTokens)
	if config.Temperature != nil {
		t.Logf("Config - Temperature: %f", *config.Temperature)
	}

	// Verify conversion was successful
	if googleModel != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", googleModel)
	}

	if len(contents) == 0 {
		t.Error("Expected at least one content item")
	}

	// Check system instruction
	if config.SystemInstruction != nil {
		t.Logf("System instruction: %s", config.SystemInstruction.Parts[0].Text)
	}

	// Note: Actual API call would require Google client initialization.
	// This test validates that conversion logic which is adaptor's responsibility.
	/*
		ctx := context.Background()
		client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
		if err != nil {
			t.Fatalf("Failed to create Google client: %v", err)
		}
		// resp, err := client.Models.GenerateContent(ctx, model, config, contents)
		if err != nil {
			t.Fatalf("Failed to call Google API: %v", err)
		}
		// Process response...
	*/
}

// TestConvertOpenAIToGoogleRequest tests converting OpenAI request to Google format
func TestConvertOpenAIToGoogleRequest(t *testing.T) {
	t.Run("simple user message", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Hello, world!"),
			},
		}

		model, contents, config := ConvertOpenAIToGoogleRequest(req, 4096)

		if model != "gpt-4" {
			t.Errorf("expected model 'gpt-4', got '%s'", model)
		}
		if len(contents) != 1 {
			t.Errorf("expected 1 content, got %d", len(contents))
		}
		if contents[0].Role != "user" {
			t.Errorf("expected role 'user', got '%s'", contents[0].Role)
		}
		if len(contents[0].Parts) != 1 {
			t.Errorf("expected 1 part, got %d", len(contents[0].Parts))
		}
		if contents[0].Parts[0].Text != "Hello, world!" {
			t.Errorf("expected text 'Hello, world!', got '%s'", contents[0].Parts[0].Text)
		}
		if config.MaxOutputTokens != 4096 {
			t.Errorf("expected MaxOutputTokens 4096, got %d", config.MaxOutputTokens)
		}
	})

	t.Run("with system message", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage("You are a helpful assistant"),
				openai.UserMessage("Hello"),
			},
		}

		_, _, config := ConvertOpenAIToGoogleRequest(req, 4096)

		if config.SystemInstruction == nil {
			t.Error("expected system instruction")
		} else if config.SystemInstruction.Parts[0].Text != "You are a helpful assistant\n" {
			t.Errorf("unexpected system instruction: %q", config.SystemInstruction.Parts[0].Text)
		}
	})

	t.Run("with tool calls", func(t *testing.T) {
		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Get weather"),
				openai.AssistantMessage("I'll check the weather"),
			},
		}

		_, contents, _ := ConvertOpenAIToGoogleRequest(req, 4096)

		if len(contents) != 2 {
			t.Errorf("expected 2 contents, got %d", len(contents))
		}
		if contents[1].Role != "model" {
			t.Errorf("expected role 'model', got '%s'", contents[1].Role)
		}
	})

	t.Run("with tools", func(t *testing.T) {
		tool := openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "get_weather",
			Description: param.Opt[string]{Value: "Get weather"},
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"location": map[string]interface{}{
						"type": "string",
					},
				},
			},
		})

		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Get weather"),
			},
			Tools: []openai.ChatCompletionToolUnionParam{tool},
		}

		_, _, config := ConvertOpenAIToGoogleRequest(req, 4096)

		if config.Tools == nil {
			t.Error("expected tools")
		} else if len(config.Tools) == 0 {
			t.Error("expected tools to have function declarations")
		}
	})

	t.Run("with temperature", func(t *testing.T) {
		temp := float64(0.7)
		req := &openai.ChatCompletionNewParams{
			Model:       openai.ChatModel("gpt-4"),
			Messages:    []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello")},
			Temperature: openai.Opt(temp),
		}

		_, _, config := ConvertOpenAIToGoogleRequest(req, 4096)

		if config.Temperature == nil {
			t.Error("expected temperature to be set")
		} else if *config.Temperature != float32(0.7) {
			t.Errorf("expected temperature 0.7, got %f", *config.Temperature)
		}
	})
}

// TestConvertAnthropicToGoogleRequest tests converting Anthropic request to Google format
func TestConvertAnthropicToGoogleRequest(t *testing.T) {
	t.Run("simple user message", func(t *testing.T) {
		req := &anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-3"),
			MaxTokens: 4096,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Hello, world!")),
			},
		}

		model, contents, _ := ConvertAnthropicToGoogleRequest(req, 4096)

		if model != "claude-3" {
			t.Errorf("expected model 'claude-3', got '%s'", model)
		}
		if len(contents) != 1 {
			t.Errorf("expected 1 content, got %d", len(contents))
		}
		if contents[0].Role != "user" {
			t.Errorf("expected role 'user', got '%s'", contents[0].Role)
		}
	})

	t.Run("with system instruction", func(t *testing.T) {
		req := &anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-3"),
			MaxTokens: 4096,
			System: []anthropic.TextBlockParam{
				{Text: "You are helpful"},
			},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
			},
		}

		_, _, config := ConvertAnthropicToGoogleRequest(req, 4096)

		if config.SystemInstruction == nil {
			t.Error("expected system instruction")
		}
	})

	t.Run("with tool use", func(t *testing.T) {
		req := &anthropic.MessageNewParams{
			Model:     anthropic.Model("claude-3"),
			MaxTokens: 4096,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Use tool")),
				anthropic.NewAssistantMessage(
					anthropic.NewToolUseBlock("call_123", map[string]interface{}{"loc": "NYC"}, "get_weather"),
				),
			},
		}

		_, contents, _ := ConvertAnthropicToGoogleRequest(req, 4096)

		if len(contents) != 2 {
			t.Errorf("expected 2 contents, got %d", len(contents))
		}
		if contents[1].Role != "model" {
			t.Errorf("expected role 'model', got '%s'", contents[1].Role)
		}
		if len(contents[1].Parts) != 1 {
			t.Errorf("expected 1 part, got %d", len(contents[1].Parts))
		}
		if contents[1].Parts[0].FunctionCall == nil {
			t.Error("expected function call")
		} else if contents[1].Parts[0].FunctionCall.Name != "get_weather" {
			t.Errorf("expected function name 'get_weather', got '%s'", contents[1].Parts[0].FunctionCall.Name)
		}
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

// TestGoogleFinishReasonMapping tests finish reason conversion
func TestGoogleFinishReasonMapping(t *testing.T) {
	t.Run("mapOpenAIFinishReasonToGoogle", func(t *testing.T) {
		tests := []struct {
			input    string
			expected genai.FinishReason
		}{
			{"stop", genai.FinishReasonStop},
			{"length", genai.FinishReasonMaxTokens},
			{"content_filter", genai.FinishReasonSafety},
			{"tool_calls", genai.FinishReasonStop},
			{"unknown", genai.FinishReasonOther},
		}

		for _, tt := range tests {
			result := mapOpenAIFinishReasonToGoogle(tt.input)
			if result != tt.expected {
				t.Errorf("mapOpenAIFinishReasonToGoogle(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		}
	})

	t.Run("mapAnthropicFinishReasonToGoogle", func(t *testing.T) {
		tests := []struct {
			input    string
			expected genai.FinishReason
		}{
			{"end_turn", genai.FinishReasonStop},
			{"max_tokens", genai.FinishReasonMaxTokens},
			{"tool_use", genai.FinishReasonStop},
			{"content_filter", genai.FinishReasonSafety},
			{"unknown", genai.FinishReasonOther},
		}

		for _, tt := range tests {
			result := mapAnthropicFinishReasonToGoogle(tt.input)
			if result != tt.expected {
				t.Errorf("mapAnthropicFinishReasonToGoogle(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		}
	})

	t.Run("mapGoogleFinishReasonToOpenAI", func(t *testing.T) {
		tests := []struct {
			input    genai.FinishReason
			expected string
		}{
			{genai.FinishReasonStop, "stop"},
			{genai.FinishReasonMaxTokens, "length"},
			{genai.FinishReasonSafety, "content_filter"},
			{genai.FinishReasonOther, "stop"},
		}

		for _, tt := range tests {
			result := mapGoogleFinishReasonToOpenAI(tt.input)
			if result != tt.expected {
				t.Errorf("mapGoogleFinishReasonToOpenAI(%v) = %q, want %q", tt.input, result, tt.expected)
			}
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
			{genai.FinishReasonOther, "end_turn"},
		}

		for _, tt := range tests {
			result := mapGoogleFinishReasonToAnthropic(tt.input)
			if result != tt.expected {
				t.Errorf("mapGoogleFinishReasonToAnthropic(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		}
	})
}

// TestConvertGooglePartsToString tests converting Google parts to string
func TestConvertGooglePartsToString(t *testing.T) {
	t.Run("single text part", func(t *testing.T) {
		parts := []*genai.Part{
			genai.NewPartFromText("Hello"),
		}
		result := ConvertGooglePartsToString(parts)
		if result != "Hello" {
			t.Errorf("expected 'Hello', got '%s'", result)
		}
	})

	t.Run("multiple text parts", func(t *testing.T) {
		parts := []*genai.Part{
			genai.NewPartFromText("Hello, "),
			genai.NewPartFromText("world!"),
		}
		result := ConvertGooglePartsToString(parts)
		if result != "Hello, world!" {
			t.Errorf("expected 'Hello, world!', got '%s'", result)
		}
	})

	t.Run("empty parts", func(t *testing.T) {
		parts := []*genai.Part{}
		result := ConvertGooglePartsToString(parts)
		if result != "" {
			t.Errorf("expected empty string, got '%s'", result)
		}
	})

	t.Run("parts with non-text content", func(t *testing.T) {
		parts := []*genai.Part{
			genai.NewPartFromText("Text"),
			{FunctionCall: &genai.FunctionCall{Name: "test"}},
			genai.NewPartFromText(" more"),
		}
		result := ConvertGooglePartsToString(parts)
		if result != "Text more" {
			t.Errorf("expected 'Text more', got '%s'", result)
		}
	})
}

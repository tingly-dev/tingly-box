package adaptor

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"google.golang.org/genai"
)

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

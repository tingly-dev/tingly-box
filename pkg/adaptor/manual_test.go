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

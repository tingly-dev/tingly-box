package server

import (
	"fmt"
	"strings"
	"time"
	"tingly-box/internal/config"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
)

// probeWithOpenAI handles probe requests for OpenAI-style APIs
func probeWithOpenAI(c *gin.Context, provider *config.Provider, model string) (string, ProbeUsage, error) {
	startTime := time.Now()

	// Configure OpenAI client
	opts := []openaiOption.RequestOption{
		openaiOption.WithAPIKey(provider.Token),
	}
	if provider.APIBase != "" {
		opts = append(opts, openaiOption.WithBaseURL(provider.APIBase))
	}
	openaiClient := openai.NewClient(opts...)

	// Create chat completion request using OpenAI SDK
	chatRequest := &openai.ChatCompletionNewParams{
		Model: model, // Use empty stats for probe testing
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hi"),
		},
	}

	// Make request using OpenAI SDK
	resp, err := openaiClient.Chat.Completions.New(c.Request.Context(), *chatRequest)
	processingTime := time.Since(startTime).Milliseconds()

	var responseContent string
	var tokenUsage ProbeUsage

	if err == nil && resp != nil {
		// Extract response data
		if len(resp.Choices) > 0 {
			responseContent = resp.Choices[0].Message.Content
		}
		if resp.Usage.PromptTokens != 0 {
			tokenUsage.PromptTokens = int(resp.Usage.PromptTokens)
			tokenUsage.CompletionTokens = int(resp.Usage.CompletionTokens)
			tokenUsage.TotalTokens = int(resp.Usage.TotalTokens)
		}
	}

	if err != nil {
		// Handle error response
		errorMessage := err.Error()
		errorCode := "PROBE_FAILED"

		// Categorize common errors
		if strings.Contains(strings.ToLower(errorMessage), "authentication") || strings.Contains(strings.ToLower(errorMessage), "unauthorized") {
			errorCode = "AUTHENTICATION_FAILED"
		} else if strings.Contains(strings.ToLower(errorMessage), "rate limit") {
			errorCode = "RATE_LIMIT_EXCEEDED"
		} else if strings.Contains(strings.ToLower(errorMessage), "model") {
			errorCode = "MODEL_NOT_AVAILABLE"
		} else if strings.Contains(strings.ToLower(errorMessage), "timeout") || strings.Contains(strings.ToLower(errorMessage), "deadline") {
			errorCode = "CONNECTION_TIMEOUT"
		} else if strings.Contains(strings.ToLower(errorMessage), "token") {
			errorCode = "INVALID_API_KEY"
		}

		return "", tokenUsage, fmt.Errorf("%s: %s (processing time: %dms)", errorCode, errorMessage, processingTime)
	}

	// If response content is empty, provide fallback
	if responseContent == "" {
		responseContent = "<response content is empty, but request success>"
	}

	return responseContent, tokenUsage, nil
}

// probeWithAnthropic handles probe requests for Anthropic-style APIs
func probeWithAnthropic(c *gin.Context, provider *config.Provider, model string) (string, ProbeUsage, error) {
	startTime := time.Now()

	// Configure Anthropic client
	opts := []anthropicOption.RequestOption{
		anthropicOption.WithAPIKey(provider.Token),
	}
	if provider.APIBase != "" {
		opts = append(opts, anthropicOption.WithBaseURL(provider.APIBase))
	}
	anthropicClient := anthropic.NewClient(opts...)

	// Create message request using Anthropic SDK
	messageRequest := anthropic.MessageNewParams{
		Model: anthropic.Model(model), // Use empty stats for probe testing
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
		},
		MaxTokens: 100,
	}

	// Make request using Anthropic SDK
	resp, err := anthropicClient.Messages.New(c.Request.Context(), messageRequest)
	processingTime := time.Since(startTime).Milliseconds()

	var responseContent string
	var tokenUsage ProbeUsage

	if err == nil && resp != nil {
		// Extract response data
		for _, block := range resp.Content {
			if block.Type == "text" {
				responseContent += string(block.Text)
			}
		}
		if resp.Usage.InputTokens != 0 {
			tokenUsage.PromptTokens = int(resp.Usage.InputTokens)
			tokenUsage.CompletionTokens = int(resp.Usage.OutputTokens)
			tokenUsage.TotalTokens = int(resp.Usage.InputTokens) + int(resp.Usage.OutputTokens)
		}
	}

	if err != nil {
		// Handle error response
		errorMessage := err.Error()
		errorCode := "PROBE_FAILED"

		// Categorize common errors
		if strings.Contains(strings.ToLower(errorMessage), "authentication") || strings.Contains(strings.ToLower(errorMessage), "unauthorized") {
			errorCode = "AUTHENTICATION_FAILED"
		} else if strings.Contains(strings.ToLower(errorMessage), "rate limit") {
			errorCode = "RATE_LIMIT_EXCEEDED"
		} else if strings.Contains(strings.ToLower(errorMessage), "model") {
			errorCode = "MODEL_NOT_AVAILABLE"
		} else if strings.Contains(strings.ToLower(errorMessage), "timeout") || strings.Contains(strings.ToLower(errorMessage), "deadline") {
			errorCode = "CONNECTION_TIMEOUT"
		} else if strings.Contains(strings.ToLower(errorMessage), "token") {
			errorCode = "INVALID_API_KEY"
		}

		return "", tokenUsage, fmt.Errorf("%s: %s (processing time: %dms)", errorCode, errorMessage, processingTime)
	}

	// If response content is empty, provide fallback
	if responseContent == "" {
		responseContent = "<response content is empty, but request success>"
	}

	return responseContent, tokenUsage, nil
}

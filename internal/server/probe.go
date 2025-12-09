package server

import (
	"fmt"
	"net/http"
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
func probeWithOpenAI(c *gin.Context, rule config.Rule, provider *config.Provider) (string, struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}, error) {
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
		Model: rule.DefaultModel,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("hi"),
		},
	}

	// Make request using OpenAI SDK
	resp, err := openaiClient.Chat.Completions.New(c.Request.Context(), *chatRequest)
	processingTime := time.Since(startTime).Milliseconds()

	var responseContent string
	var tokenUsage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	}

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
func probeWithAnthropic(c *gin.Context, rule config.Rule, provider *config.Provider) (string, struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}, error) {
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
		Model: anthropic.Model(rule.DefaultModel),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
		},
		MaxTokens: 100,
	}

	// Make request using Anthropic SDK
	resp, err := anthropicClient.Messages.New(c.Request.Context(), messageRequest)
	processingTime := time.Since(startTime).Milliseconds()

	var responseContent string
	var tokenUsage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	}

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

func probe(c *gin.Context, rule config.Rule, provider *config.Provider) {
	startTime := time.Now()

	// Create the mock request data that would be sent to the API
	mockRequest := map[string]interface{}{
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "hi",
			},
		},
		"model":       rule.DefaultModel,
		"max_tokens":  100,
		"temperature": 0.7,
		"provider":    rule.Provider,
		"timestamp":   startTime.Format(time.RFC3339),
	}

	if provider == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PROVIDER_NOT_FOUND",
				"message": fmt.Sprintf("Provider '%s' not found or disabled", rule.Provider),
			},
			"data": gin.H{
				"request": mockRequest,
				"rule_tested": gin.H{
					"name":      rule.RequestModel,
					"provider":  rule.Provider,
					"model":     rule.DefaultModel,
					"timestamp": time.Now().Format(time.RFC3339),
				},
				"test_result": gin.H{
					"success": false,
					"message": fmt.Sprintf("Provider '%s' is not enabled or configured", rule.Provider),
				},
			},
		})
		return
	}

	// Call the appropriate probe function based on provider API style
	var responseContent string
	var tokenUsage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	}
	var err error

	switch provider.APIStyle {
	case config.APIStyleAnthropic:
		responseContent, tokenUsage, err = probeWithAnthropic(c, rule, provider)
	case config.APIStyleOpenAI:
		fallthrough
	default:
		responseContent, tokenUsage, err = probeWithOpenAI(c, rule, provider)
	}

	processingTime := time.Since(startTime).Milliseconds()

	if err != nil {
		// Extract error code from the formatted error message
		errorMessage := err.Error()
		errorCode := "PROBE_FAILED"

		// Parse error to extract code if available
		if strings.Contains(errorMessage, ":") {
			parts := strings.SplitN(errorMessage, ":", 2)
			if len(parts) == 2 && len(parts[0]) <= 25 { // Reasonable length for error code
				errorCode = strings.TrimSpace(parts[0])
				errorMessage = strings.TrimSpace(parts[1])
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error": gin.H{
				"code":    errorCode,
				"message": errorMessage,
				"details": gin.H{
					"provider":           rule.Provider,
					"model":              rule.DefaultModel,
					"timestamp":          time.Now().Format(time.RFC3339),
					"processing_time_ms": processingTime,
				},
			},
			"data": gin.H{
				"request": mockRequest,
				"response": gin.H{
					"content":  nil,
					"model":    rule.DefaultModel,
					"provider": rule.Provider,
					"usage": gin.H{
						"prompt_tokens":     0,
						"completion_tokens": 0,
						"total_tokens":      0,
					},
					"finish_reason": "error",
					"error":         errorMessage,
				},
				"rule_tested": gin.H{
					"name":      rule.RequestModel,
					"provider":  rule.Provider,
					"model":     rule.DefaultModel,
					"timestamp": time.Now().Format(time.RFC3339),
				},
				"test_result": gin.H{
					"success": false,
					"message": fmt.Sprintf("Probe failed: %s", errorMessage),
				},
			},
		})
		return
	}

	finishReason := "stop"
	if tokenUsage.TotalTokens == 0 {
		finishReason = "unknown"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"request": mockRequest,
			"response": gin.H{
				"content":  responseContent,
				"model":    rule.DefaultModel,
				"provider": rule.Provider,
				"usage": gin.H{
					"prompt_tokens":     tokenUsage.PromptTokens,
					"completion_tokens": tokenUsage.CompletionTokens,
					"total_tokens":      tokenUsage.TotalTokens,
				},
				"finish_reason": finishReason,
			},
			"rule_tested": gin.H{
				"name":      rule.RequestModel,
				"provider":  rule.Provider,
				"model":     rule.DefaultModel,
				"timestamp": time.Now().Format(time.RFC3339),
			},
			"test_result": gin.H{
				"success": true,
				"message": "Rule configuration is valid and working correctly",
			},
		},
	})
}

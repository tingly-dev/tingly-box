package server

import (
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/probe"
)

// =============================================
// Token Management Models
// =============================================

// GenerateTokenRequest represents the request to generate a token
type GenerateTokenRequest struct {
	ClientID string `json:"client_id" binding:"required" description:"Client ID for token generation" example:"user123"`
}

// =============================================
// OpenAI API Models
// =============================================

// OpenAIChatCompletionResponse represents the OpenAI chat completion response
type OpenAIChatCompletionResponse struct {
	ID      string `json:"id" example:"chatcmpl-123"`
	Object  string `json:"object" example:"chat.completion"`
	Created int64  `json:"created" example:"1677652288"`
	Model   string `json:"model" example:"gpt-3.5-turbo"`
	Choices []struct {
		Index   int `json:"index" example:"0"`
		Message struct {
			Role    string `json:"role" example:"assistant"`
			Content string `json:"content" example:"Hello! How can I help you?"`
		} `json:"message"`
		FinishReason string `json:"finish_reason" example:"stop"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens" example:"10"`
		CompletionTokens int `json:"completion_tokens" example:"20"`
		TotalTokens      int `json:"total_tokens" example:"30"`
	} `json:"usage"`
}

// =============================================
// Web UI API Models — probe request/data types live in internal/probe
// =============================================

// ProbeProviderResponse represents the response from provider probing.
// The wrapper stays here because it embeds *ErrorDetail (server's global
// error model). The Data shape lives in internal/probe.
type ProbeProviderResponse struct {
	Success bool                             `json:"success" example:"true"`
	Error   *ErrorDetail                     `json:"error,omitempty"`
	Data    *probe.ProbeProviderResponseData `json:"data,omitempty"`
}

// RequestConfig represents a request configuration in defaults response
type RequestConfig struct {
	RequestModel  string `json:"request_model" example:"gpt-3.5-turbo"`
	ResponseModel string `json:"response_model" example:"gpt-3.5-turbo"`
	Provider      string `json:"provider" example:"openai"`
	DefaultModel  string `json:"default_model" example:"gpt-3.5-turbo"`
}

// =============================================
// Probe API Models
// =============================================

// ProbeUsage represents token usage information
type ProbeUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	TimeCost         int `json:"time_cost"`
}

// ProbeResponseData represents the response data structure
type ProbeResponseData struct {
	Request     ProbeRequestDetail  `json:"request"`
	Response    ProbeResponseDetail `json:"response"`
	Usage       ProbeUsage          `json:"usage"`
	CurlCommand string              `json:"curl_command,omitempty"`
}

// ProbeResponseDetail represents the API response
type ProbeResponseDetail struct {
	Content      string `json:"content"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	FinishReason string `json:"finish_reason"`
	Error        string `json:"error,omitempty"`
}

// ProbeRequestDetail represents the mock request data for probing
type ProbeRequestDetail struct {
	Messages    []map[string]interface{} `json:"messages"`
	Model       string                   `json:"model"`
	MaxTokens   int                      `json:"max_tokens"`
	Temperature float64                  `json:"temperature"`
	Provider    string                   `json:"provider"`
	Timestamp   string                   `json:"timestamp"`
}

// NewMockRequest creates a new mock request with default values
func NewMockRequest(provider, model string) ProbeRequestDetail {
	return ProbeRequestDetail{
		Messages: []map[string]interface{}{
			//{
			//	"role":    "system",
			//	"content": "work as `echo`",
			//},
			{
				"role":    "user",
				"content": "hi",
			},
		},
		Model:     model,
		MaxTokens: 100,
		Provider:  provider,
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

// GenerateCurlCommand generates a curl command for testing the provider
func GenerateCurlCommand(apiBase, apiStyle, token, model string) string {
	baseURL := strings.TrimSuffix(apiBase, "/")
	var endpoint string
	var requestBody string

	if apiStyle == "anthropic" {
		endpoint = "/v1/messages"
		requestBody = `{
  "model": "` + model + `",
  "max_tokens": 1024,
  "messages": [
    {"role": "user", "content": "Hello, world!"}
  ]
}`
	} else {
		// OpenAI style (default for ollama and others)
		// For OpenAI style, we need to ensure the URL is correct
		// The provider's APIBase should already include the correct path
		// Don't add /v1 if the base URL already has it (like ollama with /v1/v1)
		endpoint = "/chat/completions"
		requestBody = `{
  "model": "` + model + `",
  "messages": [
    {"role": "user", "content": "Hello, world!"}
  ]
}`
	}

	url := baseURL + endpoint

	curl := "curl -X POST \"" + url + "\" \\\n" +
		"  -H \"Content-Type: application/json\" \\\n" +
		"  -H \"Authorization: Bearer " + token + "\" \\\n" +
		"  -d '" + requestBody + "'"

	return curl
}

// ProbeResponse represents the overall probe response
type ProbeResponse struct {
	Success bool               `json:"success"`
	Error   *ErrorDetail       `json:"error,omitempty"`
	Data    *ProbeResponseData `json:"data,omitempty"`
}

package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Error Models

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail represents error details
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// SendErrorResponse registers the error into gin context for logging middleware and sends JSON response.
func SendErrorResponse(c *gin.Context, err error, desc string) {

	// upstreamForwardStatus returns the status code to send to the client when a
	// non-streaming forward fails. It propagates the upstream provider's HTTP status
	// when the error carries one (so a 401/429/4xx is not flattened into a 500) and
	// defaults to 500 Internal Server Error otherwise.
	statusCode := protocol.UpstreamStatus(err, http.StatusInternalServerError)

	asErr := fmt.Errorf("%s: %s", err.Error(), desc)
	c.Error(asErr).SetType(gin.ErrorTypePublic) //nolint:errcheck
	c.JSON(statusCode, ErrorResponse{
		Error: ErrorDetail{
			Message: asErr.Error(),
			Type:    "protocol_error",
			Code:    desc,
		},
	})
}

// =============================================
// Token Management Models
// =============================================

// GenerateTokenRequest represents the request to generate a token
type GenerateTokenRequest struct {
	ClientID string `json:"client_id" binding:"required" description:"Client ID for token generation" example:"user123"`
}

// TokenResponse represents the token response
type TokenResponse struct {
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	Type  string `json:"type" example:"Bearer"`
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
// Load Balancer API Models
// =============================================

// UpdateRuleTacticRequest represents the request to update rule tactic
type UpdateRuleTacticRequest struct {
	Tactic string `json:"tactic" binding:"required,oneof=token_based random latency_based speed_based adaptive" description:"Load balancing tactic" example:"adaptive"`
}

// UpdateRuleTacticResponse represents the response for updating rule tactic
type UpdateRuleTacticResponse struct {
	Message string `json:"message" example:"Tactic updated successfully"`
	Tactic  string `json:"tactic" example:"adaptive"`
}

// RuleStatsResponse represents the statistics response for a rule
type RuleStatsResponse struct {
	Rule  string                 `json:"rule" example:"gpt-4"`
	Stats map[string]interface{} `json:"stats"`
}

// ServiceStatsResponse represents the statistics response for a service
type ServiceStatsResponse struct {
	ServiceID string                 `json:"service_id" example:"openai:gpt-4"`
	Stats     map[string]interface{} `json:"stats,omitempty"`
}

// AllStatsResponse represents the response for all statistics
type AllStatsResponse struct {
	Stats map[string]interface{} `json:"stats"`
}

// CurrentServiceResponse represents the current service response
type CurrentServiceResponse struct {
	Rule      string                 `json:"rule" example:"gpt-4"`
	Service   interface{}            `json:"service"`
	ServiceID string                 `json:"service_id" example:"openai:gpt-4"`
	Tactic    string                 `json:"tactic" example:"adaptive"`
	Stats     map[string]interface{} `json:"stats,omitempty"`
}

// ServiceHealthResponse represents the health check response for services
type ServiceHealthResponse struct {
	Rule   string                 `json:"rule" example:"gpt-4"`
	Health map[string]interface{} `json:"health"`
}

// ServiceMetric represents a service metric entry
type ServiceMetric struct {
	ServiceID            string `json:"service_id" example:"openai:gpt-4"`
	RequestCount         int64  `json:"request_count" example:"100"`
	WindowRequestCount   int64  `json:"window_request_count" example:"50"`
	WindowTokensConsumed int64  `json:"window_tokens_consumed" example:"25000"`
	WindowInputTokens    int64  `json:"window_input_tokens" example:"15000"`
	WindowOutputTokens   int64  `json:"window_output_tokens" example:"10000"`
	LastUsed             string `json:"last_used" example:"2024-01-01T12:00:00Z"`
}

// MetricsResponse represents the metrics response
type MetricsResponse struct {
	Metrics       []ServiceMetric `json:"metrics"`
	TotalServices int             `json:"total_services" example:"5"`
}

// ClearStatsResponse represents the response for clearing statistics
type ClearStatsResponse struct {
	Message string `json:"message" example:"Statistics cleared for rule: gpt-4"`
}

// RuleSummaryResponse represents a rule summary response
type RuleSummaryResponse struct {
	Summary interface{} `json:"summary"`
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

// OpenAIModel represents a model in OpenAI's models API format
type StatusResponse struct {
	Success bool `json:"success" example:"true"`
	Data    struct {
		ServerRunning    bool `json:"server_running" example:"true"`
		Port             int  `json:"port" example:"12580"`
		ProvidersTotal   int  `json:"providers_total" example:"3"`
		ProvidersEnabled int  `json:"providers_enabled" example:"2"`
		RequestCount     int  `json:"request_count" example:"100"`
	} `json:"data"`
}

// HistoryResponse represents the response for request history
type HistoryResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    interface{} `json:"data"`
}

// RequestConfig represents a request configuration in defaults response
type RequestConfig struct {
	RequestModel  string `json:"request_model" example:"gpt-3.5-turbo"`
	ResponseModel string `json:"response_model" example:"gpt-3.5-turbo"`
	Provider      string `json:"provider" example:"openai"`
	DefaultModel  string `json:"default_model" example:"gpt-3.5-turbo"`
}

// ServerActionResponse represents the response for server actions (start/stop/restart)
type ServerActionResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Server stopped successfully"`
}

// OpenAIModel represents a model in OpenAI's models API format
type OpenAIModel struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	Description string `json:"description,omitempty"` // Model description
	Context     int    `json:"context,omitempty"`     // Max context window
	MaxOutput   int    `json:"max_output,omitempty"`  // Max output tokens
	// AuthType reflects the primary backing provider's auth type. It is
	// non-standard (OpenAI's models API has no such field) and consumed by
	// the tingly-box frontend to order model picker entries:
	// oauth -> api_key -> vmodel.
	AuthType string `json:"auth_type,omitempty"`
}

// OpenAIModelsResponse represents OpenAI's models API response format
type OpenAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
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

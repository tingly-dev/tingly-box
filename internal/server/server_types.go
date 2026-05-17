package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/typ"
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

// sendErrorResponse registers the error into gin context for logging middleware and sends JSON response.
func SendErrorResponse(c *gin.Context, statusCode int, err error, errType string) {
	c.Error(fmt.Errorf("%s: %w", errType, err)).SetType(gin.ErrorTypePublic) //nolint:errcheck
	c.JSON(statusCode, ErrorResponse{
		Error: ErrorDetail{
			Message: err.Error(),
			Type:    errType,
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

// ProviderResponse represents a provider configuration with masked token
type ProviderResponse struct {
	UUID             string            `json:"uuid" example:"0123456789ABCDEF"`
	Name             string            `json:"name" example:"openai"`
	APIBase          string            `json:"api_base" example:"https://api.openai.com/v1"`
	APIStyle         string            `json:"api_style" example:"openai"`
	APIBaseOpenAI    string            `json:"api_base_openai,omitempty" example:"https://api.example.com/v1"`
	APIBaseAnthropic string            `json:"api_base_anthropic,omitempty" example:"https://api.example.com"`
	Token            string            `json:"token" example:"sk-***...***"` // Only populated for api_key auth type
	NoKeyRequired    bool              `json:"no_key_required" example:"false"`
	Enabled          bool              `json:"enabled" example:"true"`
	ProxyURL         string            `json:"proxy_url,omitempty" example:"http://localhost:7890"`
	UserAgent        string            `json:"user_agent,omitempty" example:"my-gateway/1.0"`
	AuthType         string            `json:"auth_type,omitempty" example:"api_key"` // api_key, oauth, or vmodel
	OAuthDetail      *typ.OAuthDetail  `json:"oauth_detail,omitempty"`                // OAuth credentials (only for oauth auth type)
	VModelDetail     *typ.VModelDetail `json:"vmodel_detail,omitempty"`               // Virtual-model config (only for vmodel auth type)
	Source           string            `json:"source,omitempty" example:"user"`       // "user" (default) or "builtin"
}

// ProvidersResponse represents the response for listing providers
type ProvidersResponse struct {
	Success bool               `json:"success" example:"true"`
	Data    []ProviderResponse `json:"data"`
}

// StatusResponse represents the server status response
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

// CreateProviderRequest represents the request to add a new provider
type CreateProviderRequest struct {
	Name             string `json:"name" binding:"required" description:"Provider name" example:"openai"`
	APIBase          string `json:"api_base" binding:"required" description:"API base URL" example:"https://api.openai.com/v1"`
	APIStyle         string `json:"api_style" description:"API style" example:"openai"`
	APIBaseOpenAI    string `json:"api_base_openai,omitempty" description:"Fusion-mode OpenAI-compatible base URL (optional, api_key auth only)" example:"https://api.example.com/v1"`
	APIBaseAnthropic string `json:"api_base_anthropic,omitempty" description:"Fusion-mode Anthropic-compatible base URL (optional, api_key auth only)" example:"https://api.example.com"`
	Token            string `json:"token" description:"API token" example:"sk-..."`
	NoKeyRequired    bool   `json:"no_key_required" description:"Whether provider requires no API key" example:"false"`
	Enabled          bool   `json:"enabled" description:"Whether provider is enabled" example:"true"`
	ProxyURL         string `json:"proxy_url,omitempty" description:"HTTP or SOCKS proxy URL (e.g., http://localhost:7890 or socks5://localhost:1080)" example:"http://localhost:7890"`
	UserAgent        string `json:"user_agent,omitempty" description:"Custom outbound HTTP User-Agent; empty uses the built-in/default for this provider" example:"my-gateway/1.0"`
	AuthType         string `json:"auth_type,omitempty" description:"Auth type: api_key or oauth (default: api_key)" example:"api_key"`
}

// CreateProviderResponse represents the response for adding a provider
type CreateProviderResponse struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message" example:"Provider added successfully"`
	Data    interface{} `json:"data"`
}

// UpdateProviderRequest represents the request to update a provider
type UpdateProviderRequest struct {
	Name             *string `json:"name,omitempty" description:"New provider name"`
	APIBase          *string `json:"api_base,omitempty" description:"New API base URL"`
	APIStyle         *string `json:"api_style,omitempty" description:"New API style"`
	APIBaseOpenAI    *string `json:"api_base_openai,omitempty" description:"New fusion-mode OpenAI-compatible base URL (empty string clears it)"`
	APIBaseAnthropic *string `json:"api_base_anthropic,omitempty" description:"New fusion-mode Anthropic-compatible base URL (empty string clears it)"`
	Token            *string `json:"token,omitempty" description:"New API token"`
	NoKeyRequired    *bool   `json:"no_key_required,omitempty" description:"Whether provider requires no API key"`
	Enabled          *bool   `json:"enabled,omitempty" description:"New enabled status"`
	ProxyURL         *string `json:"proxy_url,omitempty" description:"HTTP or SOCKS proxy URL"`
	UserAgent        *string `json:"user_agent,omitempty" description:"Custom outbound HTTP User-Agent (empty string clears it and reverts to default)"`
}

// UpdateProviderResponse represents the response for updating a provider
type UpdateProviderResponse struct {
	Success bool             `json:"success" example:"true"`
	Message string           `json:"message" example:"Provider updated successfully"`
	Data    ProviderResponse `json:"data"`
}

// ToggleProviderResponse represents the response for toggling a provider
type ToggleProviderResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Provider openai enabled successfully"`
	Data    struct {
		Enabled bool `json:"enabled" example:"true"`
	} `json:"data"`
}

// DeleteProviderResponse represents the response for deleting a provider
type DeleteProviderResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Provider deleted successfully"`
}

// ServerActionResponse represents the response for server actions (start/stop/restart)
type ServerActionResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Server stopped successfully"`
}

// ProviderModelInfo represents model information for a specific provider
type ProviderModelInfo struct {
	Models      []string             `json:"models" example:"gpt-3.5-turbo,gpt-4"`
	StarModels  []string             `json:"star_models" example:"gpt-4"`
	CustomModel []string             `json:"custom_model" example:"custom-gpt-model"`
	APIBase     string               `json:"api_base" example:"https://api.openai.com/v1"`
	LastUpdated string               `json:"last_updated,omitempty" example:"2024-01-15 10:30:00"`
	Quota       *quota.ProviderUsage `json:"quota,omitempty"` // Quota information for this provider
}

// ProviderModelsResponse represents the response for getting provider models
type ProviderModelsResponse struct {
	Success bool              `json:"success" example:"true"`
	Message string            `json:"message" example:"Provider models successfully"`
	Data    ProviderModelInfo `json:"data"`
}

// FetchProviderModelsResponse represents the response for fetching provider models
type FetchProviderModelsResponse struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message" example:"Successfully fetched 150 models for provider openai"`
	Data    interface{} `json:"data"`
}

// OpenAIModel represents a model in OpenAI's models API format
type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
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

// =============================================
// Probe API Models
// =============================================

// ModelProbeRequest lives in internal/probe.

// EndpointProbeStatus represents the status of an endpoint probe
type EndpointProbeStatus struct {
	Available    bool   `json:"available" example:"true"`
	LatencyMs    int    `json:"latency_ms" example:"234"`
	ErrorMessage string `json:"error_message,omitempty" example:""`
	LastChecked  string `json:"last_checked" example:"2026-01-23T10:30:00Z"`
}

// ModelProbeResponse represents the response from model endpoint probing
type ModelProbeResponse struct {
	Success bool            `json:"success" example:"true"`
	Error   *ErrorDetail    `json:"error,omitempty"`
	Data    *ModelProbeData `json:"data,omitempty"`
}

// ModelProbeData represents the probe result data
type ModelProbeData struct {
	ProviderUUID      string              `json:"provider_uuid" example:"uuid-123"`
	ModelID           string              `json:"model_id" example:"gpt-4"`
	ChatEndpoint      EndpointProbeStatus `json:"chat_endpoint"`
	ResponsesEndpoint EndpointProbeStatus `json:"responses_endpoint"`
	PreferredEndpoint string              `json:"preferred_endpoint" example:"responses"`
	LastUpdated       string              `json:"last_updated" example:"2026-01-23T10:30:00Z"`
}

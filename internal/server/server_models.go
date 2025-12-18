package server

import (
	"time"
	"tingly-box/internal/config"

	"github.com/openai/openai-go/v3"
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

// =============================================
// Health Check Models
// =============================================

// HealthCheckResponse represents the health check response
type HealthCheckResponse struct {
	Status  string `json:"status" example:"healthy"`
	Service string `json:"service" example:"tingly-box"`
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

// OpenAIChatCompletionRequest is a type alias for OpenAI chat completion request
type OpenAIChatCompletionRequest = openai.ChatCompletionNewParams

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
	Tactic string `json:"tactic" binding:"required,oneof=round_robin weighted_random least_tokens least_requests" description:"Load balancing tactic" example:"round_robin"`
}

// UpdateRuleTacticResponse represents the response for updating rule tactic
type UpdateRuleTacticResponse struct {
	Message string `json:"message" example:"Tactic updated successfully"`
	Tactic  string `json:"tactic" example:"round_robin"`
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
	Tactic    string                 `json:"tactic" example:"round_robin"`
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

// RuleResponse represents a rule configuration response
type RuleResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    interface{} `json:"data"`
}

// RuleSummaryResponse represents a rule summary response
type RuleSummaryResponse struct {
	Summary interface{} `json:"summary"`
}

// =============================================
// Web UI API Models
// =============================================

// ProbeRequest represents the request to probe/test a provider and model
type ProbeRequest struct {
	Provider string `json:"provider" binding:"required" description:"Provider name to test against" example:"openai"`
	Model    string `json:"model" binding:"required" description:"Model name to test against" example:"gpt-4-latest"`
}

// ProviderResponse represents a provider configuration with masked token
type ProviderResponse struct {
	Name     string `json:"name" example:"openai"`
	APIBase  string `json:"api_base" example:"https://api.openai.com/v1"`
	APIStyle string `json:"api_style" example:"openai"`
	Token    string `json:"token" example:"sk-***...***"`
	Enabled  bool   `json:"enabled" example:"true"`
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
		Port             int  `json:"port" example:"8080"`
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

// RulesResponse represents the response for getting all rules
type RulesResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    interface{} `json:"data"`
}

// SetRuleRequest represents the request to set/update a rule
type SetRuleRequest config.Rule

// SetRuleResponse represents the response for setting/updating a rule
type SetRuleResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Rule saved successfully"`
	Data    struct {
		RequestModel  string `json:"request_model" example:"gpt-3.5-turbo"`
		ResponseModel string `json:"response_model" example:"gpt-3.5-turbo"`
		Provider      string `json:"provider" example:"openai"`
		DefaultModel  string `json:"default_model" example:"gpt-3.5-turbo"`
		Active        bool   `json:"active" example:"true"`
	} `json:"data"`
}

// DeleteRuleResponse represents the response for deleting a rule
type DeleteRuleResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Rule deleted successfully"`
}

// AddProviderRequest represents the request to add a new provider
type AddProviderRequest struct {
	Name     string `json:"name" binding:"required" description:"Provider name" example:"openai"`
	APIBase  string `json:"api_base" binding:"required" description:"API base URL" example:"https://api.openai.com/v1"`
	APIStyle string `json:"api_style" description:"API style" example:"openai"`
	Token    string `json:"token" binding:"required" description:"API token" example:"sk-..."`
	Enabled  bool   `json:"enabled" description:"Whether provider is enabled" example:"true"`
}

// AddProviderResponse represents the response for adding a provider
type AddProviderResponse struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message" example:"Provider added successfully"`
	Data    interface{} `json:"data"`
}

// UpdateProviderRequest represents the request to update a provider
type UpdateProviderRequest struct {
	Name     *string `json:"name,omitempty" description:"New provider name"`
	APIBase  *string `json:"api_base,omitempty" description:"New API base URL"`
	APIStyle *string `json:"api_style,omitempty" description:"New API style"`
	Token    *string `json:"token,omitempty" description:"New API token"`
	Enabled  *bool   `json:"enabled,omitempty" description:"New enabled status"`
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
	Models      []string `json:"models" example:"gpt-3.5-turbo,gpt-4"`
	StarModels  []string `json:"star_models" example:"gpt-4"`
	CustomModel []string `json:"custom_model" example:"custom-gpt-model"`
	APIBase     string   `json:"api_base" example:"https://api.openai.com/v1"`
	LastUpdated string   `json:"last_updated,omitempty" example:"2024-01-15 10:30:00"`
}

// ProviderModelsResponse represents the response for getting provider models
type ProviderModelsResponse struct {
	Success bool                          `json:"success" example:"true"`
	Data    map[string]*ProviderModelInfo `json:"data"`
}

// FetchProviderModelsResponse represents the response for fetching provider models
type FetchProviderModelsResponse struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message" example:"Successfully fetched 150 models for provider openai"`
	Data    interface{} `json:"data"`
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
	Request  ProbeRequestDetail  `json:"request"`
	Response ProbeResponseDetail `json:"response"`
	Usage    ProbeUsage          `json:"usage"`
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
			{
				"role":    "user",
				"content": "hi",
			},
		},
		Model:       model,
		MaxTokens:   100,
		Temperature: 0.7,
		Provider:    provider,
		Timestamp:   time.Now().Format(time.RFC3339),
	}
}

// ProbeResponse represents the overall probe response
type ProbeResponse struct {
	Success bool               `json:"success"`
	Error   *ErrorDetail       `json:"error,omitempty"`
	Data    *ProbeResponseData `json:"data,omitempty"`
}

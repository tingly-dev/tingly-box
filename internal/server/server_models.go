package server

import (
	"strings"
	"time"

	smartrouting "tingly-box/internal/smart_routing"
	"tingly-box/internal/typ"
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
	Success bool      `json:"success" example:"true"`
	Data    *typ.Rule `json:"data"`
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

// ProbeProviderRequest represents the request to probe/test a provider's API key and connectivity
type ProbeProviderRequest struct {
	Name     string `json:"name" binding:"required" description:"Provider name" example:"openai"`
	APIBase  string `json:"api_base" binding:"required" description:"API base URL" example:"https://api.openai.com/v1"`
	APIStyle string `json:"api_style" binding:"required,oneof=openai anthropic" description:"API style" example:"openai"`
	Token    string `json:"token" binding:"required" description:"API token to test" example:"sk-..."`
}

// ProbeProviderResponse represents the response from provider probing
type ProbeProviderResponse struct {
	Success bool                       `json:"success" example:"true"`
	Error   *ErrorDetail               `json:"error,omitempty"`
	Data    *ProbeProviderResponseData `json:"data,omitempty"`
}

// ProbeProviderResponseData represents the data returned from provider probing
type ProbeProviderResponseData struct {
	Provider     string `json:"provider" example:"openai"`
	APIBase      string `json:"api_base" example:"https://api.openai.com/v1"`
	APIStyle     string `json:"api_style" example:"openai"`
	Valid        bool   `json:"valid" example:"true"`
	Message      string `json:"message" example:"API key is valid and accessible"`
	TestResult   string `json:"test_result" example:"models_endpoint_success"`
	ResponseTime int64  `json:"response_time_ms" example:"250"`
	ModelsCount  int    `json:"models_count,omitempty" example:"150"`
}

// ProviderResponse represents a provider configuration with masked token
type ProviderResponse struct {
	UUID          string           `json:"uuid" example:"0123456789ABCDEF"`
	Name          string           `json:"name" example:"openai"`
	APIBase       string           `json:"api_base" example:"https://api.openai.com/v1"`
	APIStyle      string           `json:"api_style" example:"openai"`
	Token         string           `json:"token" example:"sk-***...***"` // Only populated for api_key auth type
	NoKeyRequired bool             `json:"no_key_required" example:"false"`
	Enabled       bool             `json:"enabled" example:"true"`
	ProxyURL      string           `json:"proxy_url,omitempty" example:"http://127.0.0.1:7890"`
	AuthType      string           `json:"auth_type,omitempty" example:"api_key"` // api_key or oauth
	OAuthDetail   *typ.OAuthDetail `json:"oauth_detail,omitempty"`                // OAuth credentials (only for oauth auth type)
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

// RulesResponse represents the response for getting all rules
type RulesResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    interface{} `json:"data"`
}

type CreateRuleRequest typ.Rule

// UpdateRuleRequest represents the request to set/update a rule
type UpdateRuleRequest typ.Rule

// UpdateRuleResponse represents the response for setting/updating a rule
type UpdateRuleResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Rule saved successfully"`
	Data    struct {
		UUID          string                      `json:"uuid"`
		RequestModel  string                      `json:"request_model" example:"gpt-3.5-turbo"`
		ResponseModel string                      `json:"response_model" example:"gpt-3.5-turbo"`
		Description   string                      `json:"description" example:"My rule description"`
		Provider      string                      `json:"provider" example:"openai"`
		DefaultModel  string                      `json:"default_model" example:"gpt-3.5-turbo"`
		Active        bool                        `json:"active" example:"true"`
		SmartEnabled  bool                        `json:"smart_enabled" example:"false"`
		SmartRouting  []smartrouting.SmartRouting `json:"smart_routing,omitempty"`
	} `json:"data"`
}

// DeleteRuleResponse represents the response for deleting a rule
type DeleteRuleResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Rule deleted successfully"`
}

// CreateProviderRequest represents the request to add a new provider
type CreateProviderRequest struct {
	Name          string `json:"name" binding:"required" description:"Provider name" example:"openai"`
	APIBase       string `json:"api_base" binding:"required" description:"API base URL" example:"https://api.openai.com/v1"`
	APIStyle      string `json:"api_style" description:"API style" example:"openai"`
	Token         string `json:"token" description:"API token" example:"sk-..."`
	NoKeyRequired bool   `json:"no_key_required" description:"Whether provider requires no API key" example:"false"`
	Enabled       bool   `json:"enabled" description:"Whether provider is enabled" example:"true"`
	ProxyURL      string `json:"proxy_url,omitempty" description:"HTTP or SOCKS proxy URL (e.g., http://127.0.0.1:7890 or socks5://127.0.0.1:1080)" example:"http://127.0.0.1:7890"`
}

// CreateProviderResponse represents the response for adding a provider
type CreateProviderResponse struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message" example:"Provider added successfully"`
	Data    interface{} `json:"data"`
}

// UpdateProviderRequest represents the request to update a provider
type UpdateProviderRequest struct {
	Name          *string `json:"name,omitempty" description:"New provider name"`
	APIBase       *string `json:"api_base,omitempty" description:"New API base URL"`
	APIStyle      *string `json:"api_style,omitempty" description:"New API style"`
	Token         *string `json:"token,omitempty" description:"New API token"`
	NoKeyRequired *bool   `json:"no_key_required,omitempty" description:"Whether provider requires no API key"`
	Enabled       *bool   `json:"enabled,omitempty" description:"New enabled status"`
	ProxyURL      *string `json:"proxy_url,omitempty" description:"HTTP or SOCKS proxy URL"`
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
// Usage API Models
// =============================================

// UsageStatsQuery represents query parameters for usage statistics
type UsageStatsQuery struct {
	GroupBy   string `json:"group_by" form:"group_by" description:"Aggregation level: model, provider, scenario, rule, daily, hourly" example:"model"`
	StartTime string `json:"start_time" form:"start_time" description:"ISO 8601 start time" example:"2025-01-10T00:00:00Z"`
	EndTime   string `json:"end_time" form:"end_time" description:"ISO 8601 end time" example:"2025-01-11T00:00:00Z"`
	Provider  string `json:"provider" form:"provider" description:"Filter by provider UUID"`
	Model     string `json:"model" form:"model" description:"Filter by model name"`
	Scenario  string `json:"scenario" form:"scenario" description:"Filter by scenario"`
	RuleUUID  string `json:"rule_uuid" form:"rule_uuid" description:"Filter by rule UUID"`
	Status    string `json:"status" form:"status" description:"Filter by status: success, error, partial" example:"success"`
	Limit     int    `json:"limit" form:"limit" description:"Max results to return" example:"100"`
	SortBy    string `json:"sort_by" form:"sort_by" description:"Sort field: total_tokens, request_count, avg_latency" example:"total_tokens"`
	SortOrder string `json:"sort_order" form:"sort_order" description:"asc or desc" example:"desc"`
}

// AggregatedStat represents aggregated usage statistics
type AggregatedStat struct {
	Key             string  `json:"key" example:"gpt-4"`
	ProviderUUID    string  `json:"provider_uuid,omitempty" example:"uuid-123"`
	ProviderName    string  `json:"provider_name,omitempty" example:"openai"`
	Model           string  `json:"model,omitempty" example:"gpt-4"`
	Scenario        string  `json:"scenario,omitempty" example:"openai"`
	RequestCount    int64   `json:"request_count" example:"5420"`
	TotalTokens     int64   `json:"total_tokens" example:"2140000"`
	InputTokens     int64   `json:"total_input_tokens" example:"1250000"`
	OutputTokens    int64   `json:"total_output_tokens" example:"890000"`
	AvgInputTokens  float64 `json:"avg_input_tokens" example:"230.6"`
	AvgOutputTokens float64 `json:"avg_output_tokens" example:"164.2"`
	AvgLatencyMs    float64 `json:"avg_latency_ms" example:"1250"`
	ErrorCount      int64   `json:"error_count" example:"12"`
	ErrorRate       float64 `json:"error_rate" example:"0.0022"`
	StreamedCount   int64   `json:"streamed_count" example:"4800"`
	StreamedRate    float64 `json:"streamed_rate" example:"0.885"`
}

// UsageStatsResponse represents the response for usage statistics
type UsageStatsResponse struct {
	Meta UsageStatsMeta   `json:"meta"`
	Data []AggregatedStat `json:"data"`
}

// UsageStatsMeta represents metadata for usage statistics response
type UsageStatsMeta struct {
	StartTime  string `json:"start_time" example:"2025-01-10T00:00:00Z"`
	EndTime    string `json:"end_time" example:"2025-01-11T00:00:00Z"`
	GroupBy    string `json:"group_by" example:"model"`
	TotalCount int    `json:"total_count" example:"10"`
}

// TimeSeriesQuery represents query parameters for time-series data
type TimeSeriesQuery struct {
	Interval  string `json:"interval" form:"interval" description:"Time bucket: minute, hour, day, week" example:"hour"`
	StartTime string `json:"start_time" form:"start_time" description:"ISO 8601 start time" example:"2025-01-10T00:00:00Z"`
	EndTime   string `json:"end_time" form:"end_time" description:"ISO 8601 end time" example:"2025-01-11T00:00:00Z"`
	Provider  string `json:"provider" form:"provider" description:"Filter by provider UUID"`
	Model     string `json:"model" form:"model" description:"Filter by model name"`
	Scenario  string `json:"scenario" form:"scenario" description:"Filter by scenario"`
}

// TimeSeriesData represents a single time bucket in time series data
type TimeSeriesData struct {
	Timestamp    string  `json:"timestamp" example:"2025-01-10T00:00:00Z"`
	RequestCount int64   `json:"request_count" example:"245"`
	TotalTokens  int64   `json:"total_tokens" example:"52000"`
	InputTokens  int64   `json:"input_tokens" example:"32000"`
	OutputTokens int64   `json:"output_tokens" example:"20000"`
	ErrorCount   int64   `json:"error_count" example:"0"`
	AvgLatencyMs float64 `json:"avg_latency_ms" example:"1100"`
}

// TimeSeriesResponse represents the response for time-series data
type TimeSeriesResponse struct {
	Meta TimeSeriesMeta   `json:"meta"`
	Data []TimeSeriesData `json:"data"`
}

// TimeSeriesMeta represents metadata for time-series response
type TimeSeriesMeta struct {
	Interval  string `json:"interval" example:"hour"`
	StartTime string `json:"start_time" example:"2025-01-10T00:00:00Z"`
	EndTime   string `json:"end_time" example:"2025-01-11T00:00:00Z"`
}

// UsageRecordsQuery represents query parameters for usage records
type UsageRecordsQuery struct {
	StartTime string `json:"start_time" form:"start_time" description:"ISO 8601 start time" example:"2025-01-10T00:00:00Z"`
	EndTime   string `json:"end_time" form:"end_time" description:"ISO 8601 end time" example:"2025-01-11T00:00:00Z"`
	Provider  string `json:"provider" form:"provider" description:"Filter by provider UUID"`
	Model     string `json:"model" form:"model" description:"Filter by model name"`
	Scenario  string `json:"scenario" form:"scenario" description:"Filter by scenario"`
	Status    string `json:"status" form:"status" description:"Filter by status"`
	Limit     int    `json:"limit" form:"limit" description:"Max results (max 1000)" example:"50"`
	Offset    int    `json:"offset" form:"offset" description:"Pagination offset" example:"0"`
}

// UsageRecordResponse represents a single usage record
type UsageRecordResponse struct {
	ID           uint   `json:"id" example:"1"`
	ProviderUUID string `json:"provider_uuid" example:"uuid-123"`
	ProviderName string `json:"provider_name" example:"openai"`
	Model        string `json:"model" example:"gpt-4"`
	Scenario     string `json:"scenario" example:"openai"`
	RuleUUID     string `json:"rule_uuid,omitempty" example:"rule-uuid"`
	RequestModel string `json:"request_model,omitempty" example:"gpt-4"`
	Timestamp    string `json:"timestamp" example:"2025-01-10T12:00:00Z"`
	InputTokens  int    `json:"input_tokens" example:"1000"`
	OutputTokens int    `json:"output_tokens" example:"500"`
	TotalTokens  int    `json:"total_tokens" example:"1500"`
	Status       string `json:"status" example:"success"`
	ErrorCode    string `json:"error_code,omitempty"`
	LatencyMs    int    `json:"latency_ms" example:"1200"`
	Streamed     bool   `json:"streamed" example:"true"`
}

// UsageRecordsResponse represents the response for usage records
type UsageRecordsResponse struct {
	Meta UsageRecordsMeta      `json:"meta"`
	Data []UsageRecordResponse `json:"data"`
}

// UsageRecordsMeta represents metadata for usage records response
type UsageRecordsMeta struct {
	Total  int `json:"total" example:"1000"`
	Limit  int `json:"limit" example:"50"`
	Offset int `json:"offset" example:"0"`
}

// DeleteOldRecordsRequest represents the request to delete old usage records
type DeleteOldRecordsRequest struct {
	OlderThanDays int `json:"older_than_days" binding:"required,min=1" description:"Delete records older than this many days" example:"90"`
}

// DeleteOldRecordsResponse represents the response for deleting old records
type DeleteOldRecordsResponse struct {
	Message      string `json:"message" example:"Records deleted successfully"`
	DeletedCount int64  `json:"deleted_count" example:"1500"`
	CutoffDate   string `json:"cutoff_date" example:"2024-10-13T00:00:00Z"`
}

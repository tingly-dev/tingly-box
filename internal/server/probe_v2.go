package server

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProbeTarget defines the target type for probe
type ProbeTarget string

const (
	ProbeV2TargetRule           ProbeTarget = "rule"
	ProbeV2TargetProvider       ProbeTarget = "provider"
	ProbeV2TargetProviderConfig ProbeTarget = "provider_config"
)

// ProbeMode defines the test mode
type ProbeMode string

const (
	ProbeV2ModeSimple    ProbeMode = "simple"
	ProbeV2ModeStreaming ProbeMode = "streaming"
	ProbeV2ModeTool      ProbeMode = "tool"
)

// ProbeV2Request represents a Probe V2 request
type ProbeV2Request struct {
	// Target type: rule, provider, or provider_config
	TargetType ProbeTarget `json:"target_type" binding:"required"`

	// Rule test (required when target_type is rule)
	Scenario string `json:"scenario,omitempty" example:"anthropic"`
	RuleUUID string `json:"rule_uuid,omitempty" binding:"required_if=TargetType rule"`

	// Provider test (required when target_type is provider)
	ProviderUUID string `json:"provider_uuid,omitempty" binding:"required_if=TargetType provider"`
	Model        string `json:"model,omitempty" binding:"required_if=TargetType provider"`

	// Provider config test (required when target_type is provider_config)
	Name     string `json:"name,omitempty"`
	APIBase  string `json:"api_base,omitempty"`
	APIStyle string `json:"api_style,omitempty"`
	Token    string `json:"token,omitempty"`

	// Test mode
	TestMode ProbeMode `json:"test_mode" binding:"required"`

	// Optional custom message (overrides preset)
	Message string `json:"message,omitempty"`
}

// ProbeV2Response represents a Probe V2 response
type ProbeV2Response struct {
	Success bool         `json:"success"`
	Error   *ErrorDetail `json:"error,omitempty"`
	Data    *ProbeV2Data `json:"data,omitempty"`
}

// ProbeV2Data represents the probe result data
type ProbeV2Data struct {
	Content    string            `json:"content,omitempty"`
	Usage      *ProbeV2Usage     `json:"usage,omitempty"`
	ToolCalls  []ProbeV2ToolCall `json:"tool_calls,omitempty"`
	LatencyMs  int64             `json:"latency_ms"`
	RequestURL string            `json:"request_url,omitempty"`
}

// ProbeV2Usage represents token usage
type ProbeV2Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ProbeV2ToolCall represents a tool call in probe response
type ProbeV2ToolCall struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// ProbeV2ResponseChunk represents a streaming response chunk
type ProbeV2ResponseChunk struct {
	Type      string        `json:"type"` // content, error, done
	Content   string        `json:"content,omitempty"`
	Error     string        `json:"error,omitempty"`
	Usage     *ProbeV2Usage `json:"usage,omitempty"`
	LatencyMs int64         `json:"latency_ms,omitempty"`
}

// validateProbeV2Request validates the probe request
func validateProbeV2Request(req *ProbeV2Request) error {
	switch req.TargetType {
	case ProbeV2TargetRule:
		if req.Scenario == "" {
			return &ValidationError{Field: "scenario", Message: "scenario is required for rule test"}
		}
		if req.RuleUUID == "" {
			return &ValidationError{Field: "rule_uuid", Message: "rule_uuid is required for rule test"}
		}
	case ProbeV2TargetProvider:
		if req.ProviderUUID == "" {
			return &ValidationError{Field: "provider_uuid", Message: "provider_uuid is required for provider test"}
		}
		if req.Model == "" {
			return &ValidationError{Field: "model", Message: "model is required for provider test"}
		}
	case ProbeV2TargetProviderConfig:
		if req.APIBase == "" {
			return &ValidationError{Field: "api_base", Message: "api_base is required for provider config test"}
		}
		if req.APIStyle == "" {
			return &ValidationError{Field: "api_style", Message: "api_style is required for provider config test"}
		}
		if req.Token == "" {
			return &ValidationError{Field: "token", Message: "token is required for provider config test"}
		}
	default:
		return &ValidationError{Field: "target_type", Message: "target_type must be 'rule', 'provider', or 'provider_config'"}
	}

	// Validate test mode
	switch req.TestMode {
	case ProbeV2ModeSimple, ProbeV2ModeStreaming, ProbeV2ModeTool:
		// Valid modes
	default:
		return &ValidationError{Field: "test_mode", Message: "test_mode must be 'simple', 'streaming', or 'tool'"}
	}

	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// getProbeMessage returns the probe message based on test mode
func getProbeMessage(mode ProbeMode, customMsg string) string {
	if customMsg != "" {
		return customMsg
	}

	switch mode {
	case ProbeV2ModeTool:
		return "Please use the add_numbers tool to calculate 123 + 456."
	default:
		return "Hello, this is a test message. Please respond with a short greeting."
	}
}

// getScenarioEndpoint returns the API endpoint for a given scenario
func getScenarioEndpoint(scenario string) (endpoint string, apiStyle protocol.APIStyle) {
	endpoint = fmt.Sprintf("/tingly/%s", scenario)
	switch typ.RuleScenario(scenario) {
	case typ.ScenarioAnthropic:
		fallthrough
	case typ.ScenarioOpenCode, typ.ScenarioClaudeCode:
		apiStyle = protocol.APIStyleAnthropic
	default:
		apiStyle = protocol.APIStyleOpenAI
	}
	return endpoint, apiStyle
}

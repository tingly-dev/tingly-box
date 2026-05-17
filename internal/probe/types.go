// Package probe contains the decoupled, server-independent half of the probe
// subsystem: request types, result/data types, in-memory cache, and pure
// helpers. The strategy implementations (Adaptive/Lightweight/V2) still live
// in internal/server because they remain coupled to *Server; they will be
// moved in a follow-up once that coupling is broken.
package probe

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProbeRequest represents the request to probe/test a provider and model.
type ProbeRequest struct {
	Provider string `json:"provider" binding:"required" description:"Provider UUID to test against" example:"550e8400-e29b-41d4-a716-446655440000"`
	Model    string `json:"model" binding:"required" description:"Model name to test against" example:"gpt-4-latest"`
}

// ProbeProviderRequest represents the request to probe/test a provider's API key and connectivity.
type ProbeProviderRequest struct {
	Name     string `json:"name" binding:"required" description:"Provider name" example:"openai"`
	APIBase  string `json:"api_base" binding:"required" description:"API base URL" example:"https://api.openai.com/v1"`
	APIStyle string `json:"api_style" binding:"required,oneof=openai anthropic" description:"API style" example:"openai"`
	Token    string `json:"token" binding:"required" description:"API token to test" example:"sk-..."`
}

// ProbeProviderResponseData represents the data returned from provider probing.
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

// LightweightProbeRequest represents a lightweight probe request for key validation.
type LightweightProbeRequest struct {
	Name     string `json:"name" binding:"required" description:"Provider name" example:"openai"`
	APIBase  string `json:"api_base" binding:"required" description:"API base URL" example:"https://api.openai.com/v1"`
	APIStyle string `json:"api_style" binding:"required,oneof=openai anthropic google" description:"API style" example:"openai"`
	Token    string `json:"token" binding:"required" description:"API token to test" example:"sk-..."`
	AuthType string `json:"auth_type,omitempty" description:"Auth type (e.g., api_key, oauth)" example:"api_key"`
}

// LightweightProbeResponseData represents the data returned from lightweight probing.
type LightweightProbeResponseData struct {
	Valid   bool   `json:"valid" example:"true"`
	Message string `json:"message" example:"Connection test completed"`

	OptionsSuccess      bool   `json:"options_success" example:"true"`
	OptionsMessage      string `json:"options_message,omitempty" example:"OPTIONS request successful"`
	OptionsResponseTime int64  `json:"options_response_time_ms,omitempty" example:"45"`

	ModelsSuccess      bool   `json:"models_success" example:"true"`
	ModelsMessage      string `json:"models_message,omitempty" example:"Models endpoint accessible"`
	ModelsResponseTime int64  `json:"models_response_time_ms,omitempty" example:"250"`
	ModelsCount        int    `json:"models_count,omitempty" example:"150"`

	ChatSuccess      bool   `json:"chat_success,omitempty" example:"true"`
	ChatMessage      string `json:"chat_message,omitempty" example:"Chat endpoint accessible"`
	ChatResponseTime int64  `json:"chat_response_time_ms,omitempty" example:"180"`

	ResponsesSuccess      bool   `json:"responses_success,omitempty" example:"true"`
	ResponsesMessage      string `json:"responses_message,omitempty" example:"Responses API endpoint accessible"`
	ResponsesResponseTime int64  `json:"responses_response_time_ms,omitempty" example:"200"`

	Provider string `json:"provider" example:"openai"`
	APIBase  string `json:"api_base" example:"https://api.openai.com/v1"`
	APIStyle string `json:"api_style" example:"openai"`

	Warning string `json:"warning,omitempty" example:"Models endpoint not supported for this provider type"`
}

// ModelProbeRequest represents the request to probe a specific model.
type ModelProbeRequest struct {
	ProviderUUID string `json:"provider_uuid" binding:"required" description:"Provider UUID to probe" example:"uuid-123"`
	ModelID      string `json:"model_id" binding:"required" description:"Model ID to probe" example:"gpt-4"`
	ForceRefresh bool   `json:"force_refresh" description:"Force new probe even if cached" example:"false"`
}

// ProbeTarget defines the target type for probe.
type ProbeTarget string

const (
	ProbeV2TargetRule           ProbeTarget = "rule"
	ProbeV2TargetProvider       ProbeTarget = "provider"
	ProbeV2TargetProviderConfig ProbeTarget = "provider_config"
)

// ProbeMode defines the test mode.
type ProbeMode string

const (
	ProbeV2ModeSimple    ProbeMode = "simple"
	ProbeV2ModeStreaming ProbeMode = "streaming"
	ProbeV2ModeTool      ProbeMode = "tool"
)

// ProbeV2Request represents a Probe V2 request.
type ProbeV2Request struct {
	TargetType ProbeTarget `json:"target_type" binding:"required"`

	Scenario string `json:"scenario,omitempty" example:"anthropic"`
	RuleUUID string `json:"rule_uuid,omitempty" binding:"required_if=TargetType rule"`

	ProviderUUID string `json:"provider_uuid,omitempty" binding:"required_if=TargetType provider"`
	Model        string `json:"model,omitempty" binding:"required_if=TargetType provider"`

	Name     string `json:"name,omitempty"`
	APIBase  string `json:"api_base,omitempty"`
	APIStyle string `json:"api_style,omitempty"`
	Token    string `json:"token,omitempty"`

	TestMode ProbeMode `json:"test_mode" binding:"required"`

	Message string `json:"message,omitempty"`
}

// ProbeV2Data is an alias to client.ProbeResult — the canonical SDK-level
// probe result. Aliased so service-layer Response wrappers and swagger
// registrations can reference a name in this package without re-importing
// internal/client.
type ProbeV2Data = client.ProbeResult

// ProbeV2ResponseChunk represents a streaming response chunk.
type ProbeV2ResponseChunk struct {
	Type      string `json:"type"` // content, error, done
	Content   string `json:"content,omitempty"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`

	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// ValidationError represents a probe-request validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// ValidateProbeV2Request validates a probe v2 request payload.
func ValidateProbeV2Request(req *ProbeV2Request) error {
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

	switch req.TestMode {
	case ProbeV2ModeSimple, ProbeV2ModeStreaming, ProbeV2ModeTool:
	default:
		return &ValidationError{Field: "test_mode", Message: "test_mode must be 'simple', 'streaming', or 'tool'"}
	}

	return nil
}

// ProbeMessage returns the probe message body based on test mode, with an
// optional caller-provided override.
func ProbeMessage(mode ProbeMode, customMsg string) string {
	if customMsg != "" {
		return customMsg
	}

	switch mode {
	case ProbeV2ModeTool:
		return "Please use the bash tool to list the current directory contents with 'ls -la'."
	default:
		return "Hello, this is a test message. Please respond with a short greeting."
	}
}

// ScenarioEndpoint returns the API endpoint and api-style for a scenario name.
func ScenarioEndpoint(scenario string) (endpoint string, apiStyle protocol.APIStyle) {
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

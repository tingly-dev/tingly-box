// Package probe contains the decoupled, server-independent half of the probe
// subsystem: request types, result/data types, in-memory cache, the E2E and
// Lightweight strategies, and pure helpers. The Adaptive strategy still
// lives in internal/server because it remains coupled to *Server; it will
// be moved in a follow-up once that coupling is broken.
package probe

import (
	"fmt"

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

// E2ETarget defines the target type for probe.
type E2ETarget string

const (
	E2ETargetRule           E2ETarget = "rule"
	E2ETargetProvider       E2ETarget = "provider"
	E2ETargetProviderConfig E2ETarget = "provider_config"
)

// E2EMode defines the test mode.
type E2EMode string

const (
	E2EModeSimple    E2EMode = "simple"
	E2EModeStreaming E2EMode = "streaming"
	E2EModeTool      E2EMode = "tool"
)

// E2ERequest represents a Probe V2 request.
type E2ERequest struct {
	TargetType E2ETarget `json:"target_type" binding:"required"`

	Scenario string `json:"scenario,omitempty" example:"anthropic"`
	RuleUUID string `json:"rule_uuid,omitempty" binding:"required_if=TargetType rule"`

	ProviderUUID string `json:"provider_uuid,omitempty" binding:"required_if=TargetType provider"`
	Model        string `json:"model,omitempty" binding:"required_if=TargetType provider"`

	Name     string `json:"name,omitempty"`
	APIBase  string `json:"api_base,omitempty"`
	APIStyle string `json:"api_style,omitempty"`
	Token    string `json:"token,omitempty"`

	TestMode E2EMode `json:"test_mode" binding:"required"`

	Message string `json:"message,omitempty"`

	// Direct skips the TB loopback and calls the upstream provider directly.
	// Only meaningful for target_type="provider". Use this to isolate whether
	// a failure is in the upstream provider or in TB's own middleware stack.
	Direct bool `json:"direct,omitempty"`
}

// E2EData is an alias to ProbeResult — the canonical SDK-level probe result.
// Aliased so service-layer Response wrappers and swagger registrations can
// keep referring to the historical E2EData name.
type E2EData = ProbeResult

// E2EResponseChunk represents a streaming response chunk.
type E2EResponseChunk struct {
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

// ValidateE2ERequest validates a probe v2 request payload.
func ValidateE2ERequest(req *E2ERequest) error {
	switch req.TargetType {
	case E2ETargetRule:
		if req.Scenario == "" {
			return &ValidationError{Field: "scenario", Message: "scenario is required for rule test"}
		}
		if req.RuleUUID == "" {
			return &ValidationError{Field: "rule_uuid", Message: "rule_uuid is required for rule test"}
		}
	case E2ETargetProvider:
		if req.ProviderUUID == "" {
			return &ValidationError{Field: "provider_uuid", Message: "provider_uuid is required for provider test"}
		}
		if req.Model == "" {
			return &ValidationError{Field: "model", Message: "model is required for provider test"}
		}
	case E2ETargetProviderConfig:
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
	case E2EModeSimple, E2EModeStreaming, E2EModeTool:
	default:
		return &ValidationError{Field: "test_mode", Message: "test_mode must be 'simple', 'streaming', or 'tool'"}
	}

	return nil
}

// E2EMessage returns the probe message body based on test mode, with an
// optional caller-provided override.
func E2EMessage(mode E2EMode, customMsg string) string {
	if customMsg != "" {
		return customMsg
	}

	switch mode {
	case E2EModeTool:
		return "Please use the bash tool to list the current directory contents with 'ls -la'."
	default:
		return "Hello, this is a test message. Please respond with a short greeting."
	}
}

// ScenarioEndpoint returns the API endpoint and api-style for a scenario name.
// The endpoint path preserves the full scenario (including any "base:profile"
// suffix, e.g. "claude_code:p1"), while the api-style is resolved from the base
// scenario so profiled scenarios map to the correct SDK.
func ScenarioEndpoint(scenario string) (endpoint string, apiStyle protocol.APIStyle) {
	endpoint = fmt.Sprintf("/tingly/%s", scenario)
	switch typ.RuleScenario(scenario).Base() {
	case typ.ScenarioAnthropic, typ.ScenarioOpenCode, typ.ScenarioClaudeCode:
		apiStyle = protocol.APIStyleAnthropic
	default:
		apiStyle = protocol.APIStyleOpenAI
	}
	return endpoint, apiStyle
}

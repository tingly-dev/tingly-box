// Package probe contains the decoupled, server-independent half of the probe
// subsystem: request types, result/data types, in-memory cache, the E2E and
// Lightweight strategies, and pure helpers. The Adaptive strategy still
// lives in internal/server because it remains coupled to *Server; it will
// be moved in a follow-up once that coupling is broken.
//
// Two result types answer two different questions and are deliberately NOT
// unified:
//
//   - Result (alias E2EData) — SDK-level truth for one real round-trip through
//     the production client methods. Carries normalized token Usage, lifted
//     tool calls, and the routing journey. Returned by the E2E prober.
//   - LightweightProbeResponseData — a per-endpoint connectivity matrix
//     (OPTIONS / models / chat / responses success+latency). Advisory only,
//     no usage, never blocks onboarding. Returned by the Lightweight prober.
//
// Both probers share the low-level SDK dispatch helpers (probeOpenAIChat,
// probeOptions, …). A probe never invents a model: if the request omits one
// and the provider record carries none, resolution fails explicitly rather
// than guessing.
package probe

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Result is the canonical SDK-level probe result, shared by the E2E and
// lightweight probe strategies. It doubles as the JSON payload returned by the
// probe HTTP endpoints (exposed under the E2EData alias).
type Result struct {
	// Basic fields
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
	Content      string `json:"content,omitempty"`
	LatencyMs    int64  `json:"latency_ms"`
	ErrorMessage string `json:"error_message,omitempty"`

	// Streaming mode indicator (true for streaming probes; redundant with the
	// caller's test_mode but kept explicit so consumers don't have to infer the
	// response shape from Content).
	Stream bool `json:"stream,omitempty"`

	// Usage is the normalized token usage for the probe round-trip, parsed via
	// internal/protocol/usage from each provider's native usage struct. It uses
	// the canonical protocol.TokenUsage shape (input_tokens / output_tokens /
	// cache_read_tokens / cache_write_tokens / reasoning_tokens / system_tokens)
	// — the same vocabulary the rest of the codebase emits and the frontend
	// renders. Nil for cache hits, Google probes (out of scope), and providers
	// that don't report usage (notably most streaming responses unless usage is
	// requested).
	Usage *protocol.TokenUsage `json:"usage,omitempty"`

	// Tool calls lifted out of the response (tool mode only). Empty for
	// non-tool probes and for providers whose tool calls couldn't be extracted.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// Request URL (for debugging)
	RequestURL string `json:"request_url,omitempty"`

	// Routing trace — populated for TB-loopback probes (provider and rule targets).
	// Empty for direct probes and provider_config probes.
	SelectedProvider     string `json:"selected_provider,omitempty"`
	SelectedProviderUUID string `json:"selected_provider_uuid,omitempty"`
	SelectedModel        string `json:"selected_model,omitempty"`
	RoutingSource        string `json:"routing_source,omitempty"`
	MatchedSmartRule     *int   `json:"matched_smart_rule,omitempty"` // nil = none, ≥0 = index

	// Execution-level facts — the real upstream endpoint TB used, the matched
	// rule, and the flags it applied. Populated for TB-loopback probes.
	UpstreamAPI     string `json:"upstream_api,omitempty"`
	UpstreamURL     string `json:"upstream_url,omitempty"`
	MatchedRule     string `json:"matched_rule,omitempty"`
	MatchedRuleDesc string `json:"matched_rule_desc,omitempty"`
	AppliedFlags    string `json:"applied_flags,omitempty"`
}

// ToolCall represents a tool call in a probe response.
type ToolCall struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// toProbeResult builds a Result carrying the raw (JSON-marshaled) upstream
// response for a successful probe. latencyMs is the pure upstream round-trip
// time (measured by the SDK probe helper, not the HTTP handler). usage, when
// non-nil, is the normalized token usage (canonical protocol.TokenUsage shape).
// toolCalls carries any tool calls lifted from the response (tool mode).
func toProbeResult(content string, latencyMs int64, requestURL string, isStreaming bool, usage *protocol.TokenUsage, toolCalls []ToolCall) *Result {
	return &Result{
		Success:    true,
		Content:    content,
		LatencyMs:  latencyMs,
		RequestURL: requestURL,
		Stream:     isStreaming,
		Usage:      usage,
		ToolCalls:  toolCalls,
	}
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

	// Endpoint forces which OpenAI endpoint to probe: "chat" or "responses".
	// Only meaningful for OpenAI-style providers; ignored otherwise. Empty
	// (default) keeps the existing behavior: Codex OAuth providers probe
	// Responses, everything else probes Chat. Used to test whether a
	// provider genuinely supports Responses before a rule starts routing
	// there (e.g. the Codex-page "enable native Responses" toggle).
	Endpoint string `json:"endpoint,omitempty" example:"responses"`
}

// E2EData is an alias to Result — the canonical SDK-level probe result.
// Aliased so service-layer Response wrappers and swagger registrations can
// keep referring to the historical E2EData name.
type E2EData = Result

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

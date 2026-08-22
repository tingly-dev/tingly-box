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
	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
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

	// Request-echo axes — the exact stream/tool/direct/protocol/thinking
	// combination that produced this result. Stream is redundant with the
	// caller's test_mode but kept explicit so consumers don't have to infer the
	// response shape from Content; the rest let a consumer reopening a stored
	// result restore the control state that produced it (the frontend probe
	// dialog does not persist axes, so the echo is the only source).
	Stream   bool          `json:"stream,omitempty"`
	Tool     bool          `json:"tool,omitempty"`
	Direct   bool          `json:"direct,omitempty"`
	Protocol ProbeProtocol `json:"protocol,omitempty"`
	Thinking ThinkingLevel `json:"thinking,omitempty"`
	Vision   VisionChannel `json:"vision,omitempty"`

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

// ProbeProtocol is a concrete client-side wire protocol for a probe. There is
// deliberately no "auto" value — the panel always speaks a concrete protocol,
// defaulting to the provider's primary one (ResolveProbeProtocol).
type ProbeProtocol string

const (
	// ProtocolOpenAIChat probes via OpenAI Chat Completions.
	ProtocolOpenAIChat ProbeProtocol = "openai_chat"
	// ProtocolOpenAIResponses probes via the OpenAI Responses API.
	ProtocolOpenAIResponses ProbeProtocol = "openai_responses"
	// ProtocolAnthropic probes via the Anthropic Messages API.
	ProtocolAnthropic ProbeProtocol = "anthropic_v1"
)

// ProtocolFamily maps a ProbeProtocol onto the client API style it implies.
func (p ProbeProtocol) Family() protocol.APIStyle {
	switch p {
	case ProtocolAnthropic:
		return protocol.APIStyleAnthropic
	case ProtocolOpenAIChat, ProtocolOpenAIResponses:
		return protocol.APIStyleOpenAI
	default:
		return ""
	}
}

// ThinkingLevel is the probe-facing subset of the canonical thinking-effort
// ladder (internal/protocol/thinking). Orthogonal to the Stream/Tool axes —
// composes with both streaming and non-streaming probes. "none" (and the empty string) send
// no thinking param; the other levels map to each provider's native thinking
// knob (Anthropic budget_tokens, OpenAI reasoning_effort, Gemini
// thinking_budget) via thinking.BudgetMapping / the effort value.
//
// Kept narrower than the full ladder (no minimal/xhigh/max) so the UI stays a
// 4-option control; extensible to the remaining levels later without breaking
// callers.
type ThinkingLevel = thinking.Level

const (
	ThinkingNone   ThinkingLevel = "none"
	ThinkingLow    ThinkingLevel = thinking.LevelLow
	ThinkingMedium ThinkingLevel = thinking.LevelMedium
	ThinkingHigh   ThinkingLevel = thinking.LevelHigh
)

// VisionChannel identifies where an image rides in a request: the user message or
// a tool-result turn. These are exactly the two rows of the issue #1606
// control matrix — user-channel images and tool-channel images fail
// independently, so a vision check must be able to exercise each.
type VisionChannel string

const (
	// VisionNone sends no image (the default; "" normalizes to this).
	VisionNone VisionChannel = "none"
	// VisonUser puts the image in the user message content.
	VisonUser VisionChannel = "user"
	// VisionTool returns the image from a synthetic tool round
	// (assistant tool call → tool result carrying the image), the shape
	// agent frameworks use for screenshots.
	VisionTool VisionChannel = "tool"
)

// Enabled reports whether the channel carries an image. "" and "none" both
// mean "send no image".
func (c VisionChannel) Enabled() bool {
	return c == VisonUser || c == VisionTool
}

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

	// Stream and Tool are the orthogonal axes describing the probe shape.
	// nil normalizes to false. Tool does NOT force either stream value — both
	// combinations are valid (non-stream lifts structured tool_calls; stream
	// keeps raw chunks).
	Stream *bool `json:"stream,omitempty" example:"true"`
	Tool   *bool `json:"tool,omitempty" example:"false"`

	Message string `json:"message,omitempty"`

	// Direct skips the TB loopback and calls the upstream provider directly.
	// Only meaningful for target_type="provider". Use this to isolate whether
	// a failure is in the upstream provider or in TB's own middleware stack.
	Direct bool `json:"direct,omitempty"`

	// Protocol forces the client-side wire protocol: openai_chat,
	// openai_responses, or anthropic_v1. No "auto" value — empty (default)
	// keeps the provider's primary protocol (its APIStyle, plus the Codex
	// OAuth → Responses default for OpenAI providers). For dual-base
	// providers the matching dual URL is selected; for through-TB probes the
	// loopback speaks the requested protocol and TB's transform pipeline
	// handles the upstream exactly as production traffic does.
	// Not supported for rule targets (the rule's scenario fixes the protocol).
	Protocol ProbeProtocol `json:"protocol,omitempty" example:"openai_responses"`

	// Thinking sets the extended-thinking effort for the probe. Orthogonal to
	// Stream/Tool — composes with both streaming and non-streaming probes. "none"
	// (and the empty string, the default) sends no thinking param; "low"/
	// "medium"/"high" map to each provider's native thinking knob via
	// internal/protocol/thinking. Used to verify a model/provider actually
	// returns reasoning tokens before trusting it with a rule.
	Thinking ThinkingLevel `json:"thinking,omitempty" example:"medium"`

	// Vision attaches the canonical probe image (internal/protocol/vision) to
	// the request: "user" puts it in the user message, "tool" returns it from
	// a synthetic tool round — the two channels of issue #1606. "none" (and
	// the empty string, the default) sends no image. Orthogonal to Stream and
	// Protocol. A vision-capable route answers the fixture prompt with "red";
	// any other answer reveals a drop or corruption along the path. Not
	// supported for Google-style targets.
	Vision VisionChannel `json:"vision,omitempty" example:"user"`
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

	switch req.Protocol {
	case "", ProtocolOpenAIChat, ProtocolOpenAIResponses, ProtocolAnthropic:
	default:
		return &ValidationError{Field: "protocol", Message: "protocol must be 'openai_chat', 'openai_responses', or 'anthropic_v1'"}
	}

	// A rule's scenario already fixes the wire protocol; an override there
	// would be silently ignored, so reject it instead.
	if req.TargetType == E2ETargetRule && req.Protocol != "" {
		return &ValidationError{Field: "protocol", Message: "protocol override is not supported for rule targets (fixed by the rule's scenario)"}
	}

	// Thinking is optional; empty normalizes to "none". Only the probe-facing
	// subset of the ladder is accepted (minimal/xhigh/max are intentionally
	// rejected here to keep the UI a 4-option control).
	switch req.Thinking {
	case "", ThinkingNone, ThinkingLow, ThinkingMedium, ThinkingHigh:
	default:
		return &ValidationError{Field: "thinking", Message: "thinking must be 'none', 'low', 'medium', or 'high'"}
	}

	// Vision is optional; empty normalizes to "none". Google-style targets
	// are rejected later at dispatch (the style is only known after target
	// resolution).
	switch req.Vision {
	case "", VisionNone, VisonUser, VisionTool:
	default:
		return &ValidationError{Field: "vision", Message: "vision must be 'none', 'user', or 'tool'"}
	}

	return nil
}

// ResolveAxes returns the effective stream/tool decisions (nil → false).
func (req *E2ERequest) ResolveAxes() (stream, tool bool) {
	if req.Stream != nil {
		stream = *req.Stream
	}
	if req.Tool != nil {
		tool = *req.Tool
	}
	return stream, tool
}

// ResolveClientStyle returns the client-side API style the probe should speak,
// after applying the protocol override (if any) to the provider's own style.
// Returns the provider style unchanged for "", google, and unsupported
// combinations — callers decide whether that is an error.
func (req *E2ERequest) ResolveClientStyle(providerStyle protocol.APIStyle) protocol.APIStyle {
	if req.Protocol == "" {
		return providerStyle
	}
	return req.Protocol.Family()
}

// ResolveOpenAIEndpointOverride translates Protocol into the endpointOverride
// consumed by resolveOpenAIProbeEndpoint ("chat"/"responses", or "" to keep
// the provider's default).
func (req *E2ERequest) ResolveOpenAIEndpointOverride() string {
	switch req.Protocol {
	case ProtocolOpenAIChat:
		return "chat"
	case ProtocolOpenAIResponses:
		return "responses"
	}
	return ""
}

// E2EMessage returns the probe message body: a caller-provided override when
// given, otherwise a default message chosen by whether the probe attaches
// tools.
func E2EMessage(tool bool, customMsg string) string {
	if customMsg != "" {
		return customMsg
	}
	if tool {
		return "Please use the bash tool to list the current directory contents with 'ls -la'."
	}
	return "Hello, this is a test message. Please respond with a short greeting."
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

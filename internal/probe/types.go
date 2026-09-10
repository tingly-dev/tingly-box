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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

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
// Mirrors the rule flag's thinking_effort options (.design/rule-flags.md):
// low/medium/high/max. "minimal"/"xhigh" are deliberately excluded — outside
// a handful of the newest OpenAI/Anthropic models they collapse onto
// low/high/max anyway, so probing them wouldn't exercise anything the
// low/high/max probes don't already cover.
type ThinkingLevel = thinking.Level

const (
	ThinkingNone   ThinkingLevel = "none"
	ThinkingLow    ThinkingLevel = thinking.LevelLow
	ThinkingMedium ThinkingLevel = thinking.LevelMedium
	ThinkingHigh   ThinkingLevel = thinking.LevelHigh
	ThinkingMax    ThinkingLevel = thinking.LevelMax
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
	// "medium"/"high"/"max" map to each provider's native thinking knob via
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

	// Request is a raw client request body in one of the three client
	// protocols (RequestProtocol says which) — what a real client would send
	// TB. The probe parses it with the same SDK decoders TB's handlers use
	// for inbound traffic, fills in what the target decides (model; Anthropic
	// max_tokens when absent) and sends it on that protocol's wire, so text,
	// images, tools and tool results, cache breakpoints, thinking — anything
	// the protocol accepts — travel as-is. Through TB, TB's own transform
	// chain converts it to the upstream exactly as for production traffic.
	// A raw request replaces the fixture: Message and the Tool / Vision /
	// Thinking knobs (which only shape the fixture) are rejected alongside
	// it; Stream still applies. Provider targets speak RequestProtocol on
	// the wire (Protocol, if given, must agree); rule targets require the
	// scenario's protocol family.
	Request         json.RawMessage `json:"request,omitempty" swaggertype:"object"`
	RequestProtocol ProbeProtocol   `json:"request_protocol,omitempty" example:"anthropic_v1"`

	// Flags is a per-request rule-flag overlay (Bench page). Only the keys
	// present are applied; each replaces the value resolved from rule +
	// scenario inheritance for this one request, and nothing is persisted.
	// Keys and value types are validated against typ.RuleFlagRegistry().
	// Through-TB only: flags are TB middleware, so a direct probe cannot
	// carry them (rejected, never silently ignored). Travels to the loopback
	// handler in the X-Tingly-Probe-Flags header.
	Flags typ.FlagOverlay `json:"flags,omitempty" swaggertype:"object"`

	// Headers sets or overrides HTTP headers on the outgoing probe request
	// (an empty value removes the header). Applied by a probe-only round
	// tripper after the SDK built the request, so it wins over the SDK's
	// own headers and the probe pins; the rendered cURL applies the same.
	Headers map[string]string `json:"headers,omitempty"`

	// Routing picks how a rule target enters TB. "" / "natural" (default)
	// sends only the rule's request model to the scenario endpoint and lets
	// TB match the rule exactly as it would for a real client — the full
	// production chain; the Journey reports which rule actually matched,
	// which may differ from the one picked. "pinned" forces the chosen rule
	// via X-Tingly-Probe-Rule (skipping rule matching, everything else is
	// production) — for testing a rule whose request model collides with
	// another rule's, or one that is not active. Rule targets only; provider
	// targets are pinned by definition (X-Tingly-Probe-Service).
	Routing ProbeRouting `json:"routing,omitempty" example:"pinned"`
}

// ProbeRouting selects how a rule target enters TB (see E2ERequest.Routing).
type ProbeRouting string

const (
	// RoutingNatural lets TB match the rule from the request model, as for real traffic (default).
	RoutingNatural ProbeRouting = "natural"
	// RoutingPinned forces the chosen rule via X-Tingly-Probe-Rule.
	RoutingPinned ProbeRouting = "pinned"
)

// Pinned reports whether the rule target should be forced rather than matched.
func (r ProbeRouting) Pinned() bool { return r == RoutingPinned }

// Customized reports whether the request departs from the plain fixture
// shape (raw request, flag overlay, header overrides). Such probes are never
// served from the endpoint capability cache — the whole point of customizing
// a probe is to watch what that exact request does.
func (req *E2ERequest) Customized() bool {
	return req.HasRawRequest() || len(req.Flags) > 0 || len(req.Headers) > 0
}

// HasRawRequest reports whether the probe sends a caller-supplied request
// instead of the fixture.
func (req *E2ERequest) HasRawRequest() bool { return len(req.Request) > 0 }

// WireProtocol is the client protocol the probe speaks: the raw request's
// protocol when one is given, else the Protocol override, else "" (the
// target's primary protocol).
func (req *E2ERequest) WireProtocol() ProbeProtocol {
	if req.HasRawRequest() {
		return req.RequestProtocol
	}
	return req.Protocol
}

// parseRawRequest decodes the raw request with the SDK decoder of its
// protocol — the same decoders TB's handlers use — and returns the typed
// params (one of *anthropic.MessageNewParams, *openai.ChatCompletionNewParams,
// *responses.ResponseNewParams).
func (req *E2ERequest) parseRawRequest() (any, error) {
	if !req.HasRawRequest() {
		return nil, nil
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(req.Request, &probe); err != nil {
		return nil, fmt.Errorf("request must be a JSON object: %w", err)
	}
	switch req.RequestProtocol {
	case ProtocolAnthropic:
		var p anthropic.MessageNewParams
		if err := json.Unmarshal(req.Request, &p); err != nil {
			return nil, fmt.Errorf("request is not an Anthropic Messages request: %w", err)
		}
		if len(p.Messages) == 0 {
			return nil, fmt.Errorf("request.messages must contain at least one message")
		}
		return &p, nil
	case ProtocolOpenAIChat:
		var p openai.ChatCompletionNewParams
		if err := json.Unmarshal(req.Request, &p); err != nil {
			return nil, fmt.Errorf("request is not an OpenAI Chat Completions request: %w", err)
		}
		if len(p.Messages) == 0 {
			return nil, fmt.Errorf("request.messages must contain at least one message")
		}
		return &p, nil
	case ProtocolOpenAIResponses:
		// Same preprocessing as the inbound Responses handler: input items
		// need their type fields before the SDK's union decoder accepts them.
		processed, err := protocol.PreprocessInputData(req.Request)
		if err != nil {
			return nil, fmt.Errorf("request is not an OpenAI Responses request: %w", err)
		}
		var p responses.ResponseNewParams
		if err := json.Unmarshal(processed, &p); err != nil {
			return nil, fmt.Errorf("request is not an OpenAI Responses request: %w", err)
		}
		if len(p.Input.OfInputItemList) == 0 && !p.Input.OfString.Valid() {
			return nil, fmt.Errorf("request.input is required")
		}
		return &p, nil
	default:
		return nil, fmt.Errorf("request_protocol must be 'openai_chat', 'openai_responses', or 'anthropic_v1'")
	}
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
	// subset of the ladder is accepted (minimal/xhigh are intentionally
	// rejected — see the ThinkingLevel doc comment).
	switch req.Thinking {
	case "", ThinkingNone, ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingMax:
	default:
		return &ValidationError{Field: "thinking", Message: "thinking must be 'none', 'low', 'medium', 'high', or 'max'"}
	}

	// Vision is optional; empty normalizes to "none". Google-style targets
	// are rejected later at dispatch (the style is only known after target
	// resolution).
	switch req.Vision {
	case "", VisionNone, VisonUser, VisionTool:
	default:
		return &ValidationError{Field: "vision", Message: "vision must be 'none', 'user', or 'tool'"}
	}

	// Flags are TB middleware behaviour; a direct probe bypasses exactly the
	// layer they act in. Silently ignoring them would report "tested" for
	// something that never ran — the worst kind of false success.
	if len(req.Flags) > 0 {
		if req.Direct {
			return &ValidationError{Field: "flags", Message: "flags are TB middleware and cannot apply to a direct probe; switch scope to through-TB"}
		}
		if err := typ.ValidateFlagOverlay(req.Flags); err != nil {
			return &ValidationError{Field: "flags", Message: err.Error()}
		}
	}

	for name := range req.Headers {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, ": \t\r\n") {
			return &ValidationError{Field: "headers", Message: fmt.Sprintf("%q is not a valid header name", name)}
		}
	}

	switch req.Routing {
	case "", RoutingNatural:
	case RoutingPinned:
		if req.TargetType != E2ETargetRule {
			return &ValidationError{Field: "routing", Message: "pinned routing only applies to rule targets (a provider target is pinned by definition)"}
		}
	default:
		return &ValidationError{Field: "routing", Message: "routing must be 'natural' or 'pinned'"}
	}

	// A raw client request replaces the fixture; the fixture knobs and the
	// single-message override have nothing to shape, so reject them rather
	// than silently ignore a setting the user made.
	if req.HasRawRequest() {
		if req.RequestProtocol == "" {
			return &ValidationError{Field: "request_protocol", Message: "request_protocol is required with a raw request"}
		}
		if req.Message != "" {
			return &ValidationError{Field: "message", Message: "message does not apply to a raw request (put the text in the request itself)"}
		}
		if tool := req.Tool; tool != nil && *tool {
			return &ValidationError{Field: "tool", Message: "the tool knob only shapes the fixture; a raw request carries its own tools"}
		}
		if req.Vision.Enabled() {
			return &ValidationError{Field: "vision", Message: "the vision knob only shapes the fixture; put the image in the raw request"}
		}
		if req.Thinking != "" && req.Thinking != ThinkingNone {
			return &ValidationError{Field: "thinking", Message: "the thinking knob only shapes the fixture; set thinking in the raw request"}
		}
		if _, err := req.parseRawRequest(); err != nil {
			return &ValidationError{Field: "request", Message: err.Error()}
		}
		switch req.TargetType {
		case E2ETargetProvider, E2ETargetProviderConfig:
			if req.Protocol != "" && req.Protocol != req.RequestProtocol {
				return &ValidationError{Field: "protocol", Message: "protocol and request_protocol disagree; a raw request is sent on its own protocol"}
			}
		case E2ETargetRule:
			scenario := req.Scenario
			if scenario == "" {
				scenario = string(typ.ScenarioOpenAI)
			}
			if _, style := ScenarioEndpoint(scenario); req.RequestProtocol.Family() != style {
				return &ValidationError{Field: "request_protocol", Message: fmt.Sprintf("scenario %s speaks the %s protocol; the raw request is %s", scenario, style, req.RequestProtocol)}
			}
		}
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
	if p := req.WireProtocol(); p != "" {
		return p.Family()
	}
	return providerStyle
}

// ResolveOpenAIEndpointOverride translates Protocol into the endpointOverride
// consumed by resolveOpenAIProbeEndpoint ("chat"/"responses", or "" to keep
// the provider's default).
func (req *E2ERequest) ResolveOpenAIEndpointOverride() string {
	switch req.WireProtocol() {
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

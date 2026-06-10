package typ

// FlagValueType describes how a rule flag is represented in storage and UI.
type FlagValueType string

const (
	FlagTypeBool   FlagValueType = "bool"
	FlagTypeString FlagValueType = "string"
	FlagTypeEnum   FlagValueType = "enum"
	// FlagTypeInt is a non-negative integer value. The UI renders a numeric
	// text field. Zero is treated as inactive (equivalent to omitempty).
	FlagTypeInt FlagValueType = "int"
	// FlagTypeServiceRef is a {provider, model} pair selected via the model
	// picker. The UI renders a service picker (provider + model); an empty
	// pair is treated as inactive. Backed by a typed struct on RuleFlags, not
	// a scalar.
	FlagTypeServiceRef FlagValueType = "service_ref"
)

// FlagCategory groups flags for presentation in the UI.
type FlagCategory string

const (
	// FlagCategoryApp — flags that target a specific client application (IDE, CLI tool, etc).
	FlagCategoryApp FlagCategory = "app"
	// FlagCategoryRequestOpenAI — request-level adjustments for OpenAI-compatible upstreams:
	// endpoint routing, field rewrites, tool blocking, user-agent overrides.
	FlagCategoryRequestOpenAI FlagCategory = "request_openai"
	// FlagCategoryRequestAnthropic — request-level adjustments for Anthropic-compatible upstreams:
	// message normalisation and other Anthropic-protocol-specific transforms.
	FlagCategoryRequestAnthropic FlagCategory = "request_anthropic"
	// FlagCategoryResponse — flags that modify the response body/stream.
	FlagCategoryResponse FlagCategory = "response"
	// FlagCategoryReasoning — extended-thinking / reasoning-effort controls.
	FlagCategoryReasoning FlagCategory = "reasoning"
	// FlagCategoryRouting — routing / load-balancing behavior (session
	// affinity, etc) that decides which upstream service a request lands on.
	FlagCategoryRouting FlagCategory = "routing"
	// FlagCategoryVision — image/vision handling (vision proxy describer).
	FlagCategoryVision FlagCategory = "vision"
)

// FlagOption is one selectable value for a FlagTypeEnum spec.
type FlagOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FlagSpec describes a single rule-level flag's metadata for the UI catalog.
// Keys must match the JSON tag name on RuleFlags.
type FlagSpec struct {
	Key         string        `json:"key"`
	Label       string        `json:"label"`
	Description string        `json:"description"`
	Type        FlagValueType `json:"type"`
	Category    FlagCategory  `json:"category"`
	// Placeholder is the hint text shown in string-type input fields.
	Placeholder string `json:"placeholder,omitempty"`
	// Options enumerates the selectable values for FlagTypeEnum flags.
	// The first option is treated as the default when the stored value is empty.
	Options []FlagOption `json:"options,omitempty"`
	// Suggestions offers a non-exhaustive list of recommended values for a
	// FlagTypeString flag. Unlike Options (enum) these are not the only legal
	// values — the UI surfaces them as a quick-pick / autocomplete while still
	// allowing free-form input. Used by custom_user_agent to expose the common
	// CLI/agent User-Agent strings (see DefaultUserAgents).
	Suggestions []FlagOption `json:"suggestions,omitempty"`
	// Shared indicates this flag also exists at the scenario level
	// (ScenarioFlags) and participates in scenario→rule inheritance.
	Shared bool `json:"shared,omitempty"`
	// InheritanceMode describes how the scenario-level and rule-level values
	// combine when both are set:
	//   - "or":       bool OR — either level enabling it activates the flag
	//   - "override": rule non-zero/non-empty wins, else scenario default
	// Empty means the flag is rule-only (not shared).
	InheritanceMode string `json:"inheritance_mode,omitempty"`
}

// DefaultUserAgents returns a curated, non-exhaustive list of recommended
// User-Agent strings for the custom_user_agent flag (both rule- and
// scenario-level). The values mirror the vendor-pinned User-Agents the built-in
// clients send (see internal/client/*.go) plus a few widely used CLI/SDK agents,
// so operators can impersonate a known client when an upstream gates on it.
// Label is a human-friendly name; Value is the literal User-Agent header.
func DefaultUserAgents() []FlagOption {
	return []FlagOption{
		{Label: "Claude Code (CLI)", Value: "claude-cli/2.1.86 (external, cli)"},
		{Label: "Codex CLI", Value: "codex_cli_rs/0.20.0"},
		{Label: "OpenClaw", Value: "openclaw/1.0.0"},
		{Label: "Hermes", Value: "hermes-agent/1.0.0"},
		{Label: "OpenAI Python SDK", Value: "OpenAI/Python 1.51.0"},
		// Sentinel preset: send no User-Agent at all (see typ.UserAgentNone).
		{Label: "None (no User-Agent)", Value: UserAgentNone},
	}
}

// RuleFlagRegistry returns the catalog of supported rule flags. The order is
// the recommended display order in the UI — categories are grouped implicitly
// by adjacent entries sharing the same Category value.
func RuleFlagRegistry() []FlagSpec {
	return []FlagSpec{
		// ── Request (OpenAI) ───────────────────────────────────────────────
		{
			Key:             "custom_user_agent",
			Label:           "Custom User-Agent",
			Description:     "Override the outbound User-Agent header sent to the upstream provider. Takes precedence over the provider-level User-Agent for generic OpenAI / Anthropic clients; vendor-specific clients (Claude Code OAuth, Codex, Gemini, Google) keep their dedicated User-Agent. Can also be set scenario-wide (the rule value wins when both are set). Pick a preset to impersonate a known CLI/agent, enter any value, or choose \"None\" to strip the User-Agent header entirely (send no User-Agent).",
			Type:            FlagTypeString,
			Category:        FlagCategoryRequestOpenAI,
			Placeholder:     "e.g. MyApp/1.0",
			Suggestions:     DefaultUserAgents(),
			Shared:          true,
			InheritanceMode: "override",
		},
		{
			Key:         "openai_endpoint_override",
			Label:       "OpenAI endpoint override",
			Description: "Force OpenAI Chat Completions or Responses for this rule, overriding the provider's declared OpenAIEndpointMode default. OpenAI providers only; Anthropic/Google providers ignore this. If the provider declares mode=responses (e.g. Codex), \"chat\" is ignored; if mode=chat, \"responses\" is ignored.",
			Type:        FlagTypeEnum,
			Category:    FlagCategoryRequestOpenAI,
			Options: []FlagOption{
				{Value: "auto", Label: "Auto (use provider default)"},
				{Value: "chat", Label: "Force Chat Completions"},
				{Value: "responses", Label: "Force Responses API"},
			},
		},
		{
			Key:         "use_max_completion_tokens",
			Label:       "OpenAI: Use max_completion_tokens",
			Description: "OpenAI only. Rewrite `max_tokens` → `max_completion_tokens` in the outgoing request. Required by the o1/o3/gpt-5 model family, which rejects the older field name.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryRequestOpenAI,
		},
		{
			Key:         "use_max_tokens",
			Label:       "OpenAI: Use max_tokens (legacy)",
			Description: "OpenAI only. Rewrite `max_completion_tokens` → `max_tokens` in the outgoing request. Use for older OpenAI-compatible providers that do not yet accept the newer field name.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryRequestOpenAI,
		},
		{
			Key:         "block_tools",
			Label:       "Block tools",
			Description: "Comma-separated list of tool names to remove from the request before it is forwarded upstream. Matches the tool name as the client sent it; works across OpenAI Chat, OpenAI Responses, Anthropic, and Google requests.",
			Type:        FlagTypeString,
			Category:    FlagCategoryRequestOpenAI,
			Placeholder: "e.g. web_search,run_terminal_cmd",
		},
		// ── Response ───────────────────────────────────────────────────────
		{
			Key:             "skip_usage",
			Label:           "Skip usage in response",
			Description:     "Strip the `usage` block from responses (both SSE deltas and the final body).",
			Type:            FlagTypeBool,
			Category:        FlagCategoryResponse,
			Shared:          true,
			InheritanceMode: "or",
		},
		// ── Reasoning ──────────────────────────────────────────────────────
		{
			Key:             "thinking_effort",
			Label:           "Thinking",
			Description:     "Single control for extended thinking. \"By Client\" passes the client's thinking config through unchanged. \"Off\" forces thinking disabled. The level values force thinking on with the matching budget — mapped to budget_tokens for Anthropic targets (low 1K / medium 5K / high 20K / max 32K) and to reasoning_effort for OpenAI targets (\"max\" collapses to \"high\").",
			Type:            FlagTypeEnum,
			Category:        FlagCategoryReasoning,
			Shared:          true,
			InheritanceMode: "override",
			Options: []FlagOption{
				{Value: "", Label: "By Client"},
				{Value: "off", Label: "Off"},
				{Value: "low", Label: "Low (~1K tokens)"},
				{Value: "medium", Label: "Medium (~5K tokens)"},
				{Value: "high", Label: "High (~20K tokens)"},
				{Value: "max", Label: "Max (~32K tokens)"},
			},
		},
		// ── Vision ─────────────────────────────────────────────────────────
		{
			Key:         "vision_proxy_service",
			Label:       "Vision Proxy",
			Description: "Describe images via a vision-capable model so text-only downstream models can read them. Applies only to requests matched by this rule. Same effect as the scenario-level Vision Proxy but scoped to this rule; when both are configured, this rule-level service takes precedence.",
			Type:        FlagTypeServiceRef,
			Category:    FlagCategoryVision,
		},
		// ── Routing ────────────────────────────────────────────────────────
		{
			Key:         "session_affinity",
			Label:       "Session affinity",
			Description: "TTL in seconds for session-to-service pinning. Pinning improves cache hit rate → faster responses + lower token costs. Once a session lands on a service, follow-up requests keep hitting that service until the entry expires. 0 disables affinity. Works with any load-balancing tactic; does not require smart routing. Session identity is resolved from Anthropic metadata.user_id, the X-Tingly-Session-ID header, or the client IP. On by default (1800s) for the built-in Claude Code / Claude Desktop / Codex rules.",
			Type:        FlagTypeInt,
			Category:    FlagCategoryRouting,
			Placeholder: "e.g. 3600",
		},
		// ── App ────────────────────────────────────────────────────────────
		{
			Key:         "cursor_compat",
			Label:       "Cursor compatibility",
			Description: "Normalize rich content, gate tools, and strip stream usage for Cursor clients.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryApp,
		},
		{
			Key:         "cursor_compat_auto",
			Label:       "Auto-detect Cursor",
			Description: "Apply cursor compatibility automatically when request headers identify Cursor.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryApp,
		},
		{
			Key:             "claude_code_compat",
			Label:           "Claude Code compatibility",
			Description:     "Normalize Claude Code's mid-conversation \"system\" role messages before forwarding. Claude Code sends system-role entries inside the messages list (a non-standard extension); third-party Anthropic-compatible providers reject that role. This folds each system message, in place, into a neighbouring user turn — backward into the preceding user, or forward into the following user when the previous turn is an assistant — so the request stays valid without producing the consecutive user messages that strict providers also reject. On by default for the built-in Claude Code rules; turn off for native Anthropic fidelity.",
			Type:            FlagTypeBool,
			Category:        FlagCategoryApp,
			Shared:          true,
			InheritanceMode: "or",
		},
		{
			Key:         "clean_header",
			Label:       "Clean Header",
			Description: "Strip x-anthropic-billing-header blocks from system messages before forwarding. Claude Code injects this header for its own billing; it must not leak to third-party providers. On by default for the built-in Claude Code rules, and automatically suppressed when the rule routes to a Claude OAuth provider (whose billing backend consumes the header). Still auto-enabled for claude_desktop during protocol transformation. Turn off only for native Anthropic fidelity.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryApp,
		},
		{
			Key:         "context_1m",
			Label:       "1M Context Window",
			Description: "Enable Anthropic's 1M token context window for supported models (Sonnet 4.6+, Opus 4.6+). Injects the context-1m-2025-08-07 beta flag into the upstream anthropic-beta header; the model name sent to the provider is unchanged. Only enable for models that support the 1M context window.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryRequestAnthropic,
		},
	}
}

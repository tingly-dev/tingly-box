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
	// FlagCategoryRequest — request-level adjustments: transport overrides,
	// OpenAI-specific field rewrites, tool blocking, etc.
	FlagCategoryRequest FlagCategory = "request"
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
}

// RuleFlagRegistry returns the catalog of supported rule flags. The order is
// the recommended display order in the UI — categories are grouped implicitly
// by adjacent entries sharing the same Category value.
func RuleFlagRegistry() []FlagSpec {
	return []FlagSpec{
		// ── Request ────────────────────────────────────────────────────────
		{
			Key:         "custom_user_agent",
			Label:       "Custom User-Agent",
			Description: "Override the outbound User-Agent header sent to the upstream provider. Takes precedence over the provider-level User-Agent for generic OpenAI / Anthropic clients; vendor-specific clients (Claude Code OAuth, Codex, Gemini, Google) keep their dedicated User-Agent.",
			Type:        FlagTypeString,
			Category:    FlagCategoryRequest,
			Placeholder: "e.g. MyApp/1.0",
		},
		{
			Key:         "openai_endpoint_override",
			Label:       "OpenAI endpoint override",
			Description: "Force OpenAI Chat Completions or Responses for this rule, overriding the provider's declared OpenAIEndpointMode default. OpenAI providers only; Anthropic/Google providers ignore this. If the provider declares mode=responses (e.g. Codex), \"chat\" is ignored; if mode=chat, \"responses\" is ignored.",
			Type:        FlagTypeEnum,
			Category:    FlagCategoryRequest,
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
			Category:    FlagCategoryRequest,
		},
		{
			Key:         "use_max_tokens",
			Label:       "OpenAI: Use max_tokens (legacy)",
			Description: "OpenAI only. Rewrite `max_completion_tokens` → `max_tokens` in the outgoing request. Use for older OpenAI-compatible providers that do not yet accept the newer field name.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryRequest,
		},
		{
			Key:         "block_tools",
			Label:       "Block tools",
			Description: "Comma-separated list of tool names to remove from the request before it is forwarded upstream. Matches the tool name as the client sent it; works across OpenAI Chat, OpenAI Responses, Anthropic, and Google requests.",
			Type:        FlagTypeString,
			Category:    FlagCategoryRequest,
			Placeholder: "e.g. web_search,run_terminal_cmd",
		},
		// ── Response ───────────────────────────────────────────────────────
		{
			Key:         "skip_usage",
			Label:       "Skip usage in response",
			Description: "Strip the `usage` block from responses (both SSE deltas and the final body).",
			Type:        FlagTypeBool,
			Category:    FlagCategoryResponse,
		},
		// ── Reasoning ──────────────────────────────────────────────────────
		{
			Key:         "thinking_effort",
			Label:       "Thinking",
			Description: "Single control for extended thinking. \"By Client\" passes the client's thinking config through unchanged. \"Off\" forces thinking disabled. The level values force thinking on with the matching budget — mapped to budget_tokens for Anthropic targets (low 1K / medium 5K / high 20K / max 32K) and to reasoning_effort for OpenAI targets (\"max\" collapses to \"high\").",
			Type:        FlagTypeEnum,
			Category:    FlagCategoryReasoning,
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
			Description: "TTL in seconds for session-to-service pinning. Once a session lands on a service, follow-up requests in the same session keep hitting that service until the entry expires. 0 disables affinity. Works with any load-balancing tactic; does not require smart routing. Session identity is resolved from Anthropic metadata.user_id, the X-Tingly-Session-ID header, or the client IP.",
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
	}
}

package typ

// FlagValueType describes how a rule flag is represented in storage and UI.
type FlagValueType string

const (
	FlagTypeBool   FlagValueType = "bool"
	FlagTypeString FlagValueType = "string"
	FlagTypeEnum   FlagValueType = "enum"
)

// FlagCategory groups flags for presentation in the UI.
type FlagCategory string

const (
	// FlagCategoryIDE — flags that target a specific IDE/client (e.g. Cursor).
	FlagCategoryIDE FlagCategory = "ide"
	// FlagCategoryOpenAI — OpenAI-specific request-shape adjustments
	// (max_tokens vs max_completion_tokens, chat vs responses endpoint, ...).
	FlagCategoryOpenAI FlagCategory = "openai"
	// FlagCategoryHTTP — transport-level overrides (custom User-Agent, etc).
	FlagCategoryHTTP FlagCategory = "http"
	// FlagCategoryResponse — flags that modify the response body/stream.
	FlagCategoryResponse FlagCategory = "response"
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
// the recommended display order in the UI.
func RuleFlagRegistry() []FlagSpec {
	return []FlagSpec{
		{
			Key:         "cursor_compat",
			Label:       "Cursor compatibility",
			Description: "Normalize rich content, gate tools, and strip stream usage for Cursor IDE clients.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryIDE,
		},
		{
			Key:         "cursor_compat_auto",
			Label:       "Auto-detect Cursor",
			Description: "Apply cursor compatibility automatically when request headers identify Cursor.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryIDE,
		},
		{
			Key:         "skip_usage",
			Label:       "Skip usage in response",
			Description: "Strip the `usage` block from responses (both SSE deltas and the final body).",
			Type:        FlagTypeBool,
			Category:    FlagCategoryResponse,
		},
		{
			Key:         "use_max_completion_tokens",
			Label:       "Use max_completion_tokens",
			Description: "Rewrite request field `max_tokens` to `max_completion_tokens` (required by o1/o3/gpt-5 family).",
			Type:        FlagTypeBool,
			Category:    FlagCategoryOpenAI,
		},
		{
			Key:         "use_max_tokens",
			Label:       "Use max_tokens (legacy)",
			Description: "Rewrite request field `max_completion_tokens` back to the legacy `max_tokens`. Use for providers that reject the newer field name.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryOpenAI,
		},
		{
			Key:         "custom_user_agent",
			Label:       "Custom User-Agent",
			Description: "Override the outbound User-Agent header sent to the upstream provider. Takes precedence over the provider-level User-Agent for generic OpenAI / Anthropic clients; vendor-specific clients (Claude Code OAuth, Codex, Gemini, Google) keep their dedicated User-Agent.",
			Type:        FlagTypeString,
			Category:    FlagCategoryHTTP,
			Placeholder: "e.g. MyApp/1.0",
		},
		{
			Key:         "openai_endpoint_override",
			Label:       "OpenAI endpoint override",
			Description: "Force OpenAI Chat Completions or Responses regardless of the adaptive router's probe-based decision. OpenAI providers only; Anthropic/Google providers ignore this. On Codex OAuth providers, \"chat\" is ignored (Codex has no Chat endpoint).",
			Type:        FlagTypeEnum,
			Category:    FlagCategoryOpenAI,
			Options: []FlagOption{
				{Value: "auto", Label: "Auto (adaptive)"},
				{Value: "chat", Label: "Force Chat Completions"},
				{Value: "responses", Label: "Force Responses API"},
			},
		},
	}
}

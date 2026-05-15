package typ

// FlagValueType describes how a rule flag is represented in storage and UI.
type FlagValueType string

const (
	FlagTypeBool   FlagValueType = "bool"
	FlagTypeString FlagValueType = "string"
)

// FlagCategory groups flags for presentation in the UI.
type FlagCategory string

const (
	FlagCategoryCompatibility FlagCategory = "compatibility"
	FlagCategoryRequest       FlagCategory = "request"
	FlagCategoryResponse      FlagCategory = "response"
)

// FlagSpec describes a single rule-level flag's metadata for the UI catalog.
// Keys must match the JSON tag name on RuleFlags.
type FlagSpec struct {
	Key         string       `json:"key"`
	Label       string       `json:"label"`
	Description string       `json:"description"`
	Type        FlagValueType `json:"type"`
	Category    FlagCategory `json:"category"`
	// Placeholder is the hint text shown in string-type input fields.
	Placeholder string `json:"placeholder,omitempty"`
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
			Category:    FlagCategoryCompatibility,
		},
		{
			Key:         "cursor_compat_auto",
			Label:       "Auto-detect Cursor",
			Description: "Apply cursor compatibility automatically when request headers identify Cursor.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryCompatibility,
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
			Category:    FlagCategoryRequest,
		},
		{
			Key:         "use_max_tokens",
			Label:       "Use max_tokens (legacy)",
			Description: "Rewrite request field `max_completion_tokens` back to the legacy `max_tokens`. Use for providers that reject the newer field name.",
			Type:        FlagTypeBool,
			Category:    FlagCategoryRequest,
		},
		{
			Key:         "custom_user_agent",
			Label:       "Custom User-Agent",
			Description: "Override the outbound User-Agent header sent to the upstream provider. Takes precedence over the provider-level User-Agent for generic OpenAI / Anthropic clients; vendor-specific clients (Claude Code OAuth, Codex, Gemini, Google) keep their dedicated User-Agent.",
			Type:        FlagTypeString,
			Category:    FlagCategoryRequest,
			Placeholder: "e.g. MyApp/1.0",
		},
	}
}

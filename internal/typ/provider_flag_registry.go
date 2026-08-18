package typ

// providerFlagDescriptions lists, in display order, every flag configurable at
// the provider and model levels, together with its supply-side wording.
//
// The keys are a subset of RuleFlagRegistry's — the two levels share one
// vocabulary on purpose, so a knob means the same thing wherever it is set and
// only the level changes. Everything except the description (label, value
// type, category, placeholder, options, suggestions) is taken from the rule
// spec for the same key, so the UI renders an identical control on both
// surfaces and the two can never drift apart.
//
// Absent on purpose:
//   - session_affinity, vision_proxy_service — resolved before an upstream is
//     picked, so a value attached to an upstream could never be read.
//   - openai_endpoint_override — the provider already carries a first-class
//     OpenAIEndpointMode for exactly this; a second control for one setting is
//     the duplicate-mode-picker trap (.design/ux-principles.md).
//   - claude_org_id — Claude OAuth only, while provider flags are api_key only.
var providerFlagDescriptions = []struct {
	Key         string
	Description string
}{
	{
		Key:         "extra_headers",
		Description: "Append custom HTTP headers to outbound requests to this provider. Typical uses are upstreams that gate on their own headers (OpenRouter's HTTP-Referer / X-Title, gateway tenant or audit headers). Headers are sent as configured, including ones the gateway also sets (Authorization, User-Agent, …) — overriding those is your call and your responsibility.",
	},
	{
		Key:         "custom_user_agent",
		Description: "Override the User-Agent sent to this provider, for upstreams that gate on a known client. Pick a preset, enter any value, or choose \"None\" to send no User-Agent at all.",
	},
	{
		Key:         "use_max_completion_tokens",
		Description: "Rewrite `max_tokens` → `max_completion_tokens` for this provider. Set it at the model level for the o1/o3/gpt-5 family, which rejects the older field name, and provider-wide for an upstream that only accepts the newer one.",
	},
	{
		Key:         "use_max_tokens",
		Description: "Rewrite `max_completion_tokens` → `max_tokens` for this provider. Use for older OpenAI-compatible upstreams that do not yet accept the newer field name.",
	},
	{
		Key:         "block_tools",
		Description: "Comma-separated tool names to strip from every request to this provider, for upstreams that reject or mishandle a specific tool.",
	},
	{
		Key:         "thinking_effort",
		Description: "Force extended thinking on or off for this provider, for upstreams whose thinking support differs from what clients assume. \"By Client\" leaves the decision to the request.",
	},
	{
		Key:         "skip_usage",
		Description: "Strip the `usage` block from this provider's responses, for upstreams whose usage accounting is absent or wrong.",
	},
	{
		Key:         "claude_code_compat",
		Description: "Fold Claude Code's mid-conversation \"system\" role messages into neighbouring user turns before forwarding. Third-party Anthropic-compatible providers reject that role, which makes this a property of the upstream rather than of the client.",
	},
	{
		Key:         "clean_header",
		Description: "Strip Claude Code's billing-header blocks and geolocation markers from system messages before they reach this provider. Those headers are meant for Anthropic's billing backend and must not leak to a third-party upstream.",
	},
	{
		Key:         "cursor_compat",
		Description: "Apply the Cursor normalizations (rich content, tool gating, stream usage) to every request to this provider.",
	},
	{
		Key:         "cursor_compat_auto",
		Description: "Apply the Cursor normalizations to requests to this provider only when the inbound headers identify Cursor.",
	},
	{
		Key:         "context_1m",
		Description: "Request Anthropic's 1M token context window by injecting the context-1m-2025-08-07 beta flag. Support is a property of the model (Sonnet 4.6+, Opus 4.6+), so this is normally set at the model level.",
	},
}

// ProviderFlagRegistry returns the catalog of supported provider/model-level
// flags — the supply-side counterpart of RuleFlagRegistry, with the same
// contract: keys match the JSON tag names on ProviderFlags, and the UI renders
// each spec from its Type.
//
// Every spec is configurable at both the provider and the model level, and the
// levels combine uniformly by value kind (bools OR, scalars take the narrowest
// non-zero value, maps merge per key), so there is no per-spec scope or merge
// axis to declare. The scenario-inheritance fields (Shared, InheritanceMode)
// belong to the rule axis and are cleared here. See .design/provider-flags.md.
func ProviderFlagRegistry() []FlagSpec {
	ruleSpecs := make(map[string]FlagSpec, len(RuleFlagRegistry()))
	for _, spec := range RuleFlagRegistry() {
		ruleSpecs[spec.Key] = spec
	}

	specs := make([]FlagSpec, 0, len(providerFlagDescriptions))
	for _, entry := range providerFlagDescriptions {
		spec, ok := ruleSpecs[entry.Key]
		if !ok {
			// Guarded by TestProviderFlagRegistry_KeysExistInRuleRegistry.
			continue
		}
		spec.Description = entry.Description
		spec.Shared = false
		spec.InheritanceMode = ""
		specs = append(specs, spec)
	}
	return specs
}

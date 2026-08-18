package typ

import "net/textproto"

// SupplyExtraHeaders merges the two supply-side header levels for one
// (provider, model) pair — Provider.Flags ∪ Provider.ModelFlags[model], the
// model level winning on name conflicts. These are a property of the
// upstream, not of the request, so the client layer resolves them once per
// client (which is already keyed by provider + model) instead of per request.
//
// The third level, the rule's extra_headers, is a request-side flag: it
// rides the request context and the outbound transport applies it *after*
// this map, which is what makes the full precedence
//
//	provider  <  model  <  rule
//
// hold without anyone merging all three (see .design/provider-flags.md §5.1).
//
// Returns nil when neither supply level configures anything, so callers can
// cheaply skip injection.
func SupplyExtraHeaders(p *Provider, model string) map[string]string {
	if p == nil {
		return nil
	}
	provider := p.Flags.ExtraHeaders
	modelLevel := p.ModelFlags[model].ExtraHeaders
	if len(provider) == 0 && len(modelLevel) == 0 {
		return nil
	}
	merged := make(map[string]string, len(provider)+len(modelLevel))
	for _, level := range []map[string]string{provider, modelLevel} {
		for name, value := range level {
			merged[textproto.CanonicalMIMEHeaderKey(name)] = value
		}
	}
	return merged
}

// ApplyProviderFlags folds the two supply-side flag levels of a (provider,
// model) pair into an already-resolved rule-side flag set, yielding the
// effective flags for one request. Precedence is
//
//	provider  <  model  <  rule
//
// applied uniformly by value kind, so no per-flag merge metadata exists:
// bools OR together (any level enabling it activates the flag), and scalars
// take the narrowest non-zero value — the rule's if it set one, else the
// model's, else the provider's.
//
// ExtraHeaders is deliberately not merged here. Supply-side headers are
// resolved per client by SupplyExtraHeaders and written by the outbound
// transport *before* the rule's, which already produces the same precedence
// at a lower layer; merging them here as well would double-apply them and
// would make provider configuration show up as a rule flag in the applied-flag
// diagnostics. See .design/provider-flags.md §5.1.
func ApplyProviderFlags(flags RuleFlags, p *Provider, model string) RuleFlags {
	if p == nil {
		return flags
	}
	// Narrowest level first: bools OR regardless of order, and every scalar
	// only fills a slot still left at its zero value, so the first level to
	// offer a value keeps it — model over provider, and the rule over both
	// since its value is already in flags.
	for _, supply := range []ProviderFlags{p.ModelFlags[model], p.Flags} {
		flags.CursorCompat = flags.CursorCompat || supply.CursorCompat
		flags.CursorCompatAuto = flags.CursorCompatAuto || supply.CursorCompatAuto
		flags.SkipUsage = flags.SkipUsage || supply.SkipUsage
		flags.UseMaxCompletionTokens = flags.UseMaxCompletionTokens || supply.UseMaxCompletionTokens
		flags.UseMaxTokens = flags.UseMaxTokens || supply.UseMaxTokens
		flags.CleanHeader = flags.CleanHeader || supply.CleanHeader
		flags.ClaudeCodeCompat = flags.ClaudeCodeCompat || supply.ClaudeCodeCompat
		flags.Context1M = flags.Context1M || supply.Context1M

		if flags.CustomUserAgent == "" {
			flags.CustomUserAgent = supply.CustomUserAgent
		}
		if flags.BlockTools == "" {
			flags.BlockTools = supply.BlockTools
		}
		if flags.ThinkingEffort == ThinkingEffortDefault {
			flags.ThinkingEffort = supply.ThinkingEffort
		}
	}
	return flags
}

// PruneModelFlags drops entries carrying no configuration (and the empty
// model key) so a cleared model does not linger as an empty object in
// storage and in API responses. Returns nil when nothing is left.
func PruneModelFlags(modelFlags map[string]ProviderFlags) map[string]ProviderFlags {
	pruned := make(map[string]ProviderFlags, len(modelFlags))
	for model, flags := range modelFlags {
		if model == "" || flags.IsZero() {
			continue
		}
		pruned[model] = flags
	}
	if len(pruned) == 0 {
		return nil
	}
	return pruned
}

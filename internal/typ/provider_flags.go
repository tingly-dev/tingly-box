package typ

import "net/textproto"

// SupplyExtraHeaders merges the two supply-side header levels for one
// (provider, model) pair, the model level winning on name conflicts. Resolved
// once per client (already keyed by provider + model), not per request.
//
// The rule's extra_headers are not merged in: the transport writes them after
// this map, so provider < model < rule falls out of the write order rather
// than a three-level merge (.design/provider-flags.md §5.1). Returns nil when
// neither level configures anything.
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

// ApplyProviderFlags folds a (provider, model) pair's supply-side flags into
// an already-resolved rule-side set, giving the effective flags for one
// request: bools OR across levels, scalars take the narrowest non-zero value
// (provider < model < rule).
//
// ExtraHeaders is excluded — it rides SupplyExtraHeaders and the transport's
// write order instead, so merging it here would double-apply it and report
// provider config as a rule flag in the applied-flag diagnostics.
func ApplyProviderFlags(flags RuleFlags, p *Provider, model string) RuleFlags {
	if p == nil {
		return flags
	}
	// Narrowest first: each scalar only fills a slot still at its zero value,
	// so the first level offering one keeps it — model over provider, and the
	// rule over both since its value is already in flags.
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

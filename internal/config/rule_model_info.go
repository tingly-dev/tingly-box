package config

import (
	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RuleModelInfo returns the provider catalog's details for the model(s) an
// active rule routes to: the description of the first service the catalog
// knows, and per numeric field the smallest known value across services, so
// the result is safe for whichever service serves the request. The template
// is matched from the provider itself (never its user-chosen name) and looked
// up by the service's upstream model, then by the rule's request model for
// rules that keep the upstream name. ok is false when no service is known.
//
// It is the one place a rule's model metadata is resolved: /v1/models, the
// management model listing and the Codex catalog all read it.
func (c *Config) RuleModelInfo(rule typ.Rule) (info catalog.ModelInfo, ok bool) {
	tm := c.GetTemplateManager()
	if tm == nil {
		return catalog.ModelInfo{}, false
	}
	for _, svc := range rule.GetServices() {
		if svc == nil || !svc.Active {
			continue
		}
		provider, err := c.GetProviderByUUID(svc.Provider)
		if err != nil || provider == nil {
			continue
		}
		m, found := tm.GetModelInfoForProvider(provider, svc.Model)
		if !found {
			m, found = tm.GetModelInfoForProvider(provider, rule.RequestModel)
		}
		if !found {
			continue
		}
		if !ok {
			info, ok = m, true
			continue
		}
		info.Context = smallestKnown(info.Context, m.Context)
		info.MaxInput = smallestKnown(info.InputLimit(), m.InputLimit())
		info.MaxOutput = smallestKnown(info.MaxOutput, m.MaxOutput)
	}
	return info, ok
}

// RuleCatalogInputLimit is RuleModelInfo's input limit (Anthropic's
// max_input_tokens), 0 when unknown. Codex's `context_window` means this
// input budget, not the vendor's published window: Codex's own catalog gives
// gpt-5 272K, not OpenAI's 400K.
func (c *Config) RuleCatalogInputLimit(rule typ.Rule) int {
	info, _ := c.RuleModelInfo(rule)
	return info.InputLimit()
}

// smallestKnown is the smaller of two values, treating 0 as unknown.
func smallestKnown(a, b int) int {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	default:
		return min(a, b)
	}
}

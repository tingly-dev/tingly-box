package config

import (
	"strings"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RuleModelInfo returns the provider catalog's details for the model(s) an
// active rule routes to: the description of the first upstream the catalog
// knows, and per numeric field the smallest known value across upstreams, so
// the result is safe for whichever one serves the request (see ruleUpstreams
// for which count). The template is matched from the provider itself (never
// its user-chosen name) and looked up by the service's upstream model, then by
// the rule's request model for rules that keep the upstream name. ok is false
// when no upstream is known. MaxContext is kept only when one upstream is
// known: templates count it differently (with or without output), so the
// Codex window merges it per upstream instead (CodexAutoContextWindow).
//
// The rule's 1M flag counts where it opens more: an upstream that a request
// from a client of style client reaches with the context-1m header
// (sendsContext1M) has its larger window in use (ModelInfo.WithMaxContext).
//
// It is the one place a rule's model metadata is resolved: /v1/models and the
// management model listing read it, and the Codex window reads the same
// upstreams (ruleUpstreams, upstreamModelInfo).
func (c *Config) RuleModelInfo(rule typ.Rule, client ai.APIStyle) (info catalog.ModelInfo, ok bool) {
	tm := c.GetTemplateManager()
	if tm == nil {
		return info, false
	}
	for _, up := range c.ruleUpstreams(rule) {
		m, found := upstreamModelInfo(tm, up, rule.RequestModel)
		if !found {
			continue
		}
		if sendsContext1M(up.provider, rule, client) {
			m = m.WithMaxContext()
		}
		if !ok {
			info, ok = m, true
			continue
		}
		info.Context = smallestKnown(info.Context, m.Context)
		info.MaxInput = smallestKnown(info.InputLimit(), m.InputLimit())
		info.MaxOutput = smallestKnown(info.MaxOutput, m.MaxOutput)
		info.MaxContext = 0
	}
	return info, ok
}

// ruleUpstream is one upstream a rule can send a request to: a provider and
// the service's upstream model ("" when the service keeps the request model).
type ruleUpstream struct {
	provider *typ.Provider
	model    string
}

// ruleUpstreams lists the upstreams a rule can reach: its active services and,
// with smart routing on, each route's. Services on a disabled or deleted
// provider never answer and are left out.
func (c *Config) ruleUpstreams(rule typ.Rule) []ruleUpstream {
	services := rule.GetServices()
	if rule.SmartEnabled {
		for _, route := range rule.SmartRouting {
			services = append(services[:len(services):len(services)], route.Services...)
		}
	}
	var out []ruleUpstream
	for _, svc := range services {
		if svc == nil || !svc.Active {
			continue
		}
		provider, err := c.GetProviderByUUID(svc.Provider)
		if err != nil || provider == nil || !provider.Enabled {
			continue
		}
		out = append(out, ruleUpstream{provider: provider, model: svc.Model})
	}
	return out
}

// upstreamModelInfo looks an upstream's model up on its provider's matched
// template: the service's model, then the request model for rules that keep
// the upstream name. A nil catalog knows nothing.
func upstreamModelInfo(tm *catalog.ProviderCatalogManager, up ruleUpstream, requestModel string) (catalog.ModelInfo, bool) {
	if tm == nil {
		return catalog.ModelInfo{}, false
	}
	if info, ok := tm.GetModelInfoForProvider(up.provider, up.model); ok {
		return info, true
	}
	return tm.GetModelInfoForProvider(up.provider, strings.TrimSpace(requestModel))
}

// sendsContext1M reports whether a request from a client of style client
// reaches provider through rule with the context-1m beta header: the rule's
// 1M flag adds it, and only a request sent to the provider's Anthropic
// endpoint carries it. A provider with a URL in the client's own style serves
// it there unless the rule forces the Anthropic endpoint, the choice the
// gateway makes (Provider.RequestStyle).
func sendsContext1M(provider *typ.Provider, rule typ.Rule, client ai.APIStyle) bool {
	if !rule.Flags.Context1M {
		return false
	}
	_, style := provider.ResolveEndpoint(provider.RequestStyle(client, rule.Flags.ForcesAnthropicEndpoint()))
	return style == ai.APIStyleAnthropic
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

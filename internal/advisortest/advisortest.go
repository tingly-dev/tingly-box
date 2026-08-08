// Package advisortest provides shared helpers for building advisor MCP
// source configurations in tests. It replaces per-package copies of the
// same helpers in internal/server and internal/protocolserver.
package advisortest

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Provider returns a ProviderResolver that always resolves to a synthetic
// provider backed by the given URL, key, and API style.
func Provider(url, key string, style protocol.APIStyle) func(string) (*typ.Provider, error) {
	return func(string) (*typ.Provider, error) {
		return &typ.Provider{
			Name:     "test-advisor",
			APIBase:  url,
			Token:    key,
			APIStyle: style,
			Enabled:  true,
		}, nil
	}
}

// Config builds an AdvisorConfig wired to a synthetic provider.
func Config(url, key, model string, style protocol.APIStyle, maxUses int) *typ.AdvisorConfig {
	return &typ.AdvisorConfig{
		ProviderUUID:      "test",
		ProviderResolver:  Provider(url, key, style),
		Model:             model,
		MaxUsesPerRequest: maxUses,
	}
}

// SourceWithEnabled builds an advisor MCP source config with explicit enablement.
func SourceWithEnabled(url, key, model string, style protocol.APIStyle, maxUses int, enabled bool) typ.MCPSourceConfig {
	return typ.MCPSourceConfig{
		ID:         "advisor",
		Transport:  "advisor",
		Enabled:    typ.BoolPtr(enabled),
		Visibility: typ.ToolVisibilityServer,
		Tools:      []string{"advisor"},
		Advisor:    Config(url, key, model, style, maxUses),
	}
}

// Source builds an enabled advisor MCP source config.
func Source(url, key, model string, style protocol.APIStyle, maxUses int) typ.MCPSourceConfig {
	return SourceWithEnabled(url, key, model, style, maxUses, true)
}

// ResolvedProvider resolves the synthetic provider from an advisor source config.
func ResolvedProvider(source typ.MCPSourceConfig) (*typ.Provider, error) {
	if source.Advisor == nil || source.Advisor.ProviderResolver == nil || source.Advisor.ProviderUUID == "" {
		return nil, fmt.Errorf("advisor test source is missing ProviderResolver or ProviderUUID")
	}
	return source.Advisor.ProviderResolver(source.Advisor.ProviderUUID)
}

package server

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// testAdvisorProvider returns a ProviderResolver that always resolves to a
// synthetic provider backed by the given URL, key, and API style.
func testAdvisorProvider(url, key string, style protocol.APIStyle) func(string) (*typ.Provider, error) {
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

func testAdvisorConfig(url, key, model string, style protocol.APIStyle, maxUses int) *typ.AdvisorConfig {
	return &typ.AdvisorConfig{
		ProviderUUID:      "test",
		ProviderResolver:  testAdvisorProvider(url, key, style),
		Model:             model,
		MaxUsesPerRequest: maxUses,
	}
}

func testAdvisorSourceWithEnabled(url, key, model string, style protocol.APIStyle, maxUses int, enabled bool) typ.MCPSourceConfig {
	return typ.MCPSourceConfig{
		ID:         "advisor",
		Transport:  "advisor",
		Enabled:    typ.BoolPtr(enabled),
		Visibility: typ.ToolVisibilityServer,
		Tools:      []string{"advisor"},
		Advisor:    testAdvisorConfig(url, key, model, style, maxUses),
	}
}

func testAdvisorSource(url, key, model string, style protocol.APIStyle, maxUses int) typ.MCPSourceConfig {
	return testAdvisorSourceWithEnabled(url, key, model, style, maxUses, true)
}

func testAdvisorResolvedProvider(source typ.MCPSourceConfig) (*typ.Provider, error) {
	if source.Advisor == nil || source.Advisor.ProviderResolver == nil || source.Advisor.ProviderUUID == "" {
		return nil, fmt.Errorf("advisor test source is missing ProviderResolver or ProviderUUID")
	}
	return source.Advisor.ProviderResolver(source.Advisor.ProviderUUID)
}

package runtime

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func testAdvisorProviderResolver(style protocol.APIStyle) func(string) (*typ.Provider, error) {
	return testAdvisorProviderResolverWithBase("https://example.com", "test-key", style)
}

func testAdvisorProviderResolverWithBase(baseURL, token string, style protocol.APIStyle) func(string) (*typ.Provider, error) {
	return func(string) (*typ.Provider, error) {
		return &typ.Provider{
			Name:     "test",
			APIBase:  baseURL,
			Token:    token,
			APIStyle: style,
			Enabled:  true,
		}, nil
	}
}

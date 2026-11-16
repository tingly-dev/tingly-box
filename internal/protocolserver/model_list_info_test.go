package protocolserver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Model details on /v1/models come from the catalog entry of the upstream
// model a rule routes to — even when the provider has a custom display name
// and the rule exposes an alias instead of the upstream model id.
func TestRuleModelInfo_MatchesByProviderAndUpstreamModel(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	tm := catalog.NewProviderCatalogManager(catalog.EmbeddedOnly())
	require.NoError(t, tm.Initialize(context.Background()))
	cfg.SetTemplateManager(tm)

	p := &typ.Provider{
		Name:     "my deepseek", // not a template id
		APIBase:  "https://api.deepseek.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "sk-test",
		Enabled:  true,
	}
	require.NoError(t, cfg.AddProvider(p))

	rule := typ.Rule{
		RequestModel: "fast-thinker", // alias, not the upstream id
		Active:       true,
		Services: []*loadbalance.Service{
			{Provider: p.UUID, Model: "deepseek-reasoner", Active: true},
		},
	}
	info, ok := ruleModelInfo(cfg, rule)
	require.True(t, ok)
	assert.Equal(t, 128000, info.Context)
	assert.Equal(t, 64000, info.MaxOutput)

	// A rule that keeps the upstream name still resolves via the request model.
	rule.RequestModel = "deepseek-chat"
	rule.Services[0].Model = ""
	info, ok = ruleModelInfo(cfg, rule)
	require.True(t, ok)
	assert.Equal(t, 8000, info.MaxOutput)
}

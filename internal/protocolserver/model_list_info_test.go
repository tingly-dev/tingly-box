package protocolserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
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
	info, ok := ruleModelInfo(cfg, rule, ai.APIStyleOpenAI)
	require.True(t, ok)
	assert.Equal(t, 128000, info.Context)
	assert.Equal(t, 64000, info.MaxOutput)

	// A rule that keeps the upstream name still resolves via the request model.
	rule.RequestModel = "deepseek-chat"
	rule.Services[0].Model = ""
	info, ok = ruleModelInfo(cfg, rule, ai.APIStyleOpenAI)
	require.True(t, ok)
	assert.Equal(t, 8000, info.MaxOutput)
}

// With several services, numeric fields are the smallest known across them,
// and /v1/models carries them under both our names and the OpenAI-compatible
// ones.
func TestRuleModelInfo_SmallestAcrossServices(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	tm := catalog.NewProviderCatalogManager(catalog.EmbeddedOnly())
	require.NoError(t, tm.Initialize(context.Background()))
	cfg.SetTemplateManager(tm)

	p := &typ.Provider{
		Name:     "ds",
		APIBase:  "https://api.deepseek.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "sk-test",
		Enabled:  true,
	}
	require.NoError(t, cfg.AddProvider(p))
	reasoner, ok := tm.GetModelInfoForProvider(p, "deepseek-reasoner")
	require.True(t, ok)
	chat, ok := tm.GetModelInfoForProvider(p, "deepseek-chat")
	require.True(t, ok)

	rule := typ.Rule{
		RequestModel: "mixed",
		Active:       true,
		Services: []*loadbalance.Service{
			{Provider: p.UUID, Model: "deepseek-reasoner", Active: true},
			{Provider: p.UUID, Model: "deepseek-chat", Active: true},
			{Provider: p.UUID, Model: "not-in-catalog", Active: true},
		},
	}
	info, ok := ruleModelInfo(cfg, rule, ai.APIStyleOpenAI)
	require.True(t, ok)
	assert.Equal(t, min(reasoner.Context, chat.Context), info.Context)
	assert.Equal(t, min(reasoner.MaxOutput, chat.MaxOutput), info.MaxOutput)
	assert.Equal(t, reasoner.Description, info.Description, "description comes from the first known service")
}

// The Codex list carries, per model, what "Auto" writes into Codex's catalog
// with the 1M flag off and on, including the guess for a model nothing is
// known about, so the rule editor shows what is written instead of working it
// out. Rules sharing a request model share one catalog entry and carry the
// same. The catalog figures keep their meaning, and other scenarios carry no
// Codex windows.
func TestCodexModelList_CarriesAutoWindows(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	tm := catalog.NewProviderCatalogManager(catalog.EmbeddedOnly())
	require.NoError(t, tm.Initialize(context.Background()))
	cfg.SetTemplateManager(tm)

	chatgpt := &typ.Provider{Name: "ChatGPT", APIBase: "https://chatgpt.com/backend-api", APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeOAuth, OAuthDetail: &typ.OAuthDetail{Issuer: "codex", AccessToken: "t"}, Enabled: true}
	house := &typ.Provider{Name: "House", APIBase: "https://llm.example.internal/v1", APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey, Token: "k", Enabled: true}
	require.NoError(t, cfg.AddProvider(chatgpt))
	require.NoError(t, cfg.AddProvider(house))
	svc := func(p *typ.Provider, model string) []*loadbalance.Service {
		return []*loadbalance.Service{{Provider: p.UUID, Model: model, Active: true}}
	}
	cfg.Rules = []typ.Rule{
		{UUID: "a", Scenario: typ.ScenarioCodex, RequestModel: "sol", Active: true, Services: svc(chatgpt, "gpt-6-sol")},
		{UUID: "b", Scenario: typ.ScenarioCodex, RequestModel: "house", Active: true, Services: svc(house, "house-model"),
			Flags: typ.RuleFlags{Context1M: true}},
		{UUID: "c", Scenario: typ.ScenarioCodex, RequestModel: "pair", Active: true, Services: svc(chatgpt, "gpt-6-sol")},
		{UUID: "d", Scenario: typ.ScenarioCodex, RequestModel: "pair", Active: true, Services: svc(house, "house-model")},
		{UUID: "e", Scenario: typ.ScenarioOpenCode, RequestModel: "oc-sol", Active: true, Services: svc(chatgpt, "gpt-6-sol")},
		{UUID: "f", Scenario: typ.ScenarioCodex, RequestModel: "pinned", Active: true, Services: svc(chatgpt, "gpt-6-sol"),
			Flags: typ.RuleFlags{CodexContextWindow: 128000}},
	}
	ph := &ProtocolHandler{deps: ProtocolHandlerDeps{Config: cfg}}
	list := func(scenario typ.RuleScenario) []OpenAIModel {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		ph.OpenAIListModelsForScenario(c, scenario)
		require.Equal(t, http.StatusOK, w.Code)
		var resp OpenAIModelsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.Data
	}
	win := func(window, max int, source string) config.CodexContextWindow {
		return config.CodexContextWindow{Window: window, Max: max, Source: source}
	}
	want := map[string]config.CodexAutoWindows{
		"sol":   {Off: win(272000, 872000, config.CodexWindowSourceCatalog), On: win(872000, 872000, config.CodexWindowSourceExtended)},
		"house": {Off: win(200000, 200000, config.CodexWindowSourceDefault), On: win(1000000, 1000000, config.CodexWindowSourceDefault)},
		// The smaller of the two upstreams' windows, even a guess.
		"pair": {Off: win(200000, 200000, config.CodexWindowSourceDefault), On: win(872000, 872000, config.CodexWindowSourceExtended)},
		// Auto, whatever the rule pins: the editor shows the pin itself.
		"pinned": {Off: win(272000, 872000, config.CodexWindowSourceCatalog), On: win(872000, 872000, config.CodexWindowSourceExtended)},
	}

	codex := list(typ.ScenarioCodex)
	require.Len(t, codex, 5)
	for _, m := range codex {
		require.NotNil(t, m.CodexWindows, m.ID)
		assert.Equal(t, want[m.ID], *m.CodexWindows, m.ID)
		assert.Equal(t, "tingly-box", m.OwnedBy)
		if m.ID == "sol" {
			assert.Equal(t, [2]int{272000, 272000}, [2]int{m.ContextWindow, m.MaxInputTokens}, "the catalog figures stay")
		}
	}

	other := list(typ.ScenarioOpenCode)
	require.Len(t, other, 1)
	assert.Nil(t, other[0].CodexWindows)
	assert.Equal(t, 272000, other[0].MaxInputTokens)

	// The JSON the rule editor reads.
	raw, err := json.Marshal(want["sol"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"off":{"context_window":272000,"max_context_window":872000,"source":"catalog"},
		"on":{"context_window":872000,"max_context_window":872000,"source":"extended"}}`, string(raw))
}

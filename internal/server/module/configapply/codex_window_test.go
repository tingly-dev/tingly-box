package configapply

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// codexWindowConfig: Codex rules on DeepSeek's deepseek-chat (128K in the
// embedded provider catalog), on a ChatGPT-backed model (272K, up to 872K),
// on a provider the catalog doesn't know, one with its own window, and the
// ChatGPT-backed model again with the 1M flag.
func codexWindowConfig(t *testing.T, withTemplateManager bool) (*config.Config, *typ.Provider) {
	t.Helper()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	if withTemplateManager {
		tm := catalog.NewProviderCatalogManager(catalog.EmbeddedOnly())
		require.NoError(t, tm.Initialize(context.Background()))
		cfg.SetTemplateManager(tm)
	}

	ds := &typ.Provider{Name: "DeepSeek", APIBase: "https://api.deepseek.com/v1", APIStyle: protocol.APIStyleOpenAI, AuthType: typ.AuthTypeAPIKey, Token: "k", Enabled: true}
	house := &typ.Provider{Name: "House", APIBase: "https://llm.example.internal/v1", APIStyle: protocol.APIStyleOpenAI, AuthType: typ.AuthTypeAPIKey, Token: "k", Enabled: true}
	chatgpt := &typ.Provider{Name: "ChatGPT", APIBase: "https://chatgpt.com/backend-api", APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeOAuth, OAuthDetail: &typ.OAuthDetail{Issuer: "codex", AccessToken: "t"}, Enabled: true}
	for _, p := range []*typ.Provider{ds, house, chatgpt} {
		require.NoError(t, cfg.AddProvider(p))
	}
	svc := func(p *typ.Provider, model string) []*loadbalance.Service {
		return []*loadbalance.Service{{Provider: p.UUID, Model: model, Active: true}}
	}
	cfg.Rules = []typ.Rule{
		{UUID: "r-ds", Scenario: typ.ScenarioCodex, RequestModel: "ds", Active: true, Services: svc(ds, "deepseek-chat"),
			Flags: typ.RuleFlags{Context1M: true}},
		{UUID: "r-sol", Scenario: typ.ScenarioCodex, RequestModel: "sol", Active: true, Services: svc(chatgpt, "gpt-6-sol"),
			Flags: typ.RuleFlags{CodexCompactPercent: 60}},
		{UUID: "r-house", Scenario: typ.ScenarioCodex, RequestModel: "house", Active: true, Services: svc(house, "house-model")},
		{UUID: "r-pinned", Scenario: typ.ScenarioCodex, RequestModel: "pinned", Active: true, Services: svc(chatgpt, "gpt-6-sol"),
			Flags: typ.RuleFlags{CodexContextWindow: 128000}},
		{UUID: "r-wide", Scenario: typ.ScenarioCodex, RequestModel: "wide", Active: true, Services: svc(chatgpt, "gpt-6-sol"),
			Flags: typ.RuleFlags{Context1M: true}},
	}
	return cfg, ds
}

func serveCodex(t *testing.T, method, path string, h gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, path, h)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return w
}

type codexCatalogEntry struct {
	Window  int `json:"context_window"`
	Max     int `json:"max_context_window"`
	Compact int `json:"auto_compact_token_limit"`
}

func readCodexCatalog(t *testing.T, raw []byte) map[string]codexCatalogEntry {
	t.Helper()
	var file struct {
		Models []struct {
			Slug string `json:"slug"`
			codexCatalogEntry
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(raw, &file))
	out := map[string]codexCatalogEntry{}
	for _, m := range file.Models {
		out[m.Slug] = m.codexCatalogEntry
	}
	return out
}

func readCodexCatalogFile(t *testing.T, home string) map[string]codexCatalogEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".codex", "tingly-model-catalog.json"))
	require.NoError(t, err)
	return readCodexCatalog(t, raw)
}

// Apply writes each model's window and ceiling; the preview shows the same
// catalog; the status check agrees with what was written, and changing a
// rule's upstream or window makes it stale.
func TestApplyCodexConfig_WritesDerivedWindows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg, ds := codexWindowConfig(t, true)
	h := NewHandler(cfg, "localhost")

	serveCodex(t, http.MethodPost, "/config/apply/codex", h.ApplyCodexConfigFromState)
	want := map[string]codexCatalogEntry{
		"ds":     {128000, 128000, 96000},  // the 1M flag doesn't widen DeepSeek
		"sol":    {272000, 872000, 163200}, // its own 60% compact
		"house":  {200000, 200000, 150000}, // unknown: the default
		"pinned": {128000, 128000, 96000},  // own window, also its ceiling
		"wide":   {872000, 872000, 654000}, // the 1M flag opens its larger window
	}
	assert.Equal(t, want, readCodexCatalogFile(t, home))

	var preview CodexConfigPreviewResponse
	require.NoError(t, json.Unmarshal(serveCodex(t, http.MethodPost, "/config/preview/codex", h.GetCodexConfigPreview).Body.Bytes(), &preview))
	assert.Equal(t, want, readCodexCatalog(t, []byte(preview.CatalogJson)))

	status := func() ClientConfigStatusResponse {
		var s ClientConfigStatusResponse
		require.NoError(t, json.Unmarshal(serveCodex(t, http.MethodGet, "/config/codex/status", h.GetCodexConfigStatus).Body.Bytes(), &s))
		return s
	}
	s := status()
	assert.Equal(t, ClientConfigApplied, s.State, "%v", s.Differences)

	// Pointing sol at DeepSeek changes its window and ceiling.
	cfg.Rules[1].Services = []*loadbalance.Service{{Provider: ds.UUID, Model: "deepseek-chat", Active: true}}
	s = status()
	assert.Equal(t, ClientConfigOutdated, s.State)
	assert.ElementsMatch(t, []ClientConfigDifference{
		{Key: "context_window[sol]", Applied: "272000", Expected: "128000"},
		{Key: "max_context_window[sol]", Applied: "872000", Expected: "128000"},
	}, s.Differences)

	// Pinning a window is stale too.
	cfg.Rules[1].Services = cfg.Rules[3].Services
	cfg.Rules[2].Flags.CodexContextWindow = 256000
	s = status()
	assert.Equal(t, ClientConfigOutdated, s.State)
	assert.ElementsMatch(t, []ClientConfigDifference{
		{Key: "context_window[house]", Applied: "200000", Expected: "256000"},
		{Key: "max_context_window[house]", Applied: "200000", Expected: "256000"},
	}, s.Differences)
}

// The CLI / IM apply (internal/agent) writes the same catalog as the UI's Auto
// Config, also from the catalog a CLI process loads (internal/app).
func TestCodexAgentApply_MatchesAutoConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfg, _ := codexWindowConfig(t, true)
	catalogPath := filepath.Join(home, ".codex", "tingly-model-catalog.json")
	serveCodex(t, http.MethodPost, "/config/apply/codex", NewHandler(cfg, "localhost").ApplyCodexConfigFromState)
	require.Len(t, readCodexCatalogFile(t, home), 5)
	fromUI, err := os.ReadFile(catalogPath)
	require.NoError(t, err)

	// A CLI process reads the catalog the server last synced, here none yet.
	tm := catalog.NewProviderCatalogManager(catalog.LastSynced())
	require.NoError(t, tm.Initialize(context.Background()))
	cfg.SetTemplateManager(tm)
	result, err := agent.NewAgentApply(cfg, "localhost").ApplyAgent(&agent.ApplyAgentRequest{AgentType: agent.AgentTypeCodex})
	require.NoError(t, err)
	require.True(t, result.Success, result.Message)
	fromCLI, err := os.ReadFile(catalogPath)
	require.NoError(t, err)
	assert.JSONEq(t, string(fromUI), string(fromCLI))
}

package provider

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
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newTestConfigWithTemplates builds a Config wired to an embedded-only
// TemplateManager (no GitHub/network access), suitable for exercising the
// template-fallback-merge logic in GetProviderModelsByUUID hermetically.
func newTestConfigWithTemplates(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	tm := data.NewEmbeddedOnlyTemplateManager()
	require.NoError(t, tm.Initialize(context.Background()))
	cfg.SetTemplateManager(tm)

	return cfg
}

func newCodexOAuthProvider() *typ.Provider {
	return &typ.Provider{
		Name:     "Codex OAuth",
		APIBase:  protocol.CodexAPIBase,
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeOAuth,
		Enabled:  true,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken: "test-codex-token",
			Issuer:      ai.IssuerCodex,
		},
	}
}

func newClaudeCodeOAuthProvider() *typ.Provider {
	return &typ.Provider{
		Name:     "Claude Code OAuth",
		APIBase:  "https://api.anthropic.com",
		APIStyle: protocol.APIStyleAnthropic,
		AuthType: typ.AuthTypeOAuth,
		Enabled:  true,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken: "test-claude-token",
			Issuer:      ai.IssuerClaudeCode,
		},
	}
}

func getProviderModels(t *testing.T, cfg *config.Config, uuid string) ProviderModelsResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := NewHandler(cfg, nil)
	router := gin.New()
	router.GET("/provider-models/:uuid", handler.GetProviderModelsByUUID)

	req, _ := http.NewRequest("GET", "/provider-models/"+uuid, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp ProviderModelsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// TestGetProviderModelsByUUID_Codex_EmptyCache_UsesTemplate covers the
// "兜底列表" path for a Codex OAuth provider: the OpenAI client short-circuits
// ListModels for the Codex issuer (models endpoint unsupported), so with an
// empty DB cache the handler must fall through to the embedded template list
// without making any outbound network call.
func TestGetProviderModelsByUUID_Codex_EmptyCache_UsesTemplate(t *testing.T) {
	cfg := newTestConfigWithTemplates(t)
	provider := newCodexOAuthProvider()
	require.NoError(t, cfg.AddProvider(provider))

	resp := getProviderModels(t, cfg, provider.UUID)

	require.True(t, resp.Success)
	assert.Equal(t, ModelCacheSourceTemplate, resp.Data.Source)
	assert.NotEmpty(t, resp.Data.Models)
	assert.Contains(t, resp.Data.Models, "gpt-5.5")
}

// TestUpdateProviderModelsByUUID_Codex_ReturnsTemplateModels is a regression
// test for codex model-list refresh returning an empty list. The refresh
// endpoint calls FetchAndSaveProviderModels (which, for codex, short-circuits
// the unsupported /models endpoint and resolves to the embedded template
// without persisting it) and then reads back from the DB cache. Since the
// template is never persisted, a naive DB read yields zero models — the
// endpoint must apply the same template fallback the GET path does.
func TestUpdateProviderModelsByUUID_Codex_ReturnsTemplateModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := newTestConfigWithTemplates(t)
	provider := newCodexOAuthProvider()
	require.NoError(t, cfg.AddProvider(provider))

	handler := NewHandler(cfg, nil)
	router := gin.New()
	router.POST("/provider-models/:uuid", handler.UpdateProviderModelsByUUID)

	req, _ := http.NewRequest("POST", "/provider-models/"+provider.UUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp ProviderModelsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.NotEmpty(t, resp.Data.Models, "codex refresh must fall back to template models, not return an empty list")
	assert.Contains(t, resp.Data.Models, "gpt-5.5")
}

// TestGetProviderModelsByUUID_ClaudeCode_NonEmptyCache_NotPollutedByTemplate
// pins the deprecation-safety guarantee: the embedded template is a
// compile-time snapshot that can still list models the upstream has retired,
// so it must NEVER be merged into a non-empty real list. A cached upstream
// list is served verbatim, even if it is shorter than the template.
func TestGetProviderModelsByUUID_ClaudeCode_NonEmptyCache_NotPollutedByTemplate(t *testing.T) {
	cfg := newTestConfigWithTemplates(t)
	provider := newClaudeCodeOAuthProvider()
	require.NoError(t, cfg.AddProvider(provider))

	// A real upstream list that deliberately omits a model still present in
	// the embedded template (e.g. the model was retired upstream). The handler
	// must trust upstream and not resurrect the template-only model.
	upstream := []string{"claude-sonnet-4-5"}
	require.NoError(t, cfg.GetModelManager().SaveModels(provider, upstream, db.ModelSourceAPI))

	resp := getProviderModels(t, cfg, provider.UUID)

	require.True(t, resp.Success)
	assert.Equal(t, ModelCacheSourceDB, resp.Data.Source)
	assert.ElementsMatch(t, upstream, resp.Data.Models, "non-empty cache must be served verbatim, not merged with the template")
	assert.NotContains(t, resp.Data.Models, "claude-opus-4-5", "template-only model must not be resurrected into an authoritative upstream list")
}

// TestGetProviderModelsByUUID_ClaudeCode_FullCache_NoMergeNeeded verifies that
// when the cached list already contains every template model, the source is
// reported as the plain DB cache rather than "merged" — the merge should only
// be flagged when it actually contributes new models.
func TestGetProviderModelsByUUID_ClaudeCode_FullCache_NoMergeNeeded(t *testing.T) {
	cfg := newTestConfigWithTemplates(t)
	provider := newClaudeCodeOAuthProvider()
	require.NoError(t, cfg.AddProvider(provider))

	templateModels, err := cfg.GetTemplateManager().GetEmbeddedModelsForProvider(provider)
	require.NoError(t, err)
	require.NoError(t, cfg.GetModelManager().SaveModels(provider, templateModels, db.ModelSourceAPI))

	resp := getProviderModels(t, cfg, provider.UUID)

	require.True(t, resp.Success)
	assert.Equal(t, ModelCacheSourceDB, resp.Data.Source)
	assert.ElementsMatch(t, templateModels, resp.Data.Models)
}

package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/catalog"
	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// codexCatalogWindows reads ~/.codex/tingly-model-catalog.json as slug ->
// {context_window, max_context_window}.
func codexCatalogWindows(t *testing.T, home string) map[string][2]int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".codex", "tingly-model-catalog.json"))
	require.NoError(t, err)
	var file struct {
		Models []struct {
			Slug   string `json:"slug"`
			Window int    `json:"context_window"`
			Max    int    `json:"max_context_window"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(raw, &file))
	out := map[string][2]int{}
	for _, m := range file.Models {
		out[m.Slug] = [2]int{m.Window, m.Max}
	}
	return out
}

// useLastSyncedCatalog gives cfg the provider catalog a CLI command starts
// with (app.AppManager): the one the server last synced, here none yet.
func useLastSyncedCatalog(t *testing.T, cfg *serverconfig.Config) {
	t.Helper()
	tm := catalog.NewProviderCatalogManager(catalog.LastSynced())
	require.NoError(t, tm.Initialize(context.Background()))
	cfg.SetTemplateManager(tm)
}

// `tb agent apply codex --provider … --model …` points the tingly-codex rule
// at the chosen model before writing the files, so the first apply already
// lists that rule in the catalog with the model's real window.
func TestApplyAgent_CodexWritesWindowOfTheModelBeingSet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg, err := serverconfig.NewConfigWithDir(t.TempDir())
	require.NoError(t, err)
	useLastSyncedCatalog(t, cfg)
	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID: "ds", Name: "DeepSeek", APIBase: "https://api.deepseek.com/v1",
		APIStyle: protocol.APIStyleOpenAI, AuthType: typ.AuthTypeAPIKey, Token: "t", Enabled: true,
	}))

	aa := NewAgentApply(cfg, "localhost")
	result, err := aa.ApplyAgent(&ApplyAgentRequest{AgentType: AgentTypeCodex, Provider: "ds", Model: "deepseek-chat"})
	require.NoError(t, err)
	require.True(t, result.Success, result.Message)
	assert.Equal(t, "DeepSeek", result.ProviderName)
	assert.Equal(t, 1, result.RulesCreated+result.RulesUpdated)
	assert.Equal(t, [2]int{128000, 128000}, codexCatalogWindows(t, home)["tingly-codex"])

	// Switching the model re-derives the window on the same apply.
	result, err = aa.ApplyAgent(&ApplyAgentRequest{AgentType: AgentTypeCodex, Provider: "ds", Model: "deepseek-v4-flash"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.RulesUpdated)
	assert.Equal(t, [2]int{1000000, 1000000}, codexCatalogWindows(t, home)["tingly-codex"])
}

// A provider that doesn't exist is a warning: the files are still written and
// no rule is touched.
func TestApplyAgent_UnknownProviderStillWritesFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg, err := serverconfig.NewConfigWithDir(t.TempDir())
	require.NoError(t, err)

	result, err := NewAgentApply(cfg, "localhost").ApplyAgent(&ApplyAgentRequest{AgentType: AgentTypeCodex, Provider: "missing", Model: "m"})
	require.NoError(t, err)
	assert.True(t, result.Success, result.Message)
	assert.Contains(t, result.Warnings, "provider missing not found; skipping routing rule update")
	assert.Zero(t, result.RulesCreated+result.RulesUpdated)
	assert.Empty(t, result.ProviderName)
	assert.Nil(t, cfg.GetRuleByRequestModelAndScenario("tingly-codex", typ.ScenarioCodex))
}

// The rule is saved before the files are written; when writing them fails the
// result (or error) says the rule already changed.
func TestApplyAgent_FileFailureSaysRulesChanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// ~/.codex can't be created: a file is in the way.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".codex"), nil, 0o644))
	cfg, err := serverconfig.NewConfigWithDir(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID: "ds", Name: "DeepSeek", APIBase: "https://api.deepseek.com/v1",
		APIStyle: protocol.APIStyleOpenAI, AuthType: typ.AuthTypeAPIKey, Token: "t", Enabled: true,
	}))
	aa := NewAgentApply(cfg, "localhost")

	result, err := aa.ApplyAgent(&ApplyAgentRequest{AgentType: AgentTypeCodex, Provider: "ds", Model: "deepseek-chat"})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, "Configuration application failed; routing rules now use DeepSeek / deepseek-chat", result.Message)
	assert.NotNil(t, cfg.GetRuleByRequestModelAndScenario("tingly-codex", typ.ScenarioCodex))

	// Nothing routed: no mention of rules.
	result, err = aa.ApplyAgent(&ApplyAgentRequest{AgentType: AgentTypeCodex})
	require.NoError(t, err)
	assert.Equal(t, "Configuration application failed", result.Message)

	// A failure that comes back as an error says it too.
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	_, err = aa.ApplyAgent(&ApplyAgentRequest{AgentType: AgentTypeCodex, Provider: "ds", Model: "deepseek-v4-flash"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "routing rules now use DeepSeek / deepseek-v4-flash, but writing the config files failed")

	// Routing warnings are kept on that path too.
	_, err = aa.ApplyAgent(&ApplyAgentRequest{AgentType: AgentTypeCodex, Provider: "nope", Model: "deepseek-chat"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider nope not found")
}

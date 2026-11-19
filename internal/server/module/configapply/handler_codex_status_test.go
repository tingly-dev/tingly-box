package configapply

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// codexStatusRig drives the real apply and status handlers against an
// isolated HOME, the way the Agent page does: apply, then read the status
// back. Applying must always end in "applied" — anything else is a diff the
// user cannot clear.
type codexStatusRig struct {
	t      *testing.T
	cfg    *config.Config
	router *gin.Engine
	home   string
}

func newCodexStatusRig(t *testing.T, rules []typ.Rule) *codexStatusRig {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	cfg.Rules = rules
	h := NewHandler(cfg, "localhost")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/apply", h.ApplyCodexConfigFromState)
	router.GET("/status", h.GetCodexConfigStatus)
	return &codexStatusRig{t: t, cfg: cfg, router: router, home: home}
}

func (r *codexStatusRig) apply(body string) {
	r.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/apply", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.router.ServeHTTP(w, req)
	require.Equal(r.t, http.StatusOK, w.Code, w.Body.String())
	var resp ApplyCodexConfigResponse
	require.NoError(r.t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(r.t, resp.Success, w.Body.String())
}

func (r *codexStatusRig) status() ClientConfigStatusResponse {
	r.t.Helper()
	w := httptest.NewRecorder()
	r.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/status", nil))
	require.Equal(r.t, http.StatusOK, w.Code, w.Body.String())
	var resp ClientConfigStatusResponse
	require.NoError(r.t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func (r *codexStatusRig) configPath() string { return filepath.Join(r.home, ".codex", "config.toml") }

func codexRule(uuid, model string, mutate ...func(*typ.Rule)) typ.Rule {
	rule := typ.Rule{UUID: uuid, Scenario: typ.ScenarioCodex, RequestModel: model, Active: true}
	for _, m := range mutate {
		m(&rule)
	}
	return rule
}

func with1M(r *typ.Rule) { r.Flags.Context1M = true }

func TestCodexStatus_AppliedRightAfterApply(t *testing.T) {
	rig := newCodexStatusRig(t, []typ.Rule{
		codexRule("a", "gpt-5.6-luna"),
		codexRule("b", "gpt-5.6-terra", with1M),
		codexRule("c", " gpt-5.6-sol "), // surrounding spaces are trimmed on both sides
	})
	rig.apply(`{"authMode":"apikey"}`)
	got := rig.status()
	assert.Equal(t, config.ClientConfigApplied, got.State, "%+v", got.Differences)
	assert.Empty(t, got.Differences)
}

// Codex itself rewrites the top-level `model` when the user picks one with
// /model. Any model the gateway serves is as good as the first one, so that
// is not a difference between the file and the gateway.
func TestCodexStatus_ModelPickedInCodexIsNotADiff(t *testing.T) {
	rig := newCodexStatusRig(t, []typ.Rule{codexRule("a", "gpt-5.6-luna"), codexRule("b", "gpt-5.6-terra")})
	rig.apply(`{}`)

	raw, err := os.ReadFile(rig.configPath())
	require.NoError(t, err)
	edited := regexp.MustCompile(`(?m)^model = .*$`).ReplaceAll(raw, []byte(`model = 'gpt-5.6-terra'`))
	require.NotEqual(t, string(raw), string(edited), "the applied file should carry a top-level model")
	require.NoError(t, os.WriteFile(rig.configPath(), edited, 0o644))

	got := rig.status()
	assert.Equal(t, config.ClientConfigApplied, got.State, "%+v", got.Differences)

	// A model that no rule serves any more is a real difference.
	edited = regexp.MustCompile(`(?m)^model = .*$`).ReplaceAll(raw, []byte(`model = 'deleted-model'`))
	require.NoError(t, os.WriteFile(rig.configPath(), edited, 0o644))
	got = rig.status()
	assert.Equal(t, config.ClientConfigOutdated, got.State)
	require.Len(t, got.Differences, 1)
	assert.Equal(t, "model", got.Differences[0].Key)
}

// Re-applying with the catalog switched off must leave a file that reads back
// as applied; otherwise the catalog written earlier lingers, goes stale as the
// rules change, and no apply can clear the difference.
func TestCodexStatus_AppliedWithoutCatalogAfterRulesChange(t *testing.T) {
	rig := newCodexStatusRig(t, []typ.Rule{codexRule("a", "gpt-5.6-luna")})
	rig.apply(`{}`)

	rig.cfg.Rules = append(rig.cfg.Rules, codexRule("b", "gpt-5.6-terra"))
	rig.apply(`{"writeCatalog":false}`)

	got := rig.status()
	assert.Equal(t, config.ClientConfigApplied, got.State, "%+v", got.Differences)
}

// With no active Codex rule an apply writes no model list, so there is
// nothing for the catalog to disagree with.
func TestCodexStatus_NoRulesLeavesNothingToCompare(t *testing.T) {
	rig := newCodexStatusRig(t, []typ.Rule{codexRule("a", "gpt-5.6-luna")})
	rig.apply(`{}`)

	rig.cfg.Rules = nil
	rig.apply(`{}`)

	got := rig.status()
	assert.Equal(t, config.ClientConfigApplied, got.State, "%+v", got.Differences)
}

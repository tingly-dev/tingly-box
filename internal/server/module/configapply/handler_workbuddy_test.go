package configapply

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/swagger"
)

func workBuddyTestRouter(t *testing.T, rules []typ.Rule) (*gin.Engine, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".workbuddy"), 0o755))
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	cfg.Rules = rules
	h := NewHandler(cfg, "localhost")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/apply/workbuddy", h.ApplyWorkBuddyConfigFromState)
	r.GET("/preview/workbuddy", h.GetWorkBuddyConfigPreview)
	r.GET("/status/workbuddy", h.GetWorkBuddyConfigStatus)
	r.POST("/restore/workbuddy", h.RestoreWorkBuddyConfig)
	return r, filepath.Join(home, ".workbuddy", "models.json")
}

func doWB(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestApplyWorkBuddyConfig_NoRulesWritesNothing(t *testing.T) {
	r, file := workBuddyTestRouter(t, nil)
	w := doWB(r, http.MethodPost, "/apply/workbuddy")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"success":false`)
	assert.Contains(t, w.Body.String(), "Add a model rule")
	_, err := os.Stat(file)
	assert.True(t, os.IsNotExist(err), "no file should be created without rules")
}

func TestWorkBuddyConfig_ApplyStatusPreviewRestore(t *testing.T) {
	r, file := workBuddyTestRouter(t, []typ.Rule{
		{UUID: "wb1", Scenario: typ.ScenarioWorkBuddy, RequestModel: "claude-sonnet-5", Active: true},
		{UUID: "wb2", Scenario: typ.ScenarioWorkBuddy, RequestModel: "inactive", Active: false},
		{UUID: "cc1", Scenario: typ.ScenarioClaudeCode, RequestModel: "other-scenario", Active: true},
	})
	require.NoError(t, os.WriteFile(file, []byte(`[{"id":"mine","url":"https://x/v1/chat/completions"}]`), 0o600))

	status := doWB(r, http.MethodGet, "/status/workbuddy").Body.String()
	assert.Contains(t, status, `"state":"not_applied"`)
	assert.Contains(t, status, `~/.workbuddy/models.json`)
	assert.NotContains(t, status, `.workbuddy-ai`, "only the installed edition is listed")

	prev := doWB(r, http.MethodGet, "/preview/workbuddy").Body.String()
	assert.Contains(t, prev, `claude-sonnet-5`)
	assert.NotContains(t, prev, "inactive")
	assert.NotContains(t, prev, "other-scenario")

	w := doWB(r, http.MethodPost, "/apply/workbuddy")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"success":true`)
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"mine"`, "user model kept")
	assert.Contains(t, string(data), `"claude-sonnet-5"`)
	assert.Contains(t, string(data), "/tingly/workbuddy/v1/chat/completions")

	assert.Contains(t, doWB(r, http.MethodGet, "/status/workbuddy").Body.String(), `"state":"applied"`)

	// Backup file names are second-granular: apply's backup and restore's
	// pre-restore backup would collide if issued within the same second.
	time.Sleep(1100 * time.Millisecond)
	rest := doWB(r, http.MethodPost, "/restore/workbuddy")
	assert.Contains(t, rest.Body.String(), `"success":true`)
	data, err = os.ReadFile(file)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "claude-sonnet-5", "restore returns the pre-apply content")
	assert.Contains(t, string(data), `"mine"`)
}

// One edition restored, the other without a backup: not a success, and the
// message names both what was restored and what failed.
func TestRestoreWorkBuddyConfig_PartialRestoreIsNotSuccess(t *testing.T) {
	r, file := workBuddyTestRouter(t, []typ.Rule{
		{UUID: "wb1", Scenario: typ.ScenarioWorkBuddy, RequestModel: "claude-sonnet-5", Active: true},
	})
	home := filepath.Dir(filepath.Dir(file))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".workbuddy-ai"), 0o755))
	require.NoError(t, os.WriteFile(file, []byte(`[{"id":"mine","url":"https://x/v1/chat/completions"}]`), 0o600))

	// The China file exists (backed up); the international one is created (no backup).
	require.Contains(t, doWB(r, http.MethodPost, "/apply/workbuddy").Body.String(), `"success":true`)

	time.Sleep(1100 * time.Millisecond)
	var resp RestoreConfigResponse
	require.NoError(t, json.Unmarshal(doWB(r, http.MethodPost, "/restore/workbuddy").Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	require.Len(t, resp.RestoredFiles, 1)
	assert.Contains(t, resp.RestoredFiles[0], "~/.workbuddy/models.json")
	require.Len(t, resp.Failures, 1)
	assert.Contains(t, resp.Failures[0], "~/.workbuddy-ai/models.json")
	assert.Contains(t, resp.Message, "~/.workbuddy/models.json")
	assert.Contains(t, resp.Message, "~/.workbuddy-ai/models.json")

	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "claude-sonnet-5", "the edition with a backup is still restored")
}

// A route that is written but never registered fails silently (the frontend just
// gets a 404), so pin the full WorkBuddy surface through RegisterRoutes.
func TestRegisterRoutes_WorkBuddyEndpointsAreMounted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	manager := swagger.NewRouteManager(engine)
	NewHandler(cfg, "localhost").RegisterRoutes(&module.Routes{V1: manager.NewGroup("api", "v1", "")})

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/config/workbuddy/status"},
		{http.MethodGet, "/api/v1/config/preview/workbuddy"},
		{http.MethodPost, "/api/v1/config/apply/workbuddy"},
		{http.MethodPost, "/api/v1/config/restore/workbuddy"},
	} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		assert.NotEqual(t, http.StatusNotFound, w.Code, "%s %s is not registered", tc.method, tc.path)
	}
}

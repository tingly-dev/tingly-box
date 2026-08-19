package configapply

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func setupTestRouter(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	_ = NewHandler(cfg, "localhost")
	return router
}

func TestNewHandler(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	handler := NewHandler(cfg, "localhost")

	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.config != cfg {
		t.Error("expected config to be set")
	}
	if handler.host != "localhost" {
		t.Errorf("expected host 'localhost', got %q", handler.host)
	}
}

func TestGetClaudeConfig_RestoresAppliedPreferences(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	settingsDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(settingsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(`{
		"env":{"CLAUDE_CODE_MAX_OUTPUT_TOKENS":"64000"},
		"defaultMode":"plan",
		"showThinkingSummaries":false,
		"statusLine":{"type":"command","command":"status.sh"}
	}`), 0644))

	handler := NewHandler(nil, "localhost")
	router := gin.New()
	router.GET("/config/claude", handler.GetClaudeConfig)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config/claude", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response ClaudeConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.True(t, response.Exists)
	assert.True(t, response.InstallStatusLine)
	assert.Equal(t, "plan", response.DefaultMode)
	assert.Equal(t, "64000", response.Preferences.ClaudeCodeMaxOutputTokens)
	assert.False(t, response.ShowThinkingSummaries)
}

func TestGetCodexConfig_RestoresAppliedPreferences(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	codexDir := filepath.Join(home, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0755))
	// A tingly-managed config.toml carrying the four prefs keys plus the
	// catalog reference and the tingly provider stanza.
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(`
model = "tingly/codex"
model_provider = "tingly-box"
model_catalog_json = "/home/user/.codex/tingly-model-catalog.json"
model_reasoning_effort = "high"
model_reasoning_summary = "detailed"
model_verbosity = "low"
model_supports_reasoning_summaries = true

[model_providers.tingly-box]
name = "Tingly Box"
base_url = "http://localhost:12580/tingly/codex"
wire_api = "responses"
`), 0644))

	handler := NewHandler(nil, "localhost")
	router := gin.New()
	router.GET("/config/codex", handler.GetCodexConfig)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config/codex", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response CodexConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.True(t, response.Exists, "tingly-managed config should report exists=true")
	assert.True(t, response.WriteCatalog, "model_catalog_json present → writeCatalog=true")
	assert.Equal(t, "high", response.Preferences.ModelReasoningEffort)
	assert.Equal(t, "detailed", response.Preferences.ModelReasoningSummary)
	assert.Equal(t, "low", response.Preferences.ModelVerbosity)
	assert.Equal(t, "true", response.Preferences.ModelSupportsReasoningSummaries)
}

// A config.toml with no tingly footprint reads as not-applied: the form falls
// back to defaults rather than restoring stale values.
func TestGetCodexConfig_NotAppliedFallsBackToDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// No ~/.codex/config.toml at all.
	handler := NewHandler(nil, "localhost")
	router := gin.New()
	router.GET("/config/codex", handler.GetCodexConfig)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config/codex", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response CodexConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.False(t, response.Exists)
	assert.False(t, response.WriteCatalog)
	// Defaults are still returned so the form has sensible starting values.
	assert.Equal(t, "medium", response.Preferences.ModelReasoningEffort)
}

func TestApplyClaudeConfig_NilConfig(t *testing.T) {
	handler := NewHandler(nil, "localhost")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/apply/claude", handler.ApplyClaudeConfig)

	req, _ := http.NewRequest("POST", "/apply/claude", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Global config not available")
}

// A request body with valid preferences now succeeds even without routing
// rules — config files can be written before models are configured.
func TestApplyClaudeConfig_SucceedsWithoutRules(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := config.NewConfig(config.WithConfigDir(tmpDir))
	require.NoError(t, err)
	handler := NewHandler(cfg, "localhost")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/apply/claude", handler.ApplyClaudeConfig)

	body, _ := json.Marshal(ApplyClaudeConfigRequest{
		Preferences: &agent.ClaudeCodePrefs{
			AnthropicModel:   "tingly/cc",
			APITimeoutMs:     "3000000",
			DisableTelemetry: "1",
		},
	})
	req, _ := http.NewRequest("POST", "/apply/claude", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 (apply without rules), got %d. body: %s",
			w.Code, w.Body.String())
	}
	resp := w.Body.String()
	assert.Contains(t, resp, `"success":true`)
}

// preferences is required: missing it (or sending nil) is a client error,
// not a 500. Guards against accidentally re-introducing the legacy
// mode-only fallback.
func TestApplyClaudeConfig_RequiresPreferences(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := config.NewConfig(config.WithConfigDir(tmpDir))
	require.NoError(t, err)
	handler := NewHandler(cfg, "localhost")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/apply/claude", handler.ApplyClaudeConfig)

	body := []byte(`{"installStatusLine":false}`)
	req, _ := http.NewRequest("POST", "/apply/claude", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d. body: %s", w.Code, w.Body.String())
	}
	assert.Contains(t, w.Body.String(), "preferences field is required")
}

// Malformed JSON returns a structured 400 — never a 500 or panic.
func TestApplyClaudeConfig_MalformedBodyReturns400(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := config.NewConfig(config.WithConfigDir(tmpDir))
	require.NoError(t, err)
	handler := NewHandler(cfg, "localhost")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/apply/claude", handler.ApplyClaudeConfig)

	req, _ := http.NewRequest("POST", "/apply/claude", strings.NewReader("not-json{"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d. body: %s", w.Code, w.Body.String())
	}
	assert.Contains(t, w.Body.String(), "Invalid request body")
}

// Verify the wire shape — frontend serializes preferences with env-name
// JSON keys; the handler binds them into the typed struct.
func TestApplyClaudeConfigRequest_JSONShape(t *testing.T) {
	wire := []byte(`{
			"installStatusLine": true,
			"defaultMode": "delegate",
			"showThinkingSummaries": false,
			"preferences": {
			"ANTHROPIC_MODEL": "tingly/cc-default",
			"ANTHROPIC_DEFAULT_SONNET_MODEL": "tingly/cc-sonnet[1m]",
			"API_TIMEOUT_MS": "3000000",
			"DISABLE_TELEMETRY": "1"
		}
	}`)

	var req ApplyClaudeConfigRequest
	if err := json.Unmarshal(wire, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !req.InstallStatusLine {
		t.Error("InstallStatusLine = false, want true")
	}
	if req.DefaultMode != "delegate" {
		t.Errorf("DefaultMode = %q", req.DefaultMode)
	}
	if req.ShowThinkingSummaries == nil || *req.ShowThinkingSummaries != false {
		t.Errorf("ShowThinkingSummaries = %v, want false", req.ShowThinkingSummaries)
	}
	if req.Preferences == nil {
		t.Fatal("Preferences = nil, want populated")
	}
	if req.Preferences.AnthropicModel != "tingly/cc-default" {
		t.Errorf("AnthropicModel = %q", req.Preferences.AnthropicModel)
	}
	if req.Preferences.AnthropicDefaultSonnetModel != "tingly/cc-sonnet[1m]" {
		t.Errorf("AnthropicDefaultSonnetModel = %q", req.Preferences.AnthropicDefaultSonnetModel)
	}
	if req.Preferences.APITimeoutMs != "3000000" {
		t.Errorf("APITimeoutMs = %q", req.Preferences.APITimeoutMs)
	}
	if req.Preferences.DisableTelemetry != "1" {
		t.Errorf("DisableTelemetry = %q", req.Preferences.DisableTelemetry)
	}
}

func TestApplyOpenCodeConfig_NilConfig(t *testing.T) {
	handler := NewHandler(nil, "localhost")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/apply/opencode", handler.ApplyOpenCodeConfigFromState)

	req, _ := http.NewRequest("POST", "/apply/opencode", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Global config not available")
}

func TestApplyOpenCodeConfig_SucceedsWithoutRules(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := config.NewConfig(config.WithConfigDir(tmpDir))
	require.NoError(t, err)
	handler := NewHandler(cfg, "localhost")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/apply/opencode", handler.ApplyOpenCodeConfigFromState)

	req, _ := http.NewRequest("POST", "/apply/opencode", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Now succeeds even without rules — config files can be written independently.
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d. body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
}

func TestGetOpenCodeConfigPreview_IncludesRuleModelsAndModalities(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := config.NewConfig(config.WithConfigDir(tmpDir))
	require.NoError(t, err)
	cfg.Rules = []typ.Rule{
		{UUID: "oc1", Scenario: typ.ScenarioOpenCode, RequestModel: "tingly-opencode-vision", Active: true},
		{UUID: "oc2", Scenario: typ.ScenarioOpenCode, RequestModel: "inactive-model", Active: false},
		{UUID: "cc1", Scenario: typ.ScenarioClaudeCode, RequestModel: "other-scenario-model", Active: true},
	}
	handler := NewHandler(cfg, "localhost")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/preview/opencode", handler.GetOpenCodeConfigPreview)

	req, _ := http.NewRequest("GET", "/preview/opencode", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp OpenCodeConfigPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(resp.ConfigJSON), &payload))

	providers := payload["provider"].(map[string]interface{})
	tb := providers["tingly-box"].(map[string]interface{})
	models := tb["models"].(map[string]interface{})

	// Only the active OpenCode-scenario rule's model should be present.
	assert.Len(t, models, 1)
	entry, ok := models["tingly-opencode-vision"].(map[string]interface{})
	require.True(t, ok, "expected model entry for the active OpenCode rule, got %v", models)
	assert.NotContains(t, models, "inactive-model")
	assert.NotContains(t, models, "other-scenario-model")

	// The model must declare attachment/modalities support so opencode
	// doesn't default image input to unavailable (see openCodeModelEntry).
	assert.Equal(t, true, entry["attachment"])
	modalities, ok := entry["modalities"].(map[string]interface{})
	require.True(t, ok, "expected modalities object, got %v", entry["modalities"])
	assert.ElementsMatch(t, []interface{}{"text", "image"}, modalities["input"])
	assert.ElementsMatch(t, []interface{}{"text"}, modalities["output"])
}

func TestGetOpenCodeConfigPreview_NilConfig(t *testing.T) {
	handler := NewHandler(nil, "localhost")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/preview/opencode", handler.GetOpenCodeConfigPreview)

	req, _ := http.NewRequest("GET", "/preview/opencode", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Global config not available")
}

func TestGetOpenCodeConfigPreview_SucceedsWithoutRules(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := config.NewConfig(config.WithConfigDir(tmpDir))
	require.NoError(t, err)
	handler := NewHandler(cfg, "localhost")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/preview/opencode", handler.GetOpenCodeConfigPreview)

	req, _ := http.NewRequest("GET", "/preview/opencode", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Now succeeds even without rules — preview is independent of routing setup.
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d. body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
}

func TestApplyConfigResponseStructure(t *testing.T) {
	settingsResult := config.ApplyResult{
		Success:    true,
		Created:    true,
		BackupPath: "/backup/settings.json.backup",
		Message:    "Settings applied successfully",
	}

	onboardingResult := config.ApplyResult{
		Success:    true,
		Created:    false,
		BackupPath: "/backup/claude.json.backup",
		Message:    "Onboarding applied successfully",
	}

	response := ApplyConfigResponse{
		Success:          true,
		SettingsResult:   settingsResult,
		OnboardingResult: onboardingResult,
		CreatedFiles:     []string{"~/.claude/settings.json"},
		UpdatedFiles:     []string{"~/.claude.json"},
		BackupPaths:      []string{"/backup/settings.json.backup", "/backup/claude.json.backup"},
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if !response.SettingsResult.Success {
		t.Error("expected SettingsResult.Success to be true")
	}

	if !response.OnboardingResult.Success {
		t.Error("expected OnboardingResult.Success to be true")
	}

	if len(response.CreatedFiles) != 1 {
		t.Errorf("expected 1 created file, got %d", len(response.CreatedFiles))
	}

	if len(response.UpdatedFiles) != 1 {
		t.Errorf("expected 1 updated file, got %d", len(response.UpdatedFiles))
	}

	if len(response.BackupPaths) != 2 {
		t.Errorf("expected 2 backup paths, got %d", len(response.BackupPaths))
	}
}

func TestApplyOpenCodeConfigResponseStructure(t *testing.T) {
	applyResult := config.ApplyResult{
		Success:    true,
		Created:    true,
		BackupPath: "/backup/opencode.json.backup",
		Message:    "OpenCode config applied successfully",
	}

	response := ApplyOpenCodeConfigResponse{
		ApplyResult: applyResult,
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if response.BackupPath != "/backup/opencode.json.backup" {
		t.Errorf("expected BackupPath '/backup/opencode.json.backup', got %q", response.BackupPath)
	}
}

func TestOpenCodeConfigPreviewResponseStructure(t *testing.T) {
	response := OpenCodeConfigPreviewResponse{
		Success:    true,
		ConfigJSON: `{"schema": "https://opencode.ai/config.json"}`,
		ScriptWin:  "# PowerShell script",
		ScriptUnix: "# Bash script",
		Message:    "Config preview generated successfully",
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if response.ConfigJSON == "" {
		t.Error("expected ConfigJSON to be non-empty")
	}

	if response.ScriptWin == "" {
		t.Error("expected ScriptWin to be non-empty")
	}

	if response.ScriptUnix == "" {
		t.Error("expected ScriptUnix to be non-empty")
	}

	if response.Message != "Config preview generated successfully" {
		t.Errorf("expected Message 'Config preview generated successfully', got %q", response.Message)
	}
}

func TestGenerateOpenCodeScript_Windows(t *testing.T) {
	configBaseURL := "http://localhost:12580/tingly/opencode"
	apiKey := "test-api-key"
	modelsJSON := `{"tingly/cc-default":{"name":"tingly/cc-default"}}`

	script := generateOpenCodeScript(configBaseURL, apiKey, modelsJSON, "windows")

	if script == "" {
		t.Fatal("expected script to be non-empty")
	}

	// Check for Windows-specific markers
	if !contains(script, "# PowerShell") {
		t.Error("expected Windows script to contain PowerShell marker")
	}

	if !contains(script, "node -e @\"") {
		t.Error("expected Windows script to contain node -e @")
	}

	if !contains(script, configBaseURL) {
		t.Error("expected script to contain base URL")
	}

	if !contains(script, apiKey) {
		t.Error("expected script to contain API key")
	}
}

func TestGenerateOpenCodeScript_Unix(t *testing.T) {
	configBaseURL := "http://localhost:12580/tingly/opencode"
	apiKey := "test-api-key"
	modelsJSON := `{"tingly/cc-default":{"name":"tingly/cc-default"}}`

	script := generateOpenCodeScript(configBaseURL, apiKey, modelsJSON, "unix")

	if script == "" {
		t.Fatal("expected script to be non-empty")
	}

	// Check for Unix-specific markers
	if !contains(script, "# Bash") {
		t.Error("expected Unix script to contain Bash marker")
	}

	if !contains(script, "node -e '") {
		t.Error("expected Unix script to contain node -e '")
	}

	if !contains(script, configBaseURL) {
		t.Error("expected script to contain base URL")
	}

	if !contains(script, apiKey) {
		t.Error("expected script to contain API key")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

package configapply

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/server/config"
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

// A request body with valid preferences must reach the rule-lookup stage.
// With no rules configured, the response is a NoActiveRules error — but
// that's downstream of binding, which is what this test exercises.
func TestApplyClaudeConfig_AcceptsPreferencesPayload(t *testing.T) {
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

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d (NoActiveRules), got %d. body: %s",
			http.StatusBadRequest, w.Code, w.Body.String())
	}
	resp := w.Body.String()
	assert.Contains(t, resp, `"success":false`)
	assert.True(t,
		strings.Contains(resp, "No active Claude Code rules found") ||
			strings.Contains(resp, "No services configured"),
		"expected NoActiveRules-style error after binding succeeded")
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

func TestApplyOpenCodeConfig_NoActiveRules(t *testing.T) {
	// Create a config with a temp directory (no built-in rules)
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

	// Should return an error (either no rules or no services)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	// Error message depends on whether built-in rules are loaded
	// Can be "No active OpenCode rules found" or "No services configured in OpenCode rule"
	assert.True(t,
		strings.Contains(body, "No active OpenCode rules found") ||
			strings.Contains(body, "No services configured"),
		"Expected error about no rules or no services")
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

func TestGetOpenCodeConfigPreview_NoActiveRules(t *testing.T) {
	// Create a config with a temp directory (no built-in rules)
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

	// Should return an error (either no rules or no services)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	// Error message depends on whether built-in rules are loaded
	// Can be "No active OpenCode rules found" or "No services configured in OpenCode rule"
	assert.True(t,
		strings.Contains(body, "No active OpenCode rules found") ||
			strings.Contains(body, "No services configured"),
		"Expected error about no rules or no services")
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

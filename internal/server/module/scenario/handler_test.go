package scenario

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// mockRemoteControlController is a mock implementation of RemoteControlController
type mockRemoteControlController struct {
	startCalled bool
	stopCalled  bool
	syncCalled  bool
	startErr    error
	syncErr     error
}

func (m *mockRemoteControlController) StartRemoteCoder() error {
	m.startCalled = true
	return m.startErr
}

func (m *mockRemoteControlController) StopRemoteCoder() {
	m.stopCalled = true
}

func (m *mockRemoteControlController) SyncRemoteCoderBots(ctx context.Context) error {
	m.syncCalled = true
	return m.syncErr
}

func setupTestRouter(cfg *config.Config, rcCtrl RemoteControlController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	_ = NewHandler(cfg, rcCtrl)
	return router
}

func TestNewHandler(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	mockRC := &mockRemoteControlController{}

	handler := NewHandler(cfg, mockRC)

	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.config != cfg {
		t.Error("expected config to be set")
	}
	if handler.rcControl != mockRC {
		t.Error("expected rcControl to be set")
	}
}

func TestGetScenarios_Success(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	mockRC := &mockRemoteControlController{}
	router := setupTestRouter(cfg, mockRC)
	handler := NewHandler(cfg, mockRC)

	router.GET("/scenarios", handler.GetScenarios)

	req, _ := http.NewRequest("GET", "/scenarios", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
}

func TestGetScenarios_NilConfig(t *testing.T) {
	mockRC := &mockRemoteControlController{}
	router := setupTestRouter(nil, mockRC)
	handler := NewHandler(nil, mockRC)

	router.GET("/scenarios", handler.GetScenarios)

	req, _ := http.NewRequest("GET", "/scenarios", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Global config not available")
}

func TestGetScenarioConfig_Success(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	mockRC := &mockRemoteControlController{}
	router := setupTestRouter(cfg, mockRC)
	handler := NewHandler(cfg, mockRC)

	router.GET("/scenarios/:scenario", handler.GetScenarioConfig)

	// Add a test scenario
	testScenario := typ.RuleScenario("test_scenario")
	testConfig := typ.ScenarioConfig{
		Scenario: testScenario,
		Flags: typ.ScenarioFlags{
			Unified: true,
		},
	}
	cfg.SetScenarioConfig(testConfig)

	req, _ := http.NewRequest("GET", "/scenarios/test_scenario", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
	assert.Contains(t, body, "test_scenario")
}

func TestGetScenarioConfig_NotFound(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	mockRC := &mockRemoteControlController{}
	router := setupTestRouter(cfg, mockRC)
	handler := NewHandler(cfg, mockRC)

	router.GET("/scenarios/:scenario", handler.GetScenarioConfig)

	req, _ := http.NewRequest("GET", "/scenarios/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Scenario config not found")
}

func TestGetScenarioConfig_EmptyScenario(t *testing.T) {
	cfg, _ := config.NewConfig(config.WithConfigDir(t.TempDir()))
	mockRC := &mockRemoteControlController{}
	router := setupTestRouter(cfg, mockRC)
	handler := NewHandler(cfg, mockRC)

	router.GET("/scenarios/:scenario", handler.GetScenarioConfig)

	req, _ := http.NewRequest("GET", "/scenarios/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Gin returns 404 for empty path param
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestGetScenarioConfig_NilConfig(t *testing.T) {
	mockRC := &mockRemoteControlController{}
	router := setupTestRouter(nil, mockRC)
	handler := NewHandler(nil, mockRC)

	router.GET("/scenarios/:scenario", handler.GetScenarioConfig)

	req, _ := http.NewRequest("GET", "/scenarios/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Global config not available")
}

func TestProfileClaudeConfigLifecycle(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work")
	if err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(cfg, &mockRemoteControlController{})
	router := gin.New()
	route := "/scenario/:scenario/profiles/:id/claude-config"
	router.GET(route, handler.GetProfileClaudeConfig)
	router.PUT(route, handler.UpdateProfileClaudeConfig)
	router.DELETE(route, handler.DeleteProfileClaudeConfig)
	endpoint := "/scenario/claude_code/profiles/" + profile.ID + "/claude-config"

	initialResult := httptest.NewRecorder()
	router.ServeHTTP(initialResult, httptest.NewRequest(http.MethodGet, endpoint, nil))
	assert.Equal(t, http.StatusOK, initialResult.Code, initialResult.Body.String())
	assert.Contains(t, initialResult.Body.String(), `"settingsPath":"`)
	assert.Contains(t, initialResult.Body.String(), `p1--work`)
	assert.Contains(t, initialResult.Body.String(), `settings.json`)
	assert.Contains(t, initialResult.Body.String(), `"settingsExists":false`)

	updateBody, err := json.Marshal(ProfileClaudeConfigRequest{
		Preferences: &agent.ClaudeCodePrefs{
			ClaudeCodeMaxOutputTokens:        "64000",
			ClaudeCodeMaxConcurrentSubagents: "4",
		},
		DefaultMode: "plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	updateReq := httptest.NewRequest(http.MethodPut, endpoint, bytes.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateResult := httptest.NewRecorder()
	router.ServeHTTP(updateResult, updateReq)
	assert.Equal(t, http.StatusOK, updateResult.Code, updateResult.Body.String())
	assert.Contains(t, updateResult.Body.String(), `"CLAUDE_CODE_MAX_OUTPUT_TOKENS":"64000"`)
	assert.Contains(t, updateResult.Body.String(), `"CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS":"4"`)
	assert.Contains(t, updateResult.Body.String(), `"defaultMode":"plan"`)
	assert.Contains(t, updateResult.Body.String(), `"hasOverrides":true`)
	assert.Contains(t, updateResult.Body.String(), `"settingsExists":true`)

	settingsPath, _, err := agent.InspectCCProfileSettings(profile.ID, profile.Name)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var generatedSettings struct {
		Env         map[string]string `json:"env"`
		DefaultMode string            `json:"defaultMode"`
		Permissions struct {
			DefaultMode string `json:"defaultMode"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(generated, &generatedSettings); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "64000", generatedSettings.Env["CLAUDE_CODE_MAX_OUTPUT_TOKENS"])
	assert.Equal(t, "4", generatedSettings.Env["CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS"])
	assert.Equal(t, "plan", generatedSettings.DefaultMode, "legacy top-level defaultMode should still be mirrored on disk")
	assert.Equal(t, "plan", generatedSettings.Permissions.DefaultMode, "the file Claude Code actually reads must carry permissions.defaultMode")

	stored, ok := cfg.GetProfile(typ.ScenarioClaudeCode, profile.ID)
	if !ok || stored.ClaudeCode == nil {
		t.Fatalf("profile override was not persisted: %#v", stored)
	}
	assert.Equal(t, "64000", stored.ClaudeCode.Env["CLAUDE_CODE_MAX_OUTPUT_TOKENS"])
	assert.Equal(t, "4", stored.ClaudeCode.Env["CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS"])
	assert.Equal(t, "plan", stored.ClaudeCode.DefaultMode)

	getResult := httptest.NewRecorder()
	router.ServeHTTP(getResult, httptest.NewRequest(http.MethodGet, endpoint, nil))
	assert.Equal(t, http.StatusOK, getResult.Code, getResult.Body.String())
	assert.Contains(t, getResult.Body.String(), `"hasOverrides":true`)
	assert.Contains(t, getResult.Body.String(), `"CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS":"4"`)
	assert.Contains(t, getResult.Body.String(), `"defaultMode":"plan"`)

	deleteResult := httptest.NewRecorder()
	router.ServeHTTP(deleteResult, httptest.NewRequest(http.MethodDelete, endpoint, nil))
	assert.Equal(t, http.StatusOK, deleteResult.Code, deleteResult.Body.String())
	assert.Contains(t, deleteResult.Body.String(), `"hasOverrides":false`)
	cleared, _ := cfg.GetProfile(typ.ScenarioClaudeCode, profile.ID)
	assert.Nil(t, cleared.ClaudeCode)
}

func TestDuplicateProfileEndpoint(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work")
	if err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(cfg, &mockRemoteControlController{})
	router := gin.New()
	router.POST("/scenario/:scenario/profiles/:id/duplicate", handler.DuplicateProfile)
	post := func(id, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/scenario/claude_code/profiles/"+id+"/duplicate", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// An empty body is fine: the name is picked.
	w := post(profile.ID, "")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data typ.ProfileMeta `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "work-copy", resp.Data.Name)
	assert.NotEqual(t, profile.ID, resp.Data.ID)
	copyScenario := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, resp.Data.ID)
	assert.NotEmpty(t, cfg.ClaudeCodeRuleUUID(copyScenario, "cc"))

	// The copy gets its generated settings file, like a created profile.
	settingsPath, _, err := agent.InspectCCProfileSettings(resp.Data.ID, resp.Data.Name)
	assert.NoError(t, err)
	_, statErr := os.Stat(settingsPath)
	assert.NoError(t, statErr)

	// "default" copies the main Claude Code config.
	main := post("default", `{"name":"from-main"}`)
	assert.Equal(t, http.StatusOK, main.Code, main.Body.String())
	assert.Contains(t, main.Body.String(), `"from-main"`)

	// A chunked body with nothing in it (ContentLength -1) is empty too.
	chunked := httptest.NewRequest(http.MethodPost, "/scenario/claude_code/profiles/"+profile.ID+"/duplicate", io.NopCloser(bytes.NewReader(nil)))
	chunked.ContentLength = -1
	chunkedW := httptest.NewRecorder()
	router.ServeHTTP(chunkedW, chunked)
	assert.Equal(t, http.StatusOK, chunkedW.Code, chunkedW.Body.String())

	assert.Equal(t, http.StatusOK, post(profile.ID, `{"name":"named"}`).Code)
	assert.Equal(t, http.StatusBadRequest, post(profile.ID, `{"name":"work"}`).Code, "taken name")
	assert.Equal(t, http.StatusBadRequest, post(profile.ID, `{"name":"not valid"}`).Code)
	assert.Equal(t, http.StatusNotFound, post("p99", `{}`).Code, "unknown source")
	assert.Len(t, cfg.GetProfiles(typ.ScenarioClaudeCode), 5)
}

func TestScenarioTypes(t *testing.T) {
	// Test ScenarioFlagUpdateRequest
	flagReq := ScenarioFlagUpdateRequest{
		Value: true,
	}
	if flagReq.Value != true {
		t.Error("expected Value to be true")
	}

	// Test ScenarioStringFlagUpdateRequest
	stringFlagReq := ScenarioStringFlagUpdateRequest{
		Value: "test_value",
	}
	if stringFlagReq.Value != "test_value" {
		t.Error("expected Value to be 'test_value'")
	}

	// Test ScenarioUpdateRequest
	updateReq := ScenarioUpdateRequest{
		Scenario: "claude_code",
		Flags: typ.ScenarioFlags{
			Unified: true,
		},
	}
	if updateReq.Scenario != "claude_code" {
		t.Error("expected Scenario to be 'claude_code'")
	}
}

func TestScenariosResponseStructure(t *testing.T) {
	data := []typ.ScenarioConfig{
		{
			Scenario: "test_scenario",
			Flags: typ.ScenarioFlags{
				Unified: true,
			},
		},
	}

	response := ScenariosResponse{
		Success: true,
		Data:    data,
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if len(response.Data) != 1 {
		t.Errorf("expected 1 data item, got %d", len(response.Data))
	}

	if response.Data[0].Scenario != "test_scenario" {
		t.Errorf("expected Scenario 'test_scenario', got %q", response.Data[0].Scenario)
	}
}

func TestScenarioResponseStructure(t *testing.T) {
	data := typ.ScenarioConfig{
		Scenario: "test_scenario",
		Flags: typ.ScenarioFlags{
			Unified: true,
		},
	}

	response := ScenarioResponse{
		Success: true,
		Data:    data,
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if response.Data.Scenario != "test_scenario" {
		t.Errorf("expected Scenario 'test_scenario', got %q", response.Data.Scenario)
	}
}

func TestScenarioFlagResponseStructure(t *testing.T) {
	response := ScenarioFlagResponse{
		Success: true,
	}
	response.Data.Scenario = "claude_code"
	response.Data.Flag = "unified"
	response.Data.Value = true

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if response.Data.Scenario != "claude_code" {
		t.Errorf("expected Scenario 'claude_code', got %q", response.Data.Scenario)
	}

	if response.Data.Flag != "unified" {
		t.Errorf("expected Flag 'unified', got %q", response.Data.Flag)
	}

	if response.Data.Value != true {
		t.Error("expected Value to be true")
	}
}

func TestScenarioUpdateResponseStructure(t *testing.T) {
	data := typ.ScenarioConfig{
		Scenario: "claude_code",
		Flags: typ.ScenarioFlags{
			Unified: true,
		},
	}

	response := ScenarioUpdateResponse{
		Success: true,
		Message: "Scenario config saved successfully",
		Data:    data,
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if response.Message != "Scenario config saved successfully" {
		t.Errorf("expected Message 'Scenario config saved successfully', got %q", response.Message)
	}

	if response.Data.Scenario != "claude_code" {
		t.Errorf("expected Scenario 'claude_code', got %q", response.Data.Scenario)
	}
}

func TestClaudeCodeDefaultSwitchEndpoints(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "work")
	if err != nil {
		t.Fatal(err)
	}
	profiled := typ.ProfiledScenarioName(typ.ScenarioClaudeCode, profile.ID)
	profileRule := cfg.GetRuleByUUID("builtin:" + string(profiled) + ":cc")
	if profileRule == nil {
		t.Fatal("profile has no cc rule")
	}
	provider := &typ.Provider{UUID: "work-prov", Name: "work-prov", APIBase: "https://example.invalid", APIStyle: protocol.APIStyleOpenAI, Token: "k", Enabled: true}
	if err := cfg.AddProvider(provider); err != nil {
		t.Fatal(err)
	}
	profileRule.Services = append(profileRule.Services, &loadbalance.Service{Provider: provider.UUID, Model: "work-model", Active: true})
	if err := cfg.UpdateRule(profileRule.UUID, *profileRule); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(cfg, &mockRemoteControlController{})
	router := gin.New()
	route := "/scenario/:scenario/claude-code/default-switch"
	router.GET(route, handler.GetClaudeCodeDefaultSwitch)
	router.PUT(route, handler.SwitchClaudeCodeDefault)
	router.DELETE(route, handler.RestoreClaudeCodeDefault)
	call := func(method, scenario, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, "/scenario/"+scenario+"/claude-code/default-switch", bytes.NewBufferString(body)))
		return w
	}

	assert.Contains(t, call(http.MethodGet, "claude_code", "").Body.String(), `"switched":false`)

	// Only the default can be switched: not a profile, not another scenario.
	assert.Equal(t, http.StatusBadRequest, call(http.MethodPut, string(profiled), `{"profile":"work"}`).Code)
	assert.Equal(t, http.StatusBadRequest, call(http.MethodPut, "openai", `{"profile":"work"}`).Code)
	assert.Equal(t, http.StatusBadRequest, call(http.MethodPut, "claude_code", `{"profile":"missing"}`).Code)
	assert.Equal(t, http.StatusBadRequest, call(http.MethodDelete, "claude_code", "").Code, "nothing to restore yet")

	w := call(http.MethodPut, "claude_code", `{"profile":"`+profile.ID+`"}`)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"switched":true`)
	assert.Contains(t, w.Body.String(), `"profile_name":"work"`)
	assert.Contains(t, w.Body.String(), `"in_sync":true`)
	mainRule := cfg.GetRuleByUUID("builtin:claude_code:cc")
	if assert.NotNil(t, mainRule) && assert.NotEmpty(t, mainRule.Services) {
		assert.Equal(t, "work-model", mainRule.Services[0].Model)
	}

	w = call(http.MethodDelete, "claude_code", "")
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"switched":false`)
	assert.Empty(t, cfg.GetRuleByUUID("builtin:claude_code:cc").Services)
}

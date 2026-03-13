package scenario

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/server/config"
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
	cfg, _ := config.NewConfig()
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
	cfg, _ := config.NewConfig()
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
	cfg, _ := config.NewConfig()
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
	cfg, _ := config.NewConfig()
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
	cfg, _ := config.NewConfig()
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

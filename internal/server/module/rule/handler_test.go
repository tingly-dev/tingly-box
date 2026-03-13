package rule

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/obs"
)

// setupTestActionLogger creates a test action logger for use in tests
func setupTestActionLogger() *obs.ScopedLogger {
	multiLogger, _ := obs.NewMultiLogger(&obs.MultiLoggerConfig{
		TextLogPath: "/tmp/test.log",
		JSONLogPath: "/tmp/test.jsonl",
		MemorySinkConfig: map[obs.LogSource]obs.MemorySinkConfig{
			obs.LogSourceAction: {MaxEntries: 10},
		},
	})
	return multiLogger.WithSource(obs.LogSourceAction)
}

func setupTestRouter(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	_ = NewHandler(cfg, setupTestActionLogger())
	return router
}

func TestNewHandler(t *testing.T) {
	actionLogger := setupTestActionLogger()
	handler := NewHandler(nil, actionLogger)

	cfg, _ := config.NewConfig()
	router := setupTestRouter(cfg)
	router.GET("/rules", handler.GetRules)

	req, _ := http.NewRequest("GET", "/rules", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !response["success"].(bool) {
		t.Error("expected success to be false")
	}
}

func TestGetRules_WithScenario(t *testing.T) {
	cfg, _ := config.NewConfig()
	router := setupTestRouter(cfg)

	actionLogger := setupTestActionLogger()
	handler := NewHandler(cfg, actionLogger)

	router.GET("/rules", handler.GetRules)

	// Create a test rule
	rule := &typ.Rule{
		UUID:         "test-uuid-123",
		Scenario:     "test_scenario",
		RequestModel: "gpt-4",
	}
	cfg.AddRule(*rule)

	req, _ := http.NewRequest("GET", "/rules?scenario=test_scenario", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !response["success"].(bool) {
		t.Error("expected success to be true")
	}

	data := response["data"].([]interface{})
	if len(data) != 1 {
		t.Errorf("expected 1 rule, got %d", len(data))
	}
}

func TestGetRules_NilConfig(t *testing.T) {
	router := setupTestRouter(nil)

	actionLogger := setupTestActionLogger()
	handler := NewHandler(nil, actionLogger)

	router.GET("/rules", handler.GetRules)

	req, _ := http.NewRequest("GET", "/rules?scenario=test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestGetRule_Success(t *testing.T) {
	cfg, _ := config.NewConfig()
	router := setupTestRouter(cfg)

	actionLogger := setupTestActionLogger()
	handler := NewHandler(cfg, actionLogger)

	router.GET("/rules/:uuid", handler.GetRule)

	// Create a test rule
	ruleUUID := "test-uuid-456"
	rule := &typ.Rule{
		UUID:         ruleUUID,
		Scenario:     "test_scenario",
		RequestModel: "gpt-4",
	}
	cfg.AddRule(*rule)

	req, _ := http.NewRequest("GET", "/rules/"+ruleUUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGetRule_NotFound(t *testing.T) {
	cfg, _ := config.NewConfig()
	router := setupTestRouter(cfg)

	actionLogger := setupTestActionLogger()
	handler := NewHandler(cfg, actionLogger)

	router.GET("/rules/:uuid", handler.GetRule)

	req, _ := http.NewRequest("GET", "/rules/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestGetRule_EmptyUUID(t *testing.T) {
	cfg, _ := config.NewConfig()
	router := setupTestRouter(cfg)

	actionLogger := setupTestActionLogger()
	handler := NewHandler(cfg, actionLogger)

	router.GET("/rules/:uuid", handler.GetRule)

	req, _ := http.NewRequest("GET", "/rules/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestGetRule_NilConfig(t *testing.T) {
	router := setupTestRouter(nil)

	actionLogger := setupTestActionLogger()
	handler := NewHandler(nil, actionLogger)

	router.GET("/rules/:uuid", handler.GetRule)

	testUUID := uuid.New().String()
	req, _ := http.NewRequest("GET", "/rules/"+testUUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

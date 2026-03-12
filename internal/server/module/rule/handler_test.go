package rule

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func setupTestRouter(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger := obs.NewMemoryLogger(100)
	handler := NewHandler(cfg, logger)
	return router
}

func TestNewHandler(t *testing.T) {
	logger := obs.NewMemoryLogger(100)
	handler := NewHandler(nil, logger)

	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestGetRules_NoScenario(t *testing.T) {
	cfg, _ := config.NewTestConfig()
	router := setupTestRouter(cfg)
	logger := obs.NewMemoryLogger(100)
	handler := NewHandler(cfg, logger)

	router.GET("/rules", handler.GetRules)

	req, _ := http.NewRequest("GET", "/rules", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestGetRules_WithScenario(t *testing.T) {
	cfg, _ := config.NewTestConfig()
	router := setupTestRouter(cfg)
	logger := obs.NewMemoryLogger(100)
	handler := NewHandler(cfg, logger)

	router.GET("/rules", handler.GetRules)

	// Create a test rule
	testRule := &typ.Rule{
		UUID:         uuid.New().String(),
		RequestModel: "gpt-4",
		Scenario:     "test-scenario",
		Active:       true,
	}
	cfg.AddOrUpdateRule(testRule)

	req, _ := http.NewRequest("GET", "/rules?scenario=test-scenario", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Check response contains success
	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
}

func TestGetRules_NilConfig(t *testing.T) {
	router := setupTestRouter(nil)
	logger := obs.NewMemoryLogger(100)
	handler := NewHandler(nil, logger)

	router.GET("/rules", handler.GetRules)

	req, _ := http.NewRequest("GET", "/rules?scenario=test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestGetRule_Success(t *testing.T) {
	cfg, _ := config.NewTestConfig()
	router := setupTestRouter(cfg)
	logger := obs.NewMemoryLogger(100)
	handler := NewHandler(cfg, logger)

	router.GET("/rules/:uuid", handler.GetRule)

	// Create a test rule
	testUUID := uuid.New().String()
	testRule := &typ.Rule{
		UUID:         testUUID,
		RequestModel: "gpt-4",
		Scenario:     "test-scenario",
		Active:       true,
	}
	cfg.AddOrUpdateRule(testRule)

	req, _ := http.NewRequest("GET", "/rules/"+testUUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
	assert.Contains(t, body, testUUID)
}

func TestGetRule_NotFound(t *testing.T) {
	cfg, _ := config.NewTestConfig()
	router := setupTestRouter(cfg)
	logger := obs.NewMemoryLogger(100)
	handler := NewHandler(cfg, logger)

	router.GET("/rules/:uuid", handler.GetRule)

	req, _ := http.NewRequest("GET", "/rules/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Rule not found")
}

func TestGetRule_EmptyUUID(t *testing.T) {
	cfg, _ := config.NewTestConfig()
	router := setupTestRouter(cfg)
	logger := obs.NewMemoryLogger(100)
	handler := NewHandler(cfg, logger)

	router.GET("/rules/:uuid", handler.GetRule)

	req, _ := http.NewRequest("GET", "/rules/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Gin will return 404 for empty path parameter
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestGetRule_NilConfig(t *testing.T) {
	router := setupTestRouter(nil)
	logger := obs.NewMemoryLogger(100)
	handler := NewHandler(nil, logger)

	router.GET("/rules/:uuid", handler.GetRule)

	testUUID := uuid.New().String()
	req, _ := http.NewRequest("GET", "/rules/"+testUUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

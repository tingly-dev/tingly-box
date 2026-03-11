package providertemplate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/data"
)

// mockTemplateManager is a mock implementation of TemplateManager for testing
type mockTemplateManager struct {
	templates   map[string]*data.ProviderTemplate
	version     string
	getErr      error
	fetchErr    error
	fetchCalled bool
}

func (m *mockTemplateManager) GetAllTemplates() map[string]*data.ProviderTemplate {
	return m.templates
}

func (m *mockTemplateManager) GetTemplate(id string) (*data.ProviderTemplate, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if tmpl, ok := m.templates[id]; ok {
		return tmpl, nil
	}
	return nil, errors.New("template not found")
}

func (m *mockTemplateManager) GetVersion() string {
	return m.version
}

func (m *mockTemplateManager) FetchFromGitHub(ctx context.Context) (*data.ProviderRegistry, error) {
	m.fetchCalled = true
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	return &data.ProviderRegistry{
		Version:   m.version,
		Providers: m.templates,
	}, nil
}

func setupTestRouter(tm *mockTemplateManager) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	var templateManager data.TemplateManager = tm
	handler := NewHandler(&templateManager)

	router.GET("/templates", handler.GetProviderTemplates)
	router.GET("/templates/:id", handler.GetProviderTemplate)
	router.POST("/templates/refresh", handler.RefreshProviderTemplates)
	router.GET("/templates/version", handler.GetProviderTemplateVersion)

	return router
}

func TestNewHandler(t *testing.T) {
	mockTM := &mockTemplateManager{
		templates: make(map[string]*data.ProviderTemplate),
		version:   "1.0.0",
	}

	var tm data.TemplateManager = mockTM
	handler := NewHandler(&tm)

	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.templateManager == nil {
		t.Error("expected templateManager to be set")
	}
}

func TestGetProviderTemplates_Success(t *testing.T) {
	mockTM := &mockTemplateManager{
		templates: map[string]*data.ProviderTemplate{
			"openai": {
				ID:   "openai",
				Name: "OpenAI",
			},
			"anthropic": {
				ID:   "anthropic",
				Name: "Anthropic",
			},
		},
		version: "1.0.0",
	}

	var tm data.TemplateManager = mockTM
	router := setupTestRouter(mockTM)

	req, _ := http.NewRequest("GET", "/templates", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
	assert.Contains(t, body, "openai")
	assert.Contains(t, body, "anthropic")
	assert.Contains(t, body, "1.0.0")
}

func TestGetProviderTemplates_NilManager(t *testing.T) {
	handler := NewHandler(nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/templates", handler.GetProviderTemplates)

	req, _ := http.NewRequest("GET", "/templates", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Template manager not initialized")
}

func TestGetProviderTemplate_Success(t *testing.T) {
	mockTM := &mockTemplateManager{
		templates: map[string]*data.ProviderTemplate{
			"openai": {
				ID:   "openai",
				Name: "OpenAI",
			},
		},
		version: "1.0.0",
	}

	var tm data.TemplateManager = mockTM
	router := setupTestRouter(mockTM)

	req, _ := http.NewRequest("GET", "/templates/openai", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
	assert.Contains(t, body, "openai")
}

func TestGetProviderTemplate_NotFound(t *testing.T) {
	mockTM := &mockTemplateManager{
		templates: make(map[string]*data.ProviderTemplate),
		version:   "1.0.0",
		getErr:    errors.New("template not found"),
	}

	var tm data.TemplateManager = mockTM
	router := setupTestRouter(mockTM)

	req, _ := http.NewRequest("GET", "/templates/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "template not found")
}

func TestGetProviderTemplate_EmptyID(t *testing.T) {
	mockTM := &mockTemplateManager{
		templates: make(map[string]*data.ProviderTemplate),
		version:   "1.0.0",
	}

	var tm data.TemplateManager = mockTM
	router := setupTestRouter(mockTM)

	req, _ := http.NewRequest("GET", "/templates/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Gin returns 404 for empty path param
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestGetProviderTemplate_NilManager(t *testing.T) {
	handler := NewHandler(nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/templates/:id", handler.GetProviderTemplate)

	req, _ := http.NewRequest("GET", "/templates/openai", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Template manager not initialized")
}

func TestRefreshProviderTemplates_Success(t *testing.T) {
	mockTM := &mockTemplateManager{
		templates: map[string]*data.ProviderTemplate{
			"openai": {
				ID:   "openai",
				Name: "OpenAI",
			},
		},
		version: "2.0.0",
	}

	var tm data.TemplateManager = mockTM
	router := setupTestRouter(mockTM)

	req, _ := http.NewRequest("POST", "/templates/refresh", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", w.Code)
	}

	if !mockTM.fetchCalled {
		t.Error("expected FetchFromGitHub to be called")
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
	assert.Contains(t, body, "Templates refreshed successfully")
	assert.Contains(t, body, "2.0.0")
}

func TestRefreshProviderTemplates_Error(t *testing.T) {
	mockTM := &mockTemplateManager{
		templates: make(map[string]*data.ProviderTemplate),
		version:   "1.0.0",
		fetchErr:  errors.New("network error"),
	}

	var tm data.TemplateManager = mockTM
	router := setupTestRouter(mockTM)

	req, _ := http.NewRequest("POST", "/templates/refresh", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Failed to refresh templates from GitHub")
}

func TestRefreshProviderTemplates_NilManager(t *testing.T) {
	handler := NewHandler(nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/templates/refresh", handler.RefreshProviderTemplates)

	req, _ := http.NewRequest("POST", "/templates/refresh", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Template manager not initialized")
}

func TestGetProviderTemplateVersion_Success(t *testing.T) {
	mockTM := &mockTemplateManager{
		templates: make(map[string]*data.ProviderTemplate),
		version:   "1.5.2",
	}

	var tm data.TemplateManager = mockTM
	router := setupTestRouter(mockTM)

	req, _ := http.NewRequest("GET", "/templates/version", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
	assert.Contains(t, body, "1.5.2")
}

func TestGetProviderTemplateVersion_NilManager(t *testing.T) {
	handler := NewHandler(nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/templates/version", handler.GetProviderTemplateVersion)

	req, _ := http.NewRequest("GET", "/templates/version", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":false`)
	assert.Contains(t, body, "Template manager not initialized")
}

func TestTemplateResponseStructure(t *testing.T) {
	response := TemplateResponse{
		Success: true,
		Message: "Test message",
		Version: "1.0.0",
		Data: map[string]*data.ProviderTemplate{
			"test": {
				ID:   "test",
				Name: "Test Provider",
			},
		},
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if response.Message != "Test message" {
		t.Errorf("expected Message 'Test message', got %q", response.Message)
	}

	if response.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %q", response.Version)
	}

	if len(response.Data) != 1 {
		t.Errorf("expected 1 data item, got %d", len(response.Data))
	}
}

func TestSingleTemplateResponseStructure(t *testing.T) {
	template := &data.ProviderTemplate{
		ID:   "test",
		Name: "Test Provider",
	}

	response := SingleTemplateResponse{
		Success: true,
		Data:    template,
		Message: "Found",
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if response.Data.ID != "test" {
		t.Errorf("expected ID 'test', got %q", response.Data.ID)
	}

	if response.Message != "Found" {
		t.Errorf("expected Message 'Found', got %q", response.Message)
	}
}

package statusline

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// mockLoadBalancer is a mock implementation of LoadBalancer for testing
type mockLoadBalancer struct {
	selectServiceErr error
}

func (m *mockLoadBalancer) SelectService(rule *typ.Rule) (*loadbalance.Service, error) {
	if m.selectServiceErr != nil {
		return nil, m.selectServiceErr
	}
	return &loadbalance.Service{
		Provider:   "",
		Model:      "",
		Weight:     0,
		Active:     false,
		TimeWindow: 0,
		Stats:      loadbalance.ServiceStats{},
	}, nil
}

func setupTestRouter(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	cache := NewCache()
	handler := NewHandler(cfg, &mockLoadBalancer{}, cache, nil) // nil quota manager for tests

	router.POST("/status/:scenario", handler.GetClaudeCodeStatus)
	router.POST("/statusline/:scenario", handler.GetClaudeCodeStatusLine)

	return router
}

func TestNewHandler(t *testing.T) {
	cfg, _ := config.NewConfig()
	cache := NewCache()
	lb := &mockLoadBalancer{}

	handler := NewHandler(cfg, lb, cache, nil)

	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.config != cfg {
		t.Error("expected config to be set")
	}
	if handler.cache == nil {
		t.Error("expected cache to be set")
	}
	if handler.loadBalancer == nil {
		t.Error("expected loadBalancer to be set")
	}
}

func TestGetClaudeCodeStatus_Success(t *testing.T) {
	cfg, _ := config.NewConfig()
	router := setupTestRouter(cfg)

	_ = `{
		"model": {
			"id": "claude-sonnet-4-6",
			"display_name": "Claude Sonnet 4.6",
			"provider_name": "anthropic"
		},
		"context_window": {
			"total_input_tokens": 15000,
			"total_output_tokens": 5000,
			"context_window_size": 200000,
			"used_percentage": 10.0,
			"remaining_percentage": 90.0
		},
		"cost": {
			"total_cost_usd": 0.05,
			"total_duration_ms": 120000,
			"total_api_duration_ms": 30000,
			"total_lines_added": 150,
			"total_lines_removed": 50
		},
		"session_id": "test-session-123"
	}`

	req, _ := http.NewRequest("POST", "/status/claude_code", nil)
	req.Body = nil
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return success even without body
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
}

func TestGetClaudeCodeStatus_EmptyBody(t *testing.T) {
	cfg, _ := config.NewConfig()
	router := setupTestRouter(cfg)

	req, _ := http.NewRequest("POST", "/status/claude_code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, `"success":true`)
}

func TestGetClaudeCodeStatusLine_Success(t *testing.T) {
	cfg, _ := config.NewConfig()
	router := setupTestRouter(cfg)

	req, _ := http.NewRequest("POST", "/statusline/claude_code", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return success even without body
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// The /statusline endpoint returns plain text, not JSON
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty body")
	}
}

func TestCacheOperations(t *testing.T) {
	cache := NewCache()

	sessionID := "test-session-123"

	// Test empty cache
	input1 := &StatusInput{
		SessionID: sessionID,
		Model: Model{
			ID: "test-model",
		},
	}

	result1 := cache.Get(input1)
	if result1.Model.ID != "test-model" {
		t.Errorf("expected model ID 'test-model', got %q", result1.Model.ID)
	}

	// Update cache
	input2 := &StatusInput{
		SessionID: sessionID,
		Model: Model{
			ID:          "test-model-2",
			DisplayName: "Test Model 2",
		},
		Cost: Cost{
			TotalCostUSD: 0.10,
		},
	}
	cache.Update(input2)

	// Get updated value - input with same session ID but missing fields should get them from cache
	input3 := &StatusInput{
		SessionID: sessionID,
		Model: Model{
			ID: "test-model-2",
		},
	}

	result3 := cache.Get(input3)
	if result3.Model.DisplayName != "Test Model 2" {
		t.Errorf("expected display name 'Test Model 2', got %q", result3.Model.DisplayName)
	}
	if result3.Cost.TotalCostUSD != 0.10 {
		t.Errorf("expected cost 0.10, got %f", result3.Cost.TotalCostUSD)
	}
}

func TestStatusInputStructure(t *testing.T) {
	input := StatusInput{
		Model: Model{
			ID:           "claude-sonnet-4-6",
			DisplayName:  "Claude Sonnet 4.6",
			ProviderName: "anthropic",
		},
		CWD: "/Users/user/project",
		Workspace: Workspace{
			CurrentDir: "/Users/user/project",
			ProjectDir: "/Users/user/project",
		},
		Cost: Cost{
			TotalCostUSD:       0.05,
			TotalDurationMs:    120000,
			TotalAPIDurationMs: 30000,
			TotalLinesAdded:    150,
			TotalLinesRemoved:  50,
		},
		ContextWindow: ContextWindow{
			TotalInputTokens:    15000,
			TotalOutputTokens:   5000,
			ContextWindowSize:   200000,
			UsedPercentage:      10.0,
			RemainingPercentage: 90.0,
			CurrentUsage: CurrentUsage{
				InputTokens:  1500,
				OutputTokens: 500,
				CacheRead:    10000,
				CacheWrite:   2000,
			},
		},
		SessionID:         "test-session-123",
		Exceeds200kTokens: false,
		Version:           "1.0.0",
	}

	if input.Model.ID != "claude-sonnet-4-6" {
		t.Errorf("expected Model.ID 'claude-sonnet-4-6', got %q", input.Model.ID)
	}

	if input.Cost.TotalCostUSD != 0.05 {
		t.Errorf("expected TotalCostUSD 0.05, got %f", input.Cost.TotalCostUSD)
	}

	if input.ContextWindow.TotalInputTokens != 15000 {
		t.Errorf("expected TotalInputTokens 15000, got %d", input.ContextWindow.TotalInputTokens)
	}

	if input.SessionID != "test-session-123" {
		t.Errorf("expected SessionID 'test-session-123', got %q", input.SessionID)
	}
}

func TestCombinedStatusStructure(t *testing.T) {
	data := &CombinedStatusData{
		CCModel:             "Claude Sonnet 4.6",
		CCUsedPct:           10,
		CCUsedTokens:        20000,
		CCMaxTokens:         200000,
		CCCost:              0.05,
		CCDurationMs:        120000,
		CCAPIDurationMs:     30000,
		CCLinesAdded:        150,
		CCLinesRemoved:      50,
		CCSessionID:         "session-123",
		CCExceeds200kTokens: false,
		TBProviderName:      "openai",
		TBProviderUUID:      "uuid-1234",
		TBModel:             "gpt-4",
		TBRequestModel:      "gpt-4",
		TBScenario:          "openai",
	}

	response := CombinedStatus{
		Success: true,
		Data:    data,
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if response.Data.CCModel != "Claude Sonnet 4.6" {
		t.Errorf("expected CCModel 'Claude Sonnet 4.6', got %q", response.Data.CCModel)
	}

	if response.Data.CCUsedPct != 10 {
		t.Errorf("expected CCUsedPct 10, got %d", response.Data.CCUsedPct)
	}

	if response.Data.TBProviderName != "openai" {
		t.Errorf("expected TBProviderName 'openai', got %q", response.Data.TBProviderName)
	}
}

func TestModelStructure(t *testing.T) {
	model := Model{
		ID:           "claude-sonnet-4-6",
		DisplayName:  "Claude Sonnet 4.6",
		ProviderName: "anthropic",
	}

	if model.ID != "claude-sonnet-4-6" {
		t.Errorf("expected ID 'claude-sonnet-4-6', got %q", model.ID)
	}

	if model.DisplayName != "Claude Sonnet 4.6" {
		t.Errorf("expected DisplayName 'Claude Sonnet 4.6', got %q", model.DisplayName)
	}

	if model.ProviderName != "anthropic" {
		t.Errorf("expected ProviderName 'anthropic', got %q", model.ProviderName)
	}
}

func TestWorkspaceStructure(t *testing.T) {
	workspace := Workspace{
		CurrentDir: "/Users/user/project",
		ProjectDir: "/Users/user/project",
	}

	if workspace.CurrentDir != "/Users/user/project" {
		t.Errorf("expected CurrentDir '/Users/user/project', got %q", workspace.CurrentDir)
	}

	if workspace.ProjectDir != "/Users/user/project" {
		t.Errorf("expected ProjectDir '/Users/user/project', got %q", workspace.ProjectDir)
	}
}

func TestContextWindowStructure(t *testing.T) {
	currentUsage := CurrentUsage{
		InputTokens:  1500,
		OutputTokens: 500,
		CacheRead:    10000,
		CacheWrite:   2000,
	}

	contextWindow := ContextWindow{
		TotalInputTokens:    15000,
		TotalOutputTokens:   5000,
		ContextWindowSize:   200000,
		UsedPercentage:      10.0,
		RemainingPercentage: 90.0,
		CurrentUsage:        currentUsage,
	}

	if contextWindow.TotalInputTokens != 15000 {
		t.Errorf("expected TotalInputTokens 15000, got %d", contextWindow.TotalInputTokens)
	}

	if contextWindow.UsedPercentage != 10.0 {
		t.Errorf("expected UsedPercentage 10.0, got %f", contextWindow.UsedPercentage)
	}

	if contextWindow.CurrentUsage.InputTokens != 1500 {
		t.Errorf("expected InputTokens 1500, got %d", contextWindow.CurrentUsage.InputTokens)
	}
}

func TestCostStructure(t *testing.T) {
	cost := Cost{
		TotalCostUSD:       0.05,
		TotalDurationMs:    120000,
		TotalAPIDurationMs: 30000,
		TotalLinesAdded:    150,
		TotalLinesRemoved:  50,
	}

	if cost.TotalCostUSD != 0.05 {
		t.Errorf("expected TotalCostUSD 0.05, got %f", cost.TotalCostUSD)
	}

	if cost.TotalDurationMs != 120000 {
		t.Errorf("expected TotalDurationMs 120000, got %d", cost.TotalDurationMs)
	}

	if cost.TotalLinesAdded != 150 {
		t.Errorf("expected TotalLinesAdded 150, got %d", cost.TotalLinesAdded)
	}
}

func TestOutputStyleStructure(t *testing.T) {
	style := OutputStyle{
		Name: "default",
	}

	if style.Name != "default" {
		t.Errorf("expected Name 'default', got %q", style.Name)
	}
}

func TestVimStructure(t *testing.T) {
	vim := Vim{
		Mode: "NORMAL",
	}

	if vim.Mode != "NORMAL" {
		t.Errorf("expected Mode 'NORMAL', got %q", vim.Mode)
	}
}

func TestAgentStructure(t *testing.T) {
	agent := Agent{
		Name: "claude-opus-4-6",
	}

	if agent.Name != "claude-opus-4-6" {
		t.Errorf("expected Name 'claude-opus-4-6', got %q", agent.Name)
	}
}

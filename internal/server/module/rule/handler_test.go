package rule

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/dataimport"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
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

func registerTestRuleScenario(t *testing.T, scenario typ.RuleScenario) {
	t.Helper()
	err := typ.RegisterScenario(typ.ScenarioDescriptor{
		ID:                 scenario,
		SupportedTransport: []typ.ScenarioTransport{typ.TransportOpenAI},
		AllowRuleBinding:   true,
		AllowDirectPathUse: false,
	})
	if err != nil {
		t.Fatalf("RegisterScenario(%q) error = %v", scenario, err)
	}
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

func TestCreateRule_Success(t *testing.T) {
	registerTestRuleScenario(t, typ.RuleScenario("test-scenario"))

	cfg := NewConfig(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(cfg, logger)

	router.POST("/rules", handler.CreateRule)

	rule := typ.Rule{
		RequestModel:  "gpt-4",
		ResponseModel: "gpt-4",
		Scenario:      "test-scenario",
		Description:   "Test rule",
		Active:        true,
	}
	body, _ := json.Marshal(rule)
	req, _ := http.NewRequest("POST", "/rules", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":true`)
	assert.Contains(t, bodyResp, `"uuid"`)
}

func TestCreateRule_NoScenario(t *testing.T) {
	cfg := NewConfig(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger, _ := obs.NewMemoryLogger()
	handler := NewHandler(cfg, logger)

	router.POST("/rules", handler.CreateRule)

	rule := typ.Rule{
		RequestModel:  "gpt-4",
		ResponseModel: "gpt-4",
		// Missing Scenario
		Description: "Test rule",
		Active:      true,
	}
	body, _ := json.Marshal(rule)
	req, _ := http.NewRequest("POST", "/rules", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":false`)
}

func TestUpdateRule_Success(t *testing.T) {
	registerTestRuleScenario(t, typ.RuleScenario("test-scenario"))

	cfg := NewConfig(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger, _ := obs.NewMemoryLogger()
	handler := NewHandler(cfg, logger)

	router.PUT("/rules/:uuid", handler.UpdateRule)

	// First create a rule
	testUUID := uuid.New().String()
	originalRule := typ.Rule{
		UUID:          testUUID,
		RequestModel:  "gpt-4",
		ResponseModel: "gpt-4",
		Scenario:      "test-scenario",
		Description:   "Original description",
		Active:        true,
	}
	if err := cfg.AddRule(originalRule); err != nil {
		t.Fatalf("Failed to add test rule: %v", err)
	}

	// Now update it
	updatedRule := typ.Rule{
		RequestModel:  "gpt-4",
		ResponseModel: "gpt-4",
		Scenario:      "test-scenario",
		Description:   "Updated description",
		Active:        false,
	}
	body, _ := json.Marshal(updatedRule)
	req, _ := http.NewRequest("PUT", "/rules/"+testUUID, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":true`)

	// Verify the update
	retrievedRule := cfg.GetRuleByUUID(testUUID)
	if retrievedRule == nil {
		t.Fatal("Rule not found after update")
	}
	if retrievedRule.Description != "Updated description" {
		t.Errorf("Expected description 'Updated description', got '%s'", retrievedRule.Description)
	}
}

func TestDeleteRule_Success(t *testing.T) {
	cfg := setupTestConfig(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger, _ := obs.NewMemoryLogger()
	handler := NewHandler(cfg, logger)

	router.DELETE("/rules/:uuid", handler.DeleteRule)

	// First create a rule
	testUUID := uuid.New().String()
	testRule := typ.Rule{
		UUID:          testUUID,
		RequestModel:  "gpt-4",
		ResponseModel: "gpt-4",
		Scenario:      "test-scenario",
		Description:   "Test rule",
		Active:        true,
	}
	if err := cfg.AddRule(testRule); err != nil {
		t.Fatalf("Failed to add test rule: %v", err)
	}

	// Now delete it
	req, _ := http.NewRequest("DELETE", "/rules/"+testUUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":true`)

	// Verify the deletion
	retrievedRule := cfg.GetRuleByUUID(testUUID)
	if retrievedRule != nil {
		t.Error("Rule should have been deleted")
	}
}

// TestImportRule_JSONL tests importing a rule from JSONL format
func TestImportRule_JSONL(t *testing.T) {
	cfg := setupTestConfig(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger, _ := obs.NewMemoryLogger()
	handler := NewHandler(cfg, logger)

	router.POST("/rules/import", handler.ImportRule)

	// Create a minimal JSONL export with proper lb_tactic structure
	jsonlData := `{"type":"metadata","version":"1.0","exported_at":"2024-01-01T00:00:00Z"}
{"type":"provider","uuid":"prov-1","name":"TestProvider","api_base":"https://api.test.com","api_style":"openai","auth_type":"api_key","token":"sk-test","enabled":true,"timeout":30}
{"type":"rule","uuid":"rule-1","scenario":"general","request_model":"gpt-4","response_model":"gpt-4","description":"Test rule","services":[{"provider":"prov-1","model":"gpt-4","weight":100}],"lb_tactic":{"type":"round_robin","params":{}},"active":true,"smart_enabled":false,"smart_routing":[]}`

	importReq := ImportRuleRequest{
		Data:               jsonlData,
		OnProviderConflict: "use",
		OnRuleConflict:     "new",
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/rules/import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":true`)
	assert.Contains(t, bodyResp, `"rule_created":true`)

	// Verify the rule was imported
	rules := cfg.GetRequestConfigs()
	if len(rules) == 0 {
		t.Error("Expected at least one rule to be imported")
	}
}

// TestImportRule_Base64 tests importing a rule from Base64 format
func TestImportRule_Base64(t *testing.T) {
	cfg := setupTestConfig(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger, _ := obs.NewMemoryLogger()
	handler := NewHandler(cfg, logger)

	router.POST("/rules/import", handler.ImportRule)

	// Create a minimal Base64 export
	// JSONL: {"type":"metadata","version":"1.0"}\n{"type":"rule","uuid":"rule-1","scenario":"general","request_model":"gpt-4"}
	jsonlData := `{"type":"metadata","version":"1.0","exported_at":"2024-01-01T00:00:00Z"}
{"type":"provider","uuid":"prov-1","name":"TestProvider","api_base":"https://api.test.com","api_style":"openai","auth_type":"api_key","token":"sk-test","enabled":true}
{"type":"rule","uuid":"rule-1","scenario":"general","request_model":"gpt-4","response_model":"gpt-4","description":"Test","services":[{"provider":"prov-1","model":"gpt-4"}],"lb_tactic":{"type":"round_robin","params":{}},"active":true,"smart_enabled":false,"smart_routing":[]}`

	// Encode the JSONL data to Base64
	base64Payload := base64.StdEncoding.EncodeToString([]byte(jsonlData))
	base64Data := dataimport.Base64Prefix + ":1.0:" + base64Payload

	importReq := ImportRuleRequest{
		Data:               base64Data,
		OnProviderConflict: "use",
		OnRuleConflict:     "new",
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/rules/import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":true`)
}

// TestImportRule_ProviderConflictUse tests using existing provider on conflict
func TestImportRule_ProviderConflictUse(t *testing.T) {
	cfg := NewConfig(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger, _ := obs.NewMemoryLogger()
	handler := NewHandler(cfg, logger)

	router.POST("/rules/import", handler.ImportRule)

	// First create an existing provider with the same name
	existingProvider := &typ.Provider{
		UUID:     uuid.New().String(),
		Name:     "TestProvider",
		APIBase:  "https://api.existing.com",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "sk-existing",
		Enabled:  true,
	}
	cfg.AddProvider(existingProvider)

	// Import a rule that references a provider with the same name
	jsonlData := `{"type":"metadata","version":"1.0","exported_at":"2024-01-01T00:00:00Z"}
{"type":"provider","uuid":"prov-1","name":"TestProvider","api_base":"https://api.test.com","api_style":"openai","auth_type":"api_key","token":"sk-test","enabled":true}
{"type":"rule","uuid":"rule-1","scenario":"general","request_model":"gpt-4","response_model":"gpt-4","description":"Test","services":[{"provider":"prov-1","model":"gpt-4"}],"lb_tactic":{"type":"round_robin","params":{}},"active":true,"smart_enabled":false,"smart_routing":[]}`

	importReq := ImportRuleRequest{
		Data:               jsonlData,
		OnProviderConflict: "use", // Use existing provider
		OnRuleConflict:     "new",
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/rules/import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":true`)

	// Parse response to check provider info
	var resp ImportRuleResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have 0 providers created (used existing), 1 used
	if resp.Data.ProvidersCreated != 0 {
		t.Errorf("Expected 0 providers created, got %d", resp.Data.ProvidersCreated)
	}
	if resp.Data.ProvidersUsed != 1 {
		t.Errorf("Expected 1 provider used, got %d", resp.Data.ProvidersUsed)
	}
}

// TestImportRule_RuleConflictSkip tests skipping rule on conflict
func TestImportRule_RuleConflictSkip(t *testing.T) {
	cfg := setupTestConfig(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger, _ := obs.NewMemoryLogger()
	handler := NewHandler(cfg, logger)

	router.POST("/rules/import", handler.ImportRule)

	// First create an existing rule
	existingRule := typ.Rule{
		UUID:          uuid.New().String(),
		RequestModel:  "gpt-4",
		ResponseModel: "gpt-4",
		Scenario:      "general",
		Description:   "Existing rule",
		Services: []*loadbalance.Service{
			{
				Provider: uuid.New().String(),
				Model:    "gpt-4",
				Weight:   100,
			},
		},
		Active: true,
	}
	if err := cfg.AddRule(existingRule); err != nil {
		t.Fatalf("Failed to add existing rule: %v", err)
	}

	// Try to import a rule with the same request_model and scenario
	jsonlData := `{"type":"metadata","version":"1.0","exported_at":"2024-01-01T00:00:00Z"}
{"type":"provider","uuid":"prov-1","name":"TestProvider","api_base":"https://api.test.com","api_style":"openai","auth_type":"api_key","token":"sk-test","enabled":true}
{"type":"rule","uuid":"rule-1","scenario":"general","request_model":"gpt-4","response_model":"gpt-4","description":"New rule","services":[{"provider":"prov-1","model":"gpt-4"}],"lb_tactic":{"type":"round_robin","params":{}},"active":true,"smart_enabled":false,"smart_routing":[]}`

	importReq := ImportRuleRequest{
		Data:               jsonlData,
		OnProviderConflict: "use",
		OnRuleConflict:     "skip", // Skip on conflict
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/rules/import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":true`)

	// Parse response to check rule info
	var resp ImportRuleResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Rule should not be created (skipped)
	if resp.Data.RuleCreated {
		t.Error("Expected rule not to be created (should skip on conflict)")
	}
}

// TestImportRule_InvalidData tests importing with invalid data
func TestImportRule_InvalidData(t *testing.T) {
	cfg := setupTestConfig(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger, _ := obs.NewMemoryLogger()
	handler := NewHandler(cfg, logger)

	router.POST("/rules/import", handler.ImportRule)

	importReq := ImportRuleRequest{
		Data:               "invalid data",
		OnProviderConflict: "use",
		OnRuleConflict:     "new",
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/rules/import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":false`)
}

// TestImportRule_ProviderUUIDConflict tests real UUID conflict scenario
func TestImportRule_ProviderUUIDConflict(t *testing.T) {
	cfg := setupTestConfig(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger, _ := obs.NewMemoryLogger()
	handler := NewHandler(cfg, logger)

	router.POST("/rules/import", handler.ImportRule)

	// First create an existing provider with the same UUID (simulating re-import)
	existingProvider := &typ.Provider{
		UUID:     "prov-1", // Same UUID as in the export
		Name:     "ExistingProvider",
		APIBase:  "https://api.existing.com",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "sk-existing",
		Enabled:  true,
	}
	cfg.AddProvider(existingProvider)

	// Import a rule that references a provider with the same UUID
	jsonlData := `{"type":"metadata","version":"1.0","exported_at":"2024-01-01T00:00:00Z"}
{"type":"provider","uuid":"prov-1","name":"TestProvider","api_base":"https://api.test.com","api_style":"openai","auth_type":"api_key","token":"sk-test","enabled":true}
{"type":"rule","uuid":"rule-1","scenario":"general","request_model":"gpt-4","response_model":"gpt-4","description":"Test","services":[{"provider":"prov-1","model":"gpt-4"}],"lb_tactic":{"type":"round_robin","params":{}},"active":true,"smart_enabled":false,"smart_routing":[]}`

	importReq := ImportRuleRequest{
		Data:               jsonlData,
		OnProviderConflict: "use", // Use existing provider with same UUID
		OnRuleConflict:     "new",
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/rules/import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	bodyResp := w.Body.String()
	assert.Contains(t, bodyResp, `"success":true`)

	// Parse response to check provider info
	var resp ImportRuleResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should have 0 providers created (used existing), 1 used
	if resp.Data.ProvidersCreated != 0 {
		t.Errorf("Expected 0 providers created, got %d", resp.Data.ProvidersCreated)
	}
	if resp.Data.ProvidersUsed != 1 {
		t.Errorf("Expected 1 provider used, got %d", resp.Data.ProvidersUsed)
	}

	// Verify the used provider is the existing one
	found := false
	for _, p := range resp.Data.Providers {
		if p.UUID == "prov-1" && p.Name == "ExistingProvider" && p.Action == "used" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find existing provider being used")
	}
}

// TestImportRule_MissingData tests importing with missing data field
func TestImportRule_MissingData(t *testing.T) {
	cfg := setupTestConfig(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	logger, _ := obs.NewMemoryLogger()
	handler := NewHandler(cfg, logger)

	router.POST("/rules/import", handler.ImportRule)

	importReq := map[string]string{
		"on_provider_conflict": "use",
		"on_rule_conflict":     "new",
		// Missing "data" field
	}
	body, _ := json.Marshal(importReq)
	req, _ := http.NewRequest("POST", "/rules/import", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

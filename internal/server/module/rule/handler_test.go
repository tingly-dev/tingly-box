package rule

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestGetRules_IncludesGeneralRulesForProtocolScenario(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{
		Rules: []typ.Rule{
			{
				UUID:         "general-rule",
				RequestModel: "shared-model",
				Scenario:     typ.ScenarioGeneral,
				Active:       true,
			},
			{
				UUID:         "openai-rule",
				RequestModel: "openai-only",
				Scenario:     typ.ScenarioOpenAI,
				Active:       true,
			},
			{
				UUID:         "claude-code-rule",
				RequestModel: "cc-only",
				Scenario:     typ.ScenarioClaudeCode,
				Active:       true,
			},
		},
	})

	router := gin.New()
	router.GET("/rules", handler.GetRules)

	req := httptest.NewRequest(http.MethodGet, "/rules?scenario=openai", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Success bool       `json:"success"`
		Data    []typ.Rule `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 rules for openai path, got %#v", resp.Data)
	}

	seen := map[string]bool{}
	for _, rule := range resp.Data {
		seen[rule.UUID] = true
	}
	if !seen["general-rule"] || !seen["openai-rule"] {
		t.Fatalf("expected general and openai rules, got %#v", resp.Data)
	}
	if seen["claude-code-rule"] {
		t.Fatalf("did not expect claude_code rule in openai page, got %#v", resp.Data)
	}
}

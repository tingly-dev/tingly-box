package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestOpenAIListModelsPlacesExtensionsInDetail(t *testing.T) {
	s := newModelListTestServer(t, typ.ScenarioOpenAI)

	body := performModelListRequest(t, func(c *gin.Context) {
		s.openAIListModelsWithScenario(c, nil)
	})

	data := body["data"].([]interface{})
	if len(data) != 2 {
		t.Fatalf("expected 2 models, got %d", len(data))
	}

	// OAuth-backed model should sort before API-key-backed model, using detail.auth_type.
	first := data[0].(map[string]interface{})
	if first["id"] != "oauth-model" {
		t.Fatalf("expected oauth model first, got %v", first["id"])
	}

	assertAbsent(t, first, "description")
	assertAbsent(t, first, "context")
	assertAbsent(t, first, "max_output")
	assertAbsent(t, first, "max_tokens")
	assertAbsent(t, first, "max_completion_tokens")
	assertAbsent(t, first, "auth_type")
	assertAbsent(t, first, "capabilities")

	detail := first["detail"].(map[string]interface{})
	assertEqual(t, detail, "description", "OAuth model")
	assertEqual(t, detail, "context", float64(200000))
	assertEqual(t, detail, "max_tokens", float64(8192))
	assertEqual(t, detail, "max_completion_tokens", float64(8192))
	assertEqual(t, detail, "auth_type", string(typ.AuthTypeOAuth))
	assertStringSlice(t, detail["input_modalities"], []string{"text"})
	assertStringSlice(t, detail["output_modalities"], []string{"text"})
	assertAbsent(t, detail, "capabilities")
}

func TestAnthropicListModelsKeepsNativeFieldsAndUsesDetailForExtensions(t *testing.T) {
	s := newModelListTestServer(t, typ.ScenarioAnthropic)

	body := performModelListRequest(t, func(c *gin.Context) {
		s.anthropicListModelsWithScenario(c, nil)
	})

	data := body["data"].([]interface{})
	if len(data) != 2 {
		t.Fatalf("expected 2 models, got %d", len(data))
	}

	first := data[0].(map[string]interface{})
	if first["id"] != "oauth-model" {
		t.Fatalf("expected oauth model first, got %v", first["id"])
	}

	assertEqual(t, first, "max_input_tokens", float64(200000))
	assertEqual(t, first, "max_tokens", float64(8192))
	assertAbsent(t, first, "description")
	assertAbsent(t, first, "auth_type")
	assertAbsent(t, first, "capabilities")

	detail := first["detail"].(map[string]interface{})
	assertEqual(t, detail, "description", "OAuth model")
	assertEqual(t, detail, "context", float64(200000))
	assertEqual(t, detail, "max_tokens", float64(8192))
	assertEqual(t, detail, "max_completion_tokens", float64(8192))
	assertEqual(t, detail, "auth_type", string(typ.AuthTypeOAuth))
	assertStringSlice(t, detail["input_modalities"], []string{"text"})
	assertStringSlice(t, detail["output_modalities"], []string{"text"})
	assertAbsent(t, detail, "capabilities")
}

func newModelListTestServer(t *testing.T, scenario typ.RuleScenario) *Server {
	t.Helper()

	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()), config.WithDisableMigration())
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	providers := []*typ.Provider{
		{
			UUID:     "api-provider",
			Name:     "api-template",
			APIBase:  "https://api.example.test",
			AuthType: typ.AuthTypeAPIKey,
			Enabled:  true,
		},
		{
			UUID:     "oauth-provider",
			Name:     "oauth-template",
			APIBase:  "https://oauth.example.test",
			AuthType: typ.AuthTypeOAuth,
			Enabled:  true,
		},
	}
	for _, provider := range providers {
		if err := cfg.AddProvider(provider); err != nil {
			t.Fatalf("AddProvider(%s) error: %v", provider.UUID, err)
		}
	}

	cfg.Rules = []typ.Rule{
		{
			UUID:         "api-rule",
			Scenario:     scenario,
			RequestModel: "api-model",
			Services: []*loadbalance.Service{{
				Provider: "api-provider",
				Model:    "api-model",
				Active:   true,
			}},
			Active: true,
		},
		{
			UUID:         "oauth-rule",
			Scenario:     scenario,
			RequestModel: "oauth-model",
			Services: []*loadbalance.Service{{
				Provider: "oauth-provider",
				Model:    "oauth-model",
				Active:   true,
			}},
			Active: true,
		},
	}
	cfg.SetTemplateManager(newModelListTestTemplateManager(t))

	return &Server{config: cfg}
}

func newModelListTestTemplateManager(t *testing.T) *data.TemplateManager {
	t.Helper()

	templatePath := filepath.Join(t.TempDir(), "providers.json")
	registry := `{
  "_schema_version": 2,
  "version": "test",
  "providers": {
    "api-template": {
      "id": "api-template",
      "name": "api-template",
      "status": "active",
      "valid": true,
      "models": [
        {"id": "api-model", "description": "API model", "context": 128000, "max_tokens": 4096}
      ]
    },
    "oauth-template": {
      "id": "oauth-template",
      "name": "oauth-template",
      "status": "active",
      "valid": true,
      "models": [
        {"id": "oauth-model", "description": "OAuth model", "context": 200000, "max_tokens": 8192}
      ]
    }
  }
}`
	if err := os.WriteFile(templatePath, []byte(registry), 0o600); err != nil {
		t.Fatalf("write template registry: %v", err)
	}

	tm := data.NewTemplateManager("file://" + templatePath)
	if _, err := tm.FetchTemplates(t.Context()); err != nil {
		t.Fatalf("FetchTemplates error: %v", err)
	}
	return tm
}

func performModelListRequest(t *testing.T, handler func(*gin.Context)) map[string]interface{} {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	handler(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, w.Body.String())
	}
	return body
}

func assertAbsent(t *testing.T, obj map[string]interface{}, key string) {
	t.Helper()
	if _, ok := obj[key]; ok {
		t.Fatalf("expected %q to be absent in %#v", key, obj)
	}
}

func assertEqual(t *testing.T, obj map[string]interface{}, key string, want interface{}) {
	t.Helper()
	if got := obj[key]; got != want {
		t.Fatalf("%s = %#v, want %#v", key, got, want)
	}
}

func assertStringSlice(t *testing.T, got interface{}, want []string) {
	t.Helper()
	items, ok := got.([]interface{})
	if !ok {
		t.Fatalf("got %#v, want string slice", got)
	}
	if len(items) != len(want) {
		t.Fatalf("slice length = %d, want %d (%#v)", len(items), len(want), got)
	}
	for i, item := range items {
		if item != want[i] {
			t.Fatalf("slice[%d] = %#v, want %#v", i, item, want[i])
		}
	}
}

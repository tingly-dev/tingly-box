package plugin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// postJSON drives a gin handler with a JSON body and returns the recorder and
// the parsed response envelope.
func postJSON(t *testing.T, h gin.HandlerFunc, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	h(c)
	var parsed map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	return w, parsed
}

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	return NewHandler(cfg)
}

func TestRegisterPlugin_BindsRule(t *testing.T) {
	h := newTestHandler(t)

	w, resp := postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name:     "my-rag",
		Endpoint: "http://127.0.0.1:8765/v1",
		ModelID:  "plugin/my-rag",
		Scenario: string(typ.ScenarioExperiment),
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	data, _ := resp["data"].(map[string]any)
	if data["ready"] != true {
		t.Fatalf("expected ready=true, got %v (note=%v)", data["ready"], data["note"])
	}
	if data["model_id"] != "plugin/my-rag" {
		t.Fatalf("model_id = %v", data["model_id"])
	}
	providerUUID, _ := data["provider_uuid"].(string)
	if providerUUID == "" {
		t.Fatalf("expected a provider_uuid")
	}

	// The provider must be persisted and tagged as a plugin.
	prov, err := h.config.GetProviderByUUID(providerUUID)
	if err != nil {
		t.Fatalf("GetProviderByUUID: %v", err)
	}
	if !prov.IsPlugin() {
		t.Fatalf("provider is not tagged as plugin: %+v", prov)
	}

	// A rule must exist under the scenario whose single service is the plugin.
	var found bool
	for _, rule := range h.config.GetRequestConfigs() {
		if rule.GetScenario() == typ.ScenarioExperiment && rule.RequestModel == "plugin/my-rag" {
			found = true
			if len(rule.Services) != 1 || rule.Services[0].Provider != providerUUID {
				t.Fatalf("rule service does not point at plugin provider: %+v", rule.Services)
			}
		}
	}
	if !found {
		t.Fatalf("no rule bound for the plugin under experiment scenario")
	}
}

func TestRegisterPlugin_APIStyleDefaultsToOpenAI(t *testing.T) {
	h := newTestHandler(t)

	_, resp := postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name: "no-style", Endpoint: "http://127.0.0.1:8765/v1",
	})
	uuid := resp["data"].(map[string]any)["provider_uuid"].(string)
	prov, err := h.config.GetProviderByUUID(uuid)
	if err != nil {
		t.Fatalf("GetProviderByUUID: %v", err)
	}
	if prov.APIStyle != "openai" {
		t.Fatalf("expected default api_style openai, got %q", prov.APIStyle)
	}
}

func TestRegisterPlugin_APIStyleAnthropic(t *testing.T) {
	h := newTestHandler(t)

	_, resp := postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name: "anthropic-plug", Endpoint: "http://127.0.0.1:8765", APIStyle: "anthropic",
	})
	uuid := resp["data"].(map[string]any)["provider_uuid"].(string)
	prov, err := h.config.GetProviderByUUID(uuid)
	if err != nil {
		t.Fatalf("GetProviderByUUID: %v", err)
	}
	if prov.APIStyle != "anthropic" {
		t.Fatalf("expected api_style anthropic, got %q", prov.APIStyle)
	}
}

func TestRegisterPlugin_APIStyleRejectsUnknown(t *testing.T) {
	h := newTestHandler(t)

	w, resp := postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name: "bad-style", Endpoint: "http://127.0.0.1:8765", APIStyle: "gemini",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown api_style, got %d (%v)", w.Code, resp)
	}
}

func TestRegisterPlugin_ReregisterUpdatesAPIStyle(t *testing.T) {
	h := newTestHandler(t)

	postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name: "switches", Endpoint: "http://127.0.0.1:8765",
	})
	_, second := postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name: "switches", Endpoint: "http://127.0.0.1:8765", APIStyle: "anthropic",
	})
	uuid := second["data"].(map[string]any)["provider_uuid"].(string)
	prov, err := h.config.GetProviderByUUID(uuid)
	if err != nil {
		t.Fatalf("GetProviderByUUID: %v", err)
	}
	if prov.APIStyle != "anthropic" {
		t.Fatalf("expected re-register to update api_style to anthropic, got %q", prov.APIStyle)
	}
}

func TestRegisterPlugin_ProviderOnly(t *testing.T) {
	h := newTestHandler(t)

	_, resp := postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name:     "solo",
		Endpoint: "http://127.0.0.1:9000/v1",
	})
	data, _ := resp["data"].(map[string]any)
	if data["ready"] == true {
		t.Fatalf("expected ready=false when no scenario given")
	}
	// model id defaults to plugin/<name>
	if data["model_id"] != "plugin/solo" {
		t.Fatalf("model_id default = %v", data["model_id"])
	}
}

func TestRegisterPlugin_ReregisterUpdatesInPlace(t *testing.T) {
	h := newTestHandler(t)

	_, first := postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name: "my-rag", Endpoint: "http://127.0.0.1:8765/v1",
	})
	firstUUID := first["data"].(map[string]any)["provider_uuid"].(string)

	// Re-register (e.g. the plugin process restarted on a different port).
	_, second := postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name: "my-rag", Endpoint: "http://127.0.0.1:9999/v1",
	})
	secondUUID := second["data"].(map[string]any)["provider_uuid"].(string)

	if firstUUID != secondUUID {
		t.Fatalf("re-register should update the same provider, got %s then %s", firstUUID, secondUUID)
	}

	prov, err := h.config.GetProviderByUUID(firstUUID)
	if err != nil {
		t.Fatalf("GetProviderByUUID: %v", err)
	}
	if prov.APIBase != "http://127.0.0.1:9999/v1" {
		t.Fatalf("expected endpoint to be updated in place, got %s", prov.APIBase)
	}

	// Exactly one provider named my-rag — no duplicate created.
	count := 0
	for _, p := range h.config.ListProviders() {
		if p.Name == "my-rag" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("re-register must not duplicate the provider, got %d", count)
	}
}

func TestRegisterPlugin_ReregisterIsIdempotentForRule(t *testing.T) {
	h := newTestHandler(t)
	postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name: "p", Endpoint: "http://a/v1", Scenario: string(typ.ScenarioExperiment),
	})
	postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name: "p", Endpoint: "http://b/v1", Scenario: string(typ.ScenarioExperiment),
	})
	count := 0
	for _, rule := range h.config.GetRequestConfigs() {
		if rule.RequestModel == "plugin/p" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("re-register must not duplicate the rule, got %d", count)
	}
}

func TestListPlugins_FiltersPluginTag(t *testing.T) {
	h := newTestHandler(t)
	// a normal provider
	if err := h.config.AddProvider(&typ.Provider{
		Name: "real", APIBase: "https://api.example.com/v1", APIStyle: "openai", Enabled: true,
	}); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	postJSON(t, h.RegisterPlugin, RegisterPluginRequest{
		Name: "plug", Endpoint: "http://127.0.0.1:8765/v1", ModelID: "plugin/plug",
		Scenario: string(typ.ScenarioExperiment),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	h.ListPlugins(c)

	var resp PluginsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Name != "plug" {
		t.Fatalf("expected exactly the plugin provider, got %+v", resp.Data)
	}
	if resp.Data[0].ModelID != "plugin/plug" {
		t.Fatalf("expected model id derived from the bound rule, got %q", resp.Data[0].ModelID)
	}
}

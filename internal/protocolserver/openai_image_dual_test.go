package protocolserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestImageForwarding_DualProviderResolvesOpenAIEndpoint proves the image
// surfaces resolve a dual provider to its OpenAI-side base URL before
// forwarding, exactly like the chat/responses paths do via ResolveStyle.
// Without that call the request would go to the dead primary APIBase and
// fail. Routed through /tingly/team (default-team scope), so the team
// scenario's image path is exercised through the real route chain too.
func TestImageForwarding_DualProviderResolvesOpenAIEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream, lastPath := newPathRecordingUpstream(t, map[string]any{
		"created": 1,
		"data":    []map[string]any{{"b64_json": editTestPNGBase64}},
		"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
	})
	router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleOpenAI,
		typ.ScenarioTeam, "img-model", "img-upstream-model")

	genBody := `{"model": "img-model", "prompt": "a cat"}`
	editBody := `{"model": "img-model", "prompt": "a cat", "image": "` + editTestPNGBase64 + `"}`

	tests := []struct{ name, path, body, wantPath string }{
		{name: "generation", path: "/tingly/team/v1/images/generations", body: genBody, wantPath: "/v1/images/generations"},
		{name: "edit", path: "/tingly/team/v1/images/edits", body: editBody, wantPath: "/v1/images/edits"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			assert.Equal(t, tt.wantPath, lastPath())
		})
	}
}

// TestImageGeneration_CodexPreservesMultipleImages is a gateway-level static
// contract: n crosses the public handler and native Codex client unchanged,
// and the complete upstream data array crosses back in stable order.
func TestImageGeneration_CodexPreservesMultipleImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var upstreamN float64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/backend-api/codex/images/generations", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		upstreamN, _ = body["n"].(float64)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"first"},{"b64_json":"second"}]}`))
	}))
	t.Cleanup(upstream.Close)

	loadbalance.DefaultBreakerStore().Reset()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()), config.WithDisableMigration(), config.WithDisableBuiltIn())
	require.NoError(t, err)
	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID: "codex-images", Name: "codex-images", Enabled: true,
		AuthType: typ.AuthTypeOAuth, APIStyle: protocol.APIStyleOpenAI,
		APIBase:     upstream.URL + "/backend-api",
		OAuthDetail: &typ.OAuthDetail{Issuer: ai.IssuerCodex, AccessToken: "test-token"},
	}))
	require.NoError(t, cfg.AddRule(typ.Rule{
		Scenario: typ.ScenarioImageGen, RequestModel: "image-model", Active: true,
		Services: []*loadbalance.Service{{Provider: "codex-images", Model: "gpt-image-2", Active: true}},
	}))
	hm := loadbalance.NewHealthMonitor(loadbalance.HealthMonitorConfig{ProbeEnabled: false})
	lb := NewLoadBalancer(cfg, routing.NewHealthFilter(hm))
	ph := NewHandler(ProtocolHandlerDeps{
		Config: cfg, ClientPool: client.NewClientPool(),
		RoutingSelector: routing.NewSimpleSelector(routing.NewServiceSelector(cfg, NewAffinityStore(0), lb)),
		LoadBalancer:    lb, HealthMonitor: hm,
	})
	router := gin.New()
	ph.RegisterRoutes(router, func(c *gin.Context) { c.Next() })

	req := httptest.NewRequest(http.MethodPost, "/tingly/imagegen/v1/images/generations",
		strings.NewReader(`{"model":"image-model","prompt":"two cats","n":2}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.Equal(t, float64(2), upstreamN)
	var response struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 2)
	assert.Equal(t, "first", response.Data[0].B64JSON)
	assert.Equal(t, "second", response.Data[1].B64JSON)
}

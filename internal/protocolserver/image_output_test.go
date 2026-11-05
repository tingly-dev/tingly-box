package protocolserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/vision/imagegen"
)

func postJSON(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// The Codex CLI points its image tool at {base}/images/*, base = /tingly/codex.
// The codex scenario must clear the transport gate and reach rule resolution.
func TestImageEndpoints_CodexScenarioPassesTransportGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(ProtocolHandlerDeps{}).RegisterRoutes(router, setTeamIDFromHeader)

	genBody := `{"model":"gpt-image-2","prompt":"a cat"}`
	editBody := `{"model":"gpt-image-2","prompt":"a cat","image":"` + editTestPNGBase64 + `"}`
	for _, tc := range []struct{ name, path, body string }{
		{"generation", "/tingly/codex/images/generations", genBody},
		{"generation v1", "/tingly/codex/v1/images/generations", genBody},
		{"edit", "/tingly/codex/images/edits", editBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(router, tc.path, tc.body)
			assert.NotContains(t, w.Body.String(), "does not support")
			assert.Contains(t, w.Body.String(), "not configured")
		})
	}
	// Control: other agent scenarios still fail closed.
	w := postJSON(router, "/tingly/opencode/v1/images/generations", genBody)
	assert.Contains(t, w.Body.String(), "does not support")
}

func TestImageEndpoints_OutputOptionValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(ProtocolHandlerDeps{}).RegisterRoutes(router, setTeamIDFromHeader)

	editJSON := func(extra string) string {
		return `{"model":"m","prompt":"p","image":"` + editTestPNGBase64 + `"` + extra + `}`
	}
	for _, tc := range []struct{ name, path, body, want string }{
		{"svg is not an output format", "/tingly/imagegen/v1/images/generations", `{"model":"m","prompt":"p","output_format":"svg"}`, "output_format"},
		{"frames out of range", "/tingly/imagegen/v1/images/generations", `{"model":"m","prompt":"p","output_format":"gif","frame_count":99}`, "frame_count"},
		{"duration out of range", "/tingly/imagegen/v1/images/generations", `{"model":"m","prompt":"p","output_format":"gif","frame_duration_ms":5}`, "frame_duration_ms"},
		{"frames without gif", "/tingly/imagegen/v1/images/generations", `{"model":"m","prompt":"p","frame_count":4}`, "only apply"},
		{"too many gifs", "/tingly/imagegen/v1/images/generations", `{"model":"m","prompt":"p","output_format":"gif","n":5}`, "at most 4"},
		{"edit: svg", "/tingly/imagegen/v1/images/edits", editJSON(`,"output_format":"svg"`), "output_format"},
		{"edit: frames out of range", "/tingly/imagegen/v1/images/edits", editJSON(`,"output_format":"gif","frame_count":1`), "frame_count"},
		{"edit: too many gifs", "/tingly/imagegen/v1/images/edits", editJSON(`,"output_format":"gif","n":9`), "at most 4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(router, tc.path, tc.body)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), tc.want)
			assert.Contains(t, w.Body.String(), "invalid_request_error")
		})
	}

	t.Run("valid gif options clear validation and reach rule lookup", func(t *testing.T) {
		w := postJSON(router, "/tingly/imagegen/v1/images/generations", `{"model":"m","prompt":"p","output_format":"gif","frame_count":9,"frame_duration_ms":200,"n":4}`)
		assert.Contains(t, w.Body.String(), "not configured")
	})
}

func TestCheckImageOutputSupport(t *testing.T) {
	codex := &typ.Provider{Name: "codex", APIBase: protocol.CodexAPIBase}
	compat := &typ.Provider{Name: "compat", APIBase: "https://api.example.com/v1", APIStyle: protocol.APIStyleOpenAI}
	dashscope := &typ.Provider{Name: "ds", APIBase: "https://dashscope.aliyuncs.com/api/v1", APIStyle: protocol.APIStyleOpenAI}
	minimax := &typ.Provider{Name: "mm", APIBase: "https://api.minimax.io/v1", APIStyle: protocol.APIStyleOpenAI}

	gif := imagegen.OutputOptions{Format: "gif"}
	png := imagegen.OutputOptions{Format: "png"}

	assert.NoError(t, checkImageOutputSupport(codex, gif, false))
	assert.NoError(t, checkImageOutputSupport(codex, gif, true))
	assert.NoError(t, checkImageOutputSupport(compat, gif, false), "experimental passthrough")
	assert.Error(t, checkImageOutputSupport(compat, gif, true), "multipart edit cannot carry the frame controls")
	for _, p := range []*typ.Provider{dashscope, minimax} {
		err := checkImageOutputSupport(p, gif, false)
		require.Error(t, err, p.Name)
		assert.Contains(t, err.Error(), p.Name)
		assert.Contains(t, err.Error(), "gif")
	}
	for _, p := range []*typ.Provider{codex, compat, dashscope, minimax} {
		assert.NoError(t, checkImageOutputSupport(p, png, false), p.Name)
		assert.NoError(t, checkImageOutputSupport(p, imagegen.OutputOptions{}, false), p.Name)
	}
}

// End to end through the real route chain against an OpenAI-compatible fake
// upstream: the GIF controls reach it, and the response reports the sniffed
// format.
func TestImageGeneration_GIFPassthroughToOpenAICompatible(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const gifB64 = "R0lGODlhAQABAAAAACw="
	var mu sync.Mutex
	var seen []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = b
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"created": 1, "data": []map[string]any{{"b64_json": gifB64}}})
	}))
	defer upstream.Close()
	router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleOpenAI, typ.ScenarioTeam, "img", "img-upstream")

	w := postJSON(router, "/tingly/team/v1/images/generations",
		`{"model":"img","prompt":"dancing cat","output_format":"gif","frame_count":5,"frame_duration_ms":120}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "gif", gjson.Get(w.Body.String(), "output_format").String())
	assert.Equal(t, gifB64, gjson.Get(w.Body.String(), "data.0.b64_json").String())

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "img-upstream", gjson.GetBytes(seen, "model").String())
	assert.Equal(t, "gif", gjson.GetBytes(seen, "output_format").String())
	assert.EqualValues(t, 5, gjson.GetBytes(seen, "frame_count").Int())
	assert.EqualValues(t, 120, gjson.GetBytes(seen, "frame_duration_ms").Int())
}

func TestImageGeneration_GIFRefusedForVendorWithoutAnimation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// A MiniMax-hosted provider has no animated output; the request must be
	// refused before anything is sent upstream.
	router := newSingleProviderRouter(t, "https://api.minimax.io/v1", typ.ScenarioTeam, "img", "img-upstream")

	w := postJSON(router, "/tingly/team/v1/images/generations", `{"model":"img","prompt":"x","output_format":"gif"}`)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "gif")
	assert.Contains(t, w.Body.String(), "not supported")
}

func newSingleProviderRouter(t *testing.T, apiBase string, scenario typ.RuleScenario, requestModel, upstreamModel string) *gin.Engine {
	t.Helper()
	loadbalance.DefaultBreakerStore().Reset()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()), config.WithDisableMigration(), config.WithDisableBuiltIn())
	require.NoError(t, err)
	require.NoError(t, cfg.AddProvider(&typ.Provider{
		UUID: "prov-single", Name: "prov-single", Enabled: true,
		AuthType: typ.AuthTypeAPIKey, Token: "test-key",
		APIStyle: protocol.APIStyleOpenAI, APIBase: apiBase,
	}))
	require.NoError(t, cfg.AddRule(typ.Rule{
		Scenario: scenario, RequestModel: requestModel, Active: true,
		Services: []*loadbalance.Service{{Provider: "prov-single", Model: upstreamModel, Active: true}},
	}))
	hm := loadbalance.NewHealthMonitor(loadbalance.HealthMonitorConfig{ProbeEnabled: false})
	lb := NewLoadBalancer(cfg, routing.NewHealthFilter(hm))
	ph := NewHandler(ProtocolHandlerDeps{
		Config:          cfg,
		ClientPool:      client.NewClientPool(),
		RoutingSelector: routing.NewSimpleSelector(routing.NewServiceSelector(cfg, NewAffinityStore(0), lb)),
		LoadBalancer:    lb,
		HealthMonitor:   hm,
	})
	engine := gin.New()
	ph.RegisterRoutes(engine, func(c *gin.Context) { c.Set(constant.CtxKeyAuthKind, constant.AuthKindGlobalModelToken); c.Next() })
	return engine
}

package projects

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/internal/typ"
	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/projects/llm"
	"github.com/tingly-dev/tingly-box/swagger"
)

func newConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	return cfg
}

// The coordinator's requests go to the gateway's claude_code scenario with
// the model token, and an empty model means the scenario's default tier.
func TestHostClientForCallsTheGateway(t *testing.T) {
	var gotPath, gotKey, gotBody string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.Header.Get("X-Api-Key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer gw.Close()
	host, port, err := net.SplitHostPort(strings.TrimPrefix(gw.URL, "http://"))
	require.NoError(t, err)
	p, _ := strconv.Atoi(port)

	cfg := newConfig(t)
	require.NoError(t, cfg.SetServerHost(host))
	require.NoError(t, cfg.SetServerPort(p))
	h := NewHost(cfg)

	client, model, err := h.ClientFor(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, h.cc.ClaudeCodeDefaultModel(), model)
	assert.NotEmpty(t, model)

	_, _ = llm.Complete(context.Background(), client, llm.Request{Model: model, Messages: []llm.Message{{Role: "user", Content: "hi"}}})
	assert.Equal(t, "/tingly/claude_code/v1/messages", gotPath)
	assert.Equal(t, cfg.GetModelToken(), gotKey)
	assert.Contains(t, gotBody, `"model":"`+model+`"`)
}

// Models offers the default tier first, then every other request model the
// claude_code scenario routes, once each.
func TestHostModels(t *testing.T) {
	cfg := newConfig(t)
	require.NoError(t, cfg.AddRule(typ.Rule{UUID: "p-test", Scenario: typ.ScenarioClaudeCode, RequestModel: "team-fast", Description: "Team fast", Active: true}))
	require.NoError(t, cfg.AddRule(typ.Rule{UUID: "p-off", Scenario: typ.ScenarioClaudeCode, RequestModel: "switched-off", Active: false}))
	h := NewHost(cfg)

	models, err := h.Models(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, models)
	assert.Equal(t, h.cc.ClaudeCodeDefaultModel(), models[0].ID)
	ids := map[string]int{}
	for _, m := range models {
		ids[m.ID]++
	}
	assert.Equal(t, 1, ids["team-fast"])
	assert.Zero(t, ids["switched-off"])
	for id, n := range ids {
		assert.Equal(t, 1, n, id)
	}
}

func TestHostThreadEnvPointsAtTheGateway(t *testing.T) {
	cfg := newConfig(t)
	h := NewHost(cfg)
	env, model := h.ThreadEnv(context.Background())
	assert.Equal(t, h.cc.ClaudeCodeDefaultModel(), model)
	joined := strings.Join(env, "\n")
	assert.Contains(t, joined, "/tingly/claude_code")
	assert.Contains(t, joined, cfg.GetModelToken())
}

type nopHost struct{}

func (nopHost) ClientFor(context.Context, string) (llm.Client, string, error) {
	return nil, "", nil
}
func (nopHost) Models(context.Context) ([]llm.Model, error)  { return nil, nil }
func (nopHost) ThreadEnv(context.Context) ([]string, string) { return nil, "" }

// While the flag is off every Projects route is a 404 and nothing starts;
// once it is on, requests go through and start the runtime.
func TestGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	app, err := projectsapp.New(projectsapp.Options{Host: nopHost{}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = app.Close() })

	on, starts := false, 0
	engine := gin.New()
	mgr := swagger.NewRouteManager(engine)
	module.Mount(&module.Routes{Manager: mgr}, NewHandler(app, func() bool { return on }, func() { starts++ }))

	get := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))
		return w
	}
	w := get()
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.JSONEq(t, `{"error":"projects is disabled"}`, w.Body.String())
	assert.Zero(t, starts)

	on = true
	assert.Equal(t, http.StatusOK, get().Code)
	assert.Equal(t, 1, starts)
}

// A folder session continues in a terminal through tingly-box itself, so
// the terminal routes the way the web turns did.
func TestHostHandoffGoesThroughTinglyBox(t *testing.T) {
	h := NewHost(newConfig(t))
	assert.Equal(t, []string{selfExecutable(), "cc"}, h.HandoffLaunch(""))
	assert.Equal(t, []string{selfExecutable(), "profile", "p1"}, h.HandoffLaunch("p1"))
}

func TestHostChecksModelTiers(t *testing.T) {
	h := NewHost(newConfig(t))
	ctx := context.Background()
	require.NoError(t, h.CheckLaunch(ctx, "", ""))
	assert.Error(t, h.CheckLaunch(ctx, "", "gpt-5"))
	assert.Error(t, h.CheckLaunch(ctx, "no-such-profile", ""))
}

package protocolserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newAliasTestServer builds a ProtocolHandler whose config holds a single
// claude_code profile (p1 named "mine"), enough to exercise
// profileAliasMiddleware.
func newAliasTestServer() *ProtocolHandler {
	return NewHandler(ProtocolHandlerDeps{Config: &config.Config{
		Profiles: map[string][]typ.ProfileMeta{
			"claude_code": {{ID: "p1", Name: "mine"}},
		},
	}})
}

// invokeAliasMiddleware runs profileAliasMiddleware over a request whose
// :scenario param is rawScenario and whose path embeds it, returning the
// resulting param value and rewritten URL path.
func invokeAliasMiddleware(t *testing.T, ph *ProtocolHandler, rawScenario string) (param, path string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/tingly/"+rawScenario+"/v1/messages", nil)
	c.Params = gin.Params{{Key: "scenario", Value: rawScenario}}

	ph.profileAliasMiddleware(c)

	return c.Param("scenario"), c.Request.URL.Path
}

func TestProfileAliasMiddleware_RewritesNameToID(t *testing.T) {
	s := newAliasTestServer()

	param, path := invokeAliasMiddleware(t, s, "claude_code:mine")

	if param != "claude_code:p1" {
		t.Errorf("param = %q, want %q", param, "claude_code:p1")
	}
	if path != "/tingly/claude_code:p1/v1/messages" {
		t.Errorf("path = %q, want %q", path, "/tingly/claude_code:p1/v1/messages")
	}
	// The usage tracker derives the scenario from the (now rewritten) path.
	if sc := ExtractScenarioFromPath(path); sc != "claude_code:p1" {
		t.Errorf("extractScenarioFromPath = %q, want %q", sc, "claude_code:p1")
	}
}

func TestProfileAliasMiddleware_LeavesCanonicalAndUnknownUntouched(t *testing.T) {
	s := newAliasTestServer()

	for _, raw := range []string{
		"claude_code:p1",   // already canonical
		"claude_code:nope", // unknown alias
		"claude_code",      // not profiled
		"openai",           // not profiled
	} {
		param, path := invokeAliasMiddleware(t, s, raw)
		if param != raw {
			t.Errorf("param for %q = %q, want unchanged", raw, param)
		}
		if path != "/tingly/"+raw+"/v1/messages" {
			t.Errorf("path for %q = %q, want unchanged", raw, path)
		}
	}
}

// invokeLegacyScenarioAliasMiddleware runs legacyScenarioAliasMiddleware over
// a request whose :scenario param is rawScenario and whose path embeds it.
func invokeLegacyScenarioAliasMiddleware(t *testing.T, ph *ProtocolHandler, rawScenario string) (param, path string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/tingly/"+rawScenario+"/v1/messages", nil)
	c.Params = gin.Params{{Key: "scenario", Value: rawScenario}}

	ph.legacyScenarioAliasMiddleware(c)

	return c.Param("scenario"), c.Request.URL.Path
}

func TestLegacyScenarioAliasMiddleware_RewritesAgentToCustom(t *testing.T) {
	s := newAliasTestServer()

	param, path := invokeLegacyScenarioAliasMiddleware(t, s, "agent")

	if param != "custom" {
		t.Errorf("param = %q, want %q", param, "custom")
	}
	if path != "/tingly/custom/v1/messages" {
		t.Errorf("path = %q, want %q", path, "/tingly/custom/v1/messages")
	}
	if sc := ExtractScenarioFromPath(path); sc != "custom" {
		t.Errorf("extractScenarioFromPath = %q, want %q", sc, "custom")
	}
}

func TestLegacyScenarioAliasMiddleware_LeavesOthersUntouched(t *testing.T) {
	s := newAliasTestServer()

	for _, raw := range []string{"custom", "claude_code", "claude_code:p1", "openai"} {
		param, path := invokeLegacyScenarioAliasMiddleware(t, s, raw)
		if param != raw {
			t.Errorf("param for %q = %q, want unchanged", raw, param)
		}
		if path != "/tingly/"+raw+"/v1/messages" {
			t.Errorf("path for %q = %q, want unchanged", raw, path)
		}
	}
}

func invokeContextMiddleware(t *testing.T, ph *ProtocolHandler, rawScenario string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/tingly/"+rawScenario+"/v1/chat/completions", nil)
	c.Params = gin.Params{{Key: "scenario", Value: rawScenario}}
	ph.contextMiddleware(c)
	return w
}

// A team's routing scope is "team:<id>" (what the Team page probes and what
// rules are stored under). It is not a profile, so it must not be validated as
// one: the profile check used to answer it with 400 {"error":"unknown profile
// ..."}, which the Test all dialog showed instead of a probe result.
func TestContextMiddleware_AcceptsLiveTeamScope(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	team, err := cfg.StoreManager().Team().Create("Research")
	if err != nil {
		t.Fatal(err)
	}
	ph := NewHandler(ProtocolHandlerDeps{Config: cfg})

	if w := invokeContextMiddleware(t, ph, "team:"+team.ID); w.Code != http.StatusOK {
		t.Fatalf("live team scope rejected: %d %s", w.Code, w.Body.String())
	}

	w := invokeContextMiddleware(t, ph, "team:no-such-team")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown team scope should still be refused, got %d", w.Code)
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Message == "" {
		t.Fatalf("gateway refusals must use the error envelope clients decode, got %s", w.Body.String())
	}
}

func TestContextMiddleware_ProfileErrorsUseErrorEnvelope(t *testing.T) {
	ph := newAliasTestServer()
	w := invokeContextMiddleware(t, ph, "claude_code:nope")
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if w.Code != http.StatusBadRequest || json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error.Message == "" {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

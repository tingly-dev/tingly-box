package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newAliasTestServer builds a Server whose config holds a single claude_code
// profile (p1 named "mine"), enough to exercise profileAliasMiddleware.
func newAliasTestServer() *Server {
	return &Server{config: &config.Config{
		Profiles: map[string][]typ.ProfileMeta{
			"claude_code": {{ID: "p1", Name: "mine"}},
		},
	}}
}

// invokeAliasMiddleware runs profileAliasMiddleware over a request whose
// :scenario param is rawScenario and whose path embeds it, returning the
// resulting param value and rewritten URL path.
func invokeAliasMiddleware(t *testing.T, s *Server, rawScenario string) (param, path string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/tingly/"+rawScenario+"/v1/messages", nil)
	c.Params = gin.Params{{Key: "scenario", Value: rawScenario}}

	s.profileAliasMiddleware(c)

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

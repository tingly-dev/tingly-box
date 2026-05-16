package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestContextMiddleware_ProfiledScenarioPassesThrough verifies that profiled
// scenarios (e.g. "claude_code:p3") reach the handler regardless of whether
// the in-memory config knows about the profile. The CLI validates the
// profile client-side; the middleware must not gate on a possibly-stale
// in-memory profile map (see `cc --profile` regression that returned
// HTTP 400 "unknown profile" when the server's config briefly lagged disk).
func TestContextMiddleware_ProfiledScenarioPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	s := NewServer(cfg)

	engine := gin.New()
	group := engine.Group("/tingly/:scenario/v1")
	group.Use(s.contextMiddleware)

	var seenScenario string
	group.POST("/messages", func(c *gin.Context) {
		if v, ok := c.Request.Context().Value(client.ScenarioContextKey).(string); ok {
			seenScenario = v
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	cases := []struct {
		name     string
		path     string
		want     string
		wantCode int
	}{
		{
			name:     "profile not in config still reaches handler",
			path:     "/tingly/claude_code:p3/v1/messages",
			want:     "claude_code:p3",
			wantCode: http.StatusOK,
		},
		{
			name:     "plain scenario reaches handler",
			path:     "/tingly/claude_code/v1/messages",
			want:     "claude_code",
			wantCode: http.StatusOK,
		},
		{
			name:     "invalid scenario format is rejected",
			path:     "/tingly/claude_code:/v1/messages",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "unknown base scenario is rejected when profile-shaped",
			path:     "/tingly/not_a_scenario:p1/v1/messages",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seenScenario = ""
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d, body=%s", w.Code, tc.wantCode, w.Body.String())
			}
			if tc.wantCode == http.StatusOK && seenScenario != tc.want {
				t.Fatalf("scenario in ctx = %q, want %q", seenScenario, tc.want)
			}
		})
	}

	// Also verify that when the profile IS in config, the request still works
	// — the change must not regress the happy path.
	meta, err := cfg.CreateProfile(typ.ScenarioClaudeCode, "regression-test", false)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/tingly/claude_code:"+meta.ID+"/v1/messages", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("existing profile rejected: status=%d body=%s", w.Code, w.Body.String())
	}
}

package github

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

func newTestEngine(enabled func() bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	manager := swagger.NewRouteManager(engine)
	NewHandler(nil).WithGate(enabled).RegisterRoutes(&module.Routes{V1: manager.NewGroup("api", "v1", ""), Engine: engine})
	return engine
}

// While the flag is off every route must 404 — the API and both browser
// callbacks — not merely be hidden from the UI.
func TestRoutes_GateBlocksWhenDisabled(t *testing.T) {
	engine := newTestEngine(func() bool { return false })
	for _, p := range []string{"/api/v1/github/status", "/github/manifest/callback", "/github/install/callback"} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", p, w.Code)
		}
	}
}

// Enabled but without a service (store unavailable) the routes answer 503
// instead of panicking.
func TestRoutes_EnabledWithoutServiceIs503(t *testing.T) {
	engine := newTestEngine(func() bool { return true })
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/github/status", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestGate_NilPredicateFailsClosed(t *testing.T) {
	engine := newTestEngine(nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/github/status", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

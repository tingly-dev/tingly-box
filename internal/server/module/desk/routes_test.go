package desk

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/swagger"
)

// TestRegisterRoutes_GateBlocksWhenDisabled is the behavioral guarantee
// behind the desk feature flag: every route must 404 while it is
// off, not just be hidden from navigation. PermissionModes needs no real
// Service, so it is the cheapest route to drive both states through.
func TestRegisterRoutes_GateBlocksWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	manager := swagger.NewRouteManager(engine)
	apiV1 := manager.NewGroup("api", "v1", "")

	enabled := false
	RegisterRoutes(apiV1, NewHandler(nil), func() bool { return enabled })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/desk/permission-modes", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled: status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}

	enabled = true
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req)
	if w2.Code != http.StatusOK {
		t.Fatalf("enabled: status = %d, want %d; body=%s", w2.Code, http.StatusOK, w2.Body.String())
	}
}

// TestGate_NilEnabledFuncDefaultsClosed guards against a future call site
// that forgets to pass a real check function — nil must fail closed, not
// silently allow everything through.
func TestGate_NilEnabledFuncDefaultsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gate(nil))
	engine.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

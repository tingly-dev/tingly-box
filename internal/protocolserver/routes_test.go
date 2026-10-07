package protocolserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRegisterRoutes_HeadConnectivityProbe pins the Claude Code v2.1+
// compatibility contract: HEAD <ANTHROPIC_BASE_URL> (with and without /v1)
// must return 200 so CC's pre-flight connectivity check doesn't treat the
// gateway as broken and spiral into api_retry storms.
func TestRegisterRoutes_HeadConnectivityProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	ph := NewHandler(ProtocolHandlerDeps{})
	ph.RegisterRoutes(engine, func(c *gin.Context) { c.Next() })

	srv := httptest.NewServer(engine)
	defer srv.Close()

	for _, path := range []string{"/tingly/claude_code", "/tingly/claude_code/v1"} {
		resp, err := srv.Client().Head(srv.URL + path)
		if err != nil {
			t.Fatalf("HEAD %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("HEAD %s = %d, want 200", path, resp.StatusCode)
		}
	}
}

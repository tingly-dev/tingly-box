package routing

import (
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestResolveSessionIDFromNativeClientHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		headerName string
		headerID   string
	}{
		{name: "Codex", headerName: "session_id", headerID: "codex-session-123"},
		{name: "OpenCode", headerName: "x-opencode-session", headerID: "opencode-session-456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			c.Request.RemoteAddr = "192.0.2.10:1234"
			c.Request.Header.Set(tt.headerName, tt.headerID)

			got := ResolveSessionID(c, nil)
			if got.Source != typ.SessionSourceHeader {
				t.Fatalf("Source = %q, want %q", got.Source, typ.SessionSourceHeader)
			}
			if got.Value != tt.headerID {
				t.Errorf("Value = %q, want %q", got.Value, tt.headerID)
			}
			if got.IPBackup != "192.0.2.10" {
				t.Errorf("IPBackup = %q, want %q", got.IPBackup, "192.0.2.10")
			}
		})
	}
}

func TestResolveSessionIDPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	c.Request.RemoteAddr = "192.0.2.10:1234"
	c.Request.Header.Set("session_id", "codex-session")
	c.Request.Header.Set("x-opencode-session", "opencode-session")
	c.Request.Header.Set("X-Tingly-Session-ID", "tingly-session")

	req := anthropic.MessageNewParams{
		Metadata: anthropic.MetadataParam{
			UserID: param.NewOpt("anthropic-session"),
		},
	}

	got := ResolveSessionID(c, &req)
	if got.Value != "tingly-session" {
		t.Fatalf("Tingly header Value = %q, want %q", got.Value, "tingly-session")
	}

	c.Request.Header.Del("X-Tingly-Session-ID")
	got = ResolveSessionID(c, &req)
	if got.Value != "codex-session" {
		t.Fatalf("native header Value = %q, want Codex session %q", got.Value, "codex-session")
	}

	c.Request.Header.Del("session_id")
	c.Request.Header.Del("x-opencode-session")
	got = ResolveSessionID(c, &req)
	if got.Value != "anthropic-session" {
		t.Fatalf("metadata session Value = %q, want %q", got.Value, "anthropic-session")
	}
}

func TestResolveSessionIDFallsBackToClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Request.RemoteAddr = "192.0.2.10:1234"

	got := ResolveSessionID(c, nil)
	if got.Source != typ.SessionSourceIP || got.Value != "192.0.2.10" {
		t.Fatalf("ResolveSessionID() = %+v, want IP fallback", got)
	}
}

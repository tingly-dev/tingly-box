package protocolserver

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/middleware"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestValidationErrorIsAvailableInAccessLog(t *testing.T) {
	ml, err := obs.NewMultiLogger(obs.DefaultMultiLoggerConfig(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ml.Close() })
	access := middleware.NewMemoryLogMiddleware(ml)
	engine := gin.New()
	engine.Use(access.Middleware())
	ph := &ProtocolHandler{}
	engine.POST("/tingly/:scenario/v1/chat/completions", ph.HandleOpenAIChatCompletions)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("POST", "/tingly/openai/v1/chat/completions", strings.NewReader(`{"messages":[]}`)))
	require.Equal(t, 400, w.Code)
	require.JSONEq(t, `{"error":{"message":"Model is required","type":"invalid_request_error"}}`, w.Body.String())
	entries := access.GetEntries()
	require.Len(t, entries, 1)
	require.Equal(t, "invalid_request_error: Model is required", entries[0].Data["error"])
	require.Equal(t, "openai", entries[0].Data["scenario"])
}

func TestCommittedStreamFailureIsObservedWithoutUsageMetadata(t *testing.T) {
	ml, err := obs.NewMultiLogger(obs.DefaultMultiLoggerConfig(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ml.Close() })
	access := middleware.NewMemoryLogMiddleware(ml)
	engine := gin.New()
	engine.Use(access.Middleware())
	ph := &ProtocolHandler{}
	engine.GET("/stream", func(c *gin.Context) {
		SetTrackingContext(c, nil, nil, "model", "model", true)
		c.String(200, "data: started\n\n")
		c.Writer.Flush()
		ph.trackUsageWithTokenUsage(c, protocol.ZeroTokenUsage(), errors.New("simulated mid-stream error"))
	})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("GET", "/stream", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "simulated mid-stream error", access.GetEntries()[0].Data["error"])
}

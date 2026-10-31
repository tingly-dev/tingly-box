package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
)

func TestRecoveryAbortsInterruptedStream(t *testing.T) {
	engine := gin.New()
	engine.Use(Recovery())
	engine.GET("/audio", func(c *gin.Context) {
		c.Header("Content-Type", "audio/mpeg")
		_, _ = c.Writer.Write([]byte("partial"))
		c.Writer.Flush()
		panic(http.ErrAbortHandler)
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/audio")
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.Error(t, err)
	require.Equal(t, "partial", string(body))
}

func TestRecoveryRetainsAccessLogAndCorrelatedStack(t *testing.T) {
	ml, err := obs.NewMultiLogger(obs.DefaultMultiLoggerConfig(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ml.Close() })
	global := logrus.StandardLogger()
	previous := global.ReplaceHooks(logrus.LevelHooks{})
	t.Cleanup(func() { global.ReplaceHooks(previous) })
	global.AddHook(obs.NewMultiLoggerHook(ml, nil))
	access := NewMemoryLogMiddleware(ml)
	engine := gin.New()
	engine.Use(access.Middleware(), Recovery())
	engine.GET("/tingly/:scenario/panic", func(c *gin.Context) { panic("handler exploded") })
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("GET", "/tingly/openai/panic", nil))
	require.Equal(t, 500, w.Code)
	require.NotEmpty(t, w.Header().Get("X-Request-Id"))
	entries := access.GetEntries()
	require.Len(t, entries, 1)
	require.Equal(t, 500, entries[0].Data["status"])
	require.Equal(t, "panic: handler exploded", entries[0].Data["error"])
	require.Equal(t, "openai", entries[0].Data["scenario"])
	panicLogs := ml.WithSource(obs.LogSourceSystem).GetMemoryEntries()
	require.Len(t, panicLogs, 1)
	require.Equal(t, entries[0].Data["request_id"], panicLogs[0].Data["request_id"])
	require.Contains(t, panicLogs[0].Data["stack"], "TestRecoveryRetainsAccessLogAndCorrelatedStack")
}

func TestErrorWritersRetainMessagesWithoutChangingWireShapes(t *testing.T) {
	for _, name := range []string{"message", "failure", "send"} {
		t.Run(name, func(t *testing.T) {
			access, _ := setupTestMiddleware()
			engine := gin.New()
			engine.Use(access.Middleware())
			engine.GET("/bad", func(c *gin.Context) {
				switch name {
				case "message":
					apierr.Message(c, 400, "invalid model")
				case "failure":
					apierr.Failure(c, 400, "invalid model")
				case "send":
					apierr.Send(c, 400, errors.New("invalid model"), "invalid_request")
				}
			})
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest("GET", "/bad", nil))
			require.Equal(t, 400, w.Code)
			require.Contains(t, w.Body.String(), "invalid model")
			require.Equal(t, "invalid model", access.GetEntries()[0].Data["error"])
		})
	}
}

func TestCommittedStreamFailureIsLoggedAsError(t *testing.T) {
	access, _ := setupTestMiddleware()
	engine := gin.New()
	engine.Use(access.Middleware())
	engine.GET("/stream", func(c *gin.Context) {
		c.String(200, "data: started\n\n")
		c.Writer.Flush()
		c.Error(errors.New("stream interrupted"))
	})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("GET", "/stream", nil))
	require.Equal(t, 200, w.Code)
	entry := access.GetEntries()[0]
	require.Equal(t, logrus.ErrorLevel, entry.Level)
	require.Equal(t, "stream interrupted", entry.Data["error"])
}

func TestManagementScenarioParameterDoesNotMakeAnAIRequest(t *testing.T) {
	access, _ := setupTestMiddleware()
	engine := gin.New()
	engine.Use(access.Middleware())
	engine.GET("/api/v1/scenario/:scenario", func(c *gin.Context) { c.Status(200) })
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/scenario/_global", nil))
	require.NotContains(t, access.GetEntries()[0].Data, "scenario")
}

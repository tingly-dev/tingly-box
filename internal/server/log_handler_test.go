package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/middleware"
	"github.com/tingly-dev/tingly-box/internal/obs"
)

func TestGetLogsCombinesFiltersBeforeTakingNewest(t *testing.T) {
	ml, err := obs.NewMultiLogger(obs.DefaultMultiLoggerConfig(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ml.Close() })
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	l := ml.GetLogrusLogger(obs.LogSourceHTTP)
	for i := 0; i < 4; i++ {
		l.WithTime(at.Add(time.Duration(i)*time.Second)).Errorf("error-%d", i)
	}
	l.WithTime(at.Add(5 * time.Second)).Info("irrelevant info")
	h := NewWebHandler(WebDeps{MemoryLogMW: middleware.NewMemoryLogMiddleware(ml)})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/logs?limit=1&level=error&since=2026-10-21T00:00:01Z", nil)
	h.GetLogs(c)
	require.Equal(t, 200, w.Code)
	var response LogsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, 2, response.Total)
	require.Len(t, response.Logs, 1)
	require.Equal(t, "error-3", response.Logs[0].Message)
}
